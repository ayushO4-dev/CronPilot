// Package processes lists and controls running processes via gopsutil. CPU% is
// computed from the delta in CPU time between successive List calls (top-style
// instantaneous usage), so a Sampler keeps the previous snapshot. Signals are
// sent via `kill`, escalated with `sudo -n` when not root.
package processes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

var (
	// ErrNotFound is returned when a pid does not exist.
	ErrNotFound = errors.New("process not found")
	// ErrInvalidPID is returned for a non-positive or protected pid.
	ErrInvalidPID = errors.New("invalid pid")
	// ErrInvalidSignal is returned for an unsupported signal.
	ErrInvalidSignal = errors.New("invalid signal")
	// ErrSignalFailed wraps a failed kill (includes its output).
	ErrSignalFailed = errors.New("signal failed")
)

// Process is a row in the process list.
type Process struct {
	PID           int32   `json:"pid"`
	PPID          int32   `json:"ppid"`
	Name          string  `json:"name"`
	User          string  `json:"user"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryPercent float64 `json:"memoryPercent"`
	RSS           uint64  `json:"rss"`
	Status        string  `json:"status"`
	Cmdline       string  `json:"cmdline"`
	CreateTime    int64   `json:"createTime"`
}

// Detail is the expanded view of one process.
type Detail struct {
	Process
	Exe        string `json:"exe"`
	Cwd        string `json:"cwd"`
	NumThreads int32  `json:"numThreads"`
	Nice       int32  `json:"nice"`
}

type cpuSnap struct {
	total float64
	at    time.Time
}

// Sampler produces process lists with instantaneous CPU% derived from the
// previous snapshot.
type Sampler struct {
	mu   sync.Mutex
	prev map[int32]cpuSnap
}

// NewSampler creates an empty Sampler. The first List has CPU% of 0 (no prior
// snapshot); subsequent calls report usage over the interval between calls.
func NewSampler() *Sampler {
	return &Sampler{prev: make(map[int32]cpuSnap)}
}

// List enumerates processes. Per-process failures are skipped field-by-field
// rather than failing the whole call.
func (s *Sampler) List(ctx context.Context) ([]Process, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.prev
	next := make(map[int32]cpuSnap, len(procs))
	out := make([]Process, 0, len(procs))

	for _, p := range procs {
		pid := p.Pid

		var total float64
		if t, terr := p.TimesWithContext(ctx); terr == nil && t != nil {
			total = t.User + t.System
		}
		next[pid] = cpuSnap{total: total, at: now}

		cpu := 0.0
		if ps, ok := prev[pid]; ok {
			if dt := now.Sub(ps.at).Seconds(); dt > 0 {
				cpu = (total - ps.total) / dt * 100
				if cpu < 0 {
					cpu = 0
				}
			}
		}

		name, _ := p.NameWithContext(ctx)
		username, _ := p.UsernameWithContext(ctx)
		memPct, _ := p.MemoryPercentWithContext(ctx)
		var rss uint64
		if mi, merr := p.MemoryInfoWithContext(ctx); merr == nil && mi != nil {
			rss = mi.RSS
		}
		status := ""
		if st, serr := p.StatusWithContext(ctx); serr == nil && len(st) > 0 {
			status = st[0]
		}
		cmdline, _ := p.CmdlineWithContext(ctx)
		ppid, _ := p.PpidWithContext(ctx)
		ct, _ := p.CreateTimeWithContext(ctx)

		out = append(out, Process{
			PID:           pid,
			PPID:          ppid,
			Name:          name,
			User:          username,
			CPUPercent:    cpu,
			MemoryPercent: float64(memPct),
			RSS:           rss,
			Status:        status,
			Cmdline:       cmdline,
			CreateTime:    ct,
		})
	}

	s.prev = next
	return out, nil
}

// GetDetail returns the detailed status of one process.
func GetDetail(ctx context.Context, pid int32) (*Detail, error) {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, ErrNotFound
	}

	name, _ := p.NameWithContext(ctx)
	username, _ := p.UsernameWithContext(ctx)
	memPct, _ := p.MemoryPercentWithContext(ctx)
	var rss uint64
	if mi, merr := p.MemoryInfoWithContext(ctx); merr == nil && mi != nil {
		rss = mi.RSS
	}
	status := ""
	if st, serr := p.StatusWithContext(ctx); serr == nil && len(st) > 0 {
		status = st[0]
	}
	cmdline, _ := p.CmdlineWithContext(ctx)
	ppid, _ := p.PpidWithContext(ctx)
	ct, _ := p.CreateTimeWithContext(ctx)
	cpu, _ := p.CPUPercentWithContext(ctx)
	exe, _ := p.ExeWithContext(ctx)
	cwd, _ := p.CwdWithContext(ctx)
	nt, _ := p.NumThreadsWithContext(ctx)
	nice, _ := p.NiceWithContext(ctx)

	return &Detail{
		Process: Process{
			PID:           pid,
			PPID:          ppid,
			Name:          name,
			User:          username,
			CPUPercent:    cpu,
			MemoryPercent: float64(memPct),
			RSS:           rss,
			Status:        status,
			Cmdline:       cmdline,
			CreateTime:    ct,
		},
		Exe:        exe,
		Cwd:        cwd,
		NumThreads: nt,
		Nice:       nice,
	}, nil
}

var validSignals = map[string]bool{"TERM": true, "KILL": true, "HUP": true, "INT": true}

// Signal sends a signal to a process. pid 0/1 are rejected to avoid foot-guns.
func Signal(ctx context.Context, pid int, sig string) error {
	if pid <= 1 {
		return ErrInvalidPID
	}
	sig = strings.ToUpper(sig)
	if !validSignals[sig] {
		return ErrInvalidSignal
	}

	args := []string{"-s", sig, strconv.Itoa(pid)}
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.CommandContext(ctx, "kill", args...)
	} else {
		cmd = exec.CommandContext(ctx, "sudo", append([]string{"-n", "kill"}, args...)...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%w: %s", ErrSignalFailed, msg)
	}
	return nil
}
