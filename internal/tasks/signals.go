package tasks

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/ayushkanoje/cronpilot/internal/services"
	"github.com/ayushkanoje/cronpilot/internal/system"
)

// evalContext caches expensive lookups (services, processes, metrics) for the
// duration of one task evaluation, loading each only if a contact needs it.
type evalContext struct {
	ctx    context.Context
	now    time.Time
	m      *Manager
	taskID string // owning task, for resolving same-task "rung" contacts

	services       map[string]string // unit -> activeState
	servicesLoaded bool
	procNames      map[string]bool
	procsLoaded    bool
	metrics        *system.Summary
	metricsLoaded  bool
}

func (m *Manager) newEvalContext(ctx context.Context, taskID string) *evalContext {
	return &evalContext{ctx: ctx, now: time.Now(), m: m, taskID: taskID}
}

// rungEnergized reports whether a rung's condition is satisfied.
func (c *evalContext) rungEnergized(r Rung) bool {
	if len(r.Contacts) == 0 {
		// No conditions: an unconditional rung is always energized (so a
		// scheduled task with a bare action runs every fire).
		return true
	}
	if r.Match == MatchAny {
		for _, ct := range r.Contacts {
			if c.contactTrue(ct) {
				return true
			}
		}
		return false
	}
	for _, ct := range r.Contacts {
		if !c.contactTrue(ct) {
			return false
		}
	}
	return true
}

func (c *evalContext) contactTrue(ct Contact) bool {
	res := false
	switch ct.Kind {
	case "service":
		c.loadServices()
		want := pstr(ct.Params, "state")
		if want == "" {
			want = "active"
		}
		res = c.services[pstr(ct.Params, "unit")] == want
	case "process":
		c.loadProcs()
		res = c.procNames[pstr(ct.Params, "name")]
	case "time":
		res = c.timeWindow(ct.Params)
	case "metric":
		res = c.metricCompare(ct.Params)
	case "file":
		_, err := os.Stat(pstr(ct.Params, "path"))
		res = err == nil
	case "flag":
		c.m.mu.Lock()
		res = c.m.flags[pstr(ct.Params, "name")]
		c.m.mu.Unlock()
	case "taskState":
		want := pstr(ct.Params, "state") // enabled | disabled
		c.m.mu.Lock()
		if t := c.m.tasks[pstr(ct.Params, "task")]; t != nil {
			res = (want == "disabled") != t.Enabled
		}
		c.m.mu.Unlock()
	case "rung":
		// True if the referenced rung (same task) was energized on its most
		// recent evaluation.
		c.m.mu.Lock()
		res = c.m.lastEnergized[c.taskID+"/"+pstr(ct.Params, "rung")]
		c.m.mu.Unlock()
	}
	if ct.Negate {
		res = !res
	}
	return res
}

func (c *evalContext) loadServices() {
	if c.servicesLoaded {
		return
	}
	c.servicesLoaded = true
	c.services = map[string]string{}
	if units, err := services.List(c.ctx); err == nil {
		for _, u := range units {
			c.services[u.Name] = u.ActiveState
		}
	}
}

func (c *evalContext) loadProcs() {
	if c.procsLoaded {
		return
	}
	c.procsLoaded = true
	c.procNames = map[string]bool{}
	if procs, err := process.ProcessesWithContext(c.ctx); err == nil {
		for _, p := range procs {
			if name, err := p.NameWithContext(c.ctx); err == nil {
				c.procNames[name] = true
			}
		}
	}
}

func (c *evalContext) loadMetrics() {
	if c.metricsLoaded {
		return
	}
	c.metricsLoaded = true
	if s, err := system.Collect(); err == nil {
		c.metrics = s
	}
}

func (c *evalContext) metricCompare(p map[string]any) bool {
	c.loadMetrics()
	if c.metrics == nil {
		return false
	}
	var cur float64
	switch pstr(p, "metric") {
	case "cpu":
		cur = c.metrics.CPU.Percent
	case "mem":
		cur = c.metrics.Memory.UsedPercent
	case "swap":
		cur = c.metrics.Swap.UsedPercent
	case "load1":
		if c.metrics.Load != nil {
			cur = c.metrics.Load.Load1
		}
	case "disk":
		mount := pstr(p, "mount")
		if mount == "" {
			mount = "/"
		}
		for _, d := range c.metrics.Disks {
			if d.Mountpoint == mount {
				cur = d.UsedPercent
				break
			}
		}
	default:
		return false
	}
	return compare(cur, pstr(p, "op"), pfloat(p, "value"))
}

func (c *evalContext) timeWindow(p map[string]any) bool {
	start, ok1 := parseHM(pstr(p, "start"))
	end, ok2 := parseHM(pstr(p, "end"))
	if !ok1 || !ok2 {
		return false
	}
	if days, ok := p["days"].([]any); ok && len(days) > 0 {
		today := int(c.now.Weekday())
		match := false
		for _, d := range days {
			if int(pfloatVal(d)) == today {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	mins := c.now.Hour()*60 + c.now.Minute()
	if start <= end {
		return mins >= start && mins <= end
	}
	// Overnight window (e.g. 22:00-06:00).
	return mins >= start || mins <= end
}

func compare(a float64, op string, b float64) bool {
	switch op {
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case "==":
		return a == b
	case "!=":
		return a != b
	default:
		return false
	}
}

func parseHM(s string) (int, bool) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// --- param helpers (JSON objects decode into map[string]any) ---

func pstr(p map[string]any, k string) string {
	if v, ok := p[k].(string); ok {
		return v
	}
	return ""
}

func pbool(p map[string]any, k string) bool {
	if v, ok := p[k].(bool); ok {
		return v
	}
	return false
}

func pfloat(p map[string]any, k string) float64 {
	return pfloatVal(p[k])
}

func pfloatVal(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

func pint(p map[string]any, k string) int { return int(pfloat(p, k)) }
