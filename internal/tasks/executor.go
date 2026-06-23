package tasks

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ayushkanoje/cronpilot/internal/services"
)

const maxOutput = 8 << 10 // cap captured command output at 8 KiB

// ActionResult records the outcome of one executed action, for run history.
type ActionResult struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// execAction runs a single action. Actions run without m.mu held (so slow
// commands don't block the engine); the shared flag/task maps are guarded
// individually where touched.
func (m *Manager) execAction(ctx context.Context, a Action, defaultRunAs string) ActionResult {
	switch a.Kind {
	case "command":
		return m.execCommand(ctx, a, defaultRunAs)
	case "service":
		unit := pstr(a.Params, "unit")
		op := pstr(a.Params, "action")
		res := ActionResult{Kind: "service", Detail: op + " " + unit, OK: true}
		if err := services.Action(ctx, unit, op); err != nil {
			res.OK = false
			res.Error = err.Error()
		}
		return res
	case "flag":
		name := pstr(a.Params, "name")
		set := pbool(a.Params, "value")
		m.mu.Lock()
		m.flags[name] = set
		m.mu.Unlock()
		return ActionResult{Kind: "flag", Detail: name + "=" + boolStr(set), OK: true}
	case "taskToggle":
		id := pstr(a.Params, "task")
		enabled := pbool(a.Params, "enabled")
		res := ActionResult{Kind: "taskToggle", Detail: id + " -> " + enabledStr(enabled), OK: true}
		m.mu.Lock()
		t := m.tasks[id]
		m.mu.Unlock()
		if t != nil {
			if err := m.store.SetTaskEnabled(id, enabled); err != nil {
				res.OK = false
				res.Error = err.Error()
			} else {
				m.mu.Lock()
				t.Enabled = enabled
				m.mu.Unlock()
				go m.reschedule()
			}
		} else {
			res.OK = false
			res.Error = "task not found"
		}
		return res
	case "log":
		msg := pstr(a.Params, "message")
		m.log.Info("task log", "message", msg)
		return ActionResult{Kind: "log", Detail: msg, OK: true, Output: msg}
	default:
		return ActionResult{Kind: a.Kind, OK: false, Error: "unknown action"}
	}
}

func (m *Manager) execCommand(ctx context.Context, a Action, defaultRunAs string) ActionResult {
	command := pstr(a.Params, "command")
	res := ActionResult{Kind: "command", Detail: command}
	if strings.TrimSpace(command) == "" {
		res.Error = "empty command"
		return res
	}

	timeout := pint(a.Params, "timeoutSeconds")
	if timeout <= 0 {
		timeout = 30
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	runAs := pstr(a.Params, "runAs")
	if runAs == "" {
		runAs = defaultRunAs
	}

	var cmd *exec.Cmd
	if runAs != "" && os.Geteuid() == 0 {
		cmd = exec.CommandContext(cctx, "sudo", "-n", "-u", runAs, "sh", "-c", command)
	} else {
		cmd = exec.CommandContext(cctx, "sh", "-c", command)
	}

	out, err := cmd.CombinedOutput()
	res.Output = truncate(strings.TrimRight(string(out), "\n"), maxOutput)
	if cctx.Err() == context.DeadlineExceeded {
		res.Error = "timed out"
		return res
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	return res
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n… (truncated)"
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func enabledStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}
