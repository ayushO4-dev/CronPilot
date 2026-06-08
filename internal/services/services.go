// Package services manages systemd service units. Reads go through
// `systemctl --output=json` / `journalctl` (robust, no cgo); state-changing
// actions go through `systemctl`, escalated with `sudo -n` when not root so the
// allowlisted-sudoers security model applies. Unit names are validated to avoid
// passing flags or shell metacharacters (commands are exec'd without a shell).
package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrInvalidName is returned for a unit name failing validation.
	ErrInvalidName = errors.New("invalid unit name")
	// ErrInvalidAction is returned for an unsupported action.
	ErrInvalidAction = errors.New("invalid action")
	// ErrActionFailed wraps a failed management command (includes its output).
	ErrActionFailed = errors.New("action failed")
)

// unitNameRe permits the safe charset systemd uses for .service unit names,
// including template instances (e.g. getty@tty1.service). The first character
// must not be '-' so a name can never be mistaken for a command-line flag.
var unitNameRe = regexp.MustCompile(`^[a-zA-Z0-9@._:][a-zA-Z0-9@._:-]*\.service$`)

var validActions = map[string]bool{
	"start": true, "stop": true, "restart": true, "enable": true, "disable": true,
}

// Unit is a row in the services list.
type Unit struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	LoadState   string `json:"loadState"`
	ActiveState string `json:"activeState"`
	SubState    string `json:"subState"`
	Enabled     string `json:"enabled"`
}

// Detail is the expanded view of a single unit.
type Detail struct {
	Unit
	FragmentPath  string `json:"fragmentPath"`
	MainPID       int    `json:"mainPID"`
	MemoryCurrent uint64 `json:"memoryCurrent"`
	Since         string `json:"since"`
}

// ValidUnitName reports whether name is an acceptable .service unit name.
func ValidUnitName(name string) bool { return unitNameRe.MatchString(name) }

type listUnit struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

type listUnitFile struct {
	UnitFile string `json:"unit_file"`
	State    string `json:"state"`
}

// List returns all loaded .service units (excluding not-found references),
// merged with their unit-file enablement state.
func List(ctx context.Context) ([]Unit, error) {
	out, err := output(ctx, "systemctl", "list-units", "--type=service", "--all", "--output=json", "--no-pager")
	if err != nil {
		return nil, err
	}
	var lus []listUnit
	if err := json.Unmarshal(out, &lus); err != nil {
		return nil, fmt.Errorf("parse list-units: %w", err)
	}

	enabled := map[string]string{}
	if fout, ferr := output(ctx, "systemctl", "list-unit-files", "--type=service", "--output=json", "--no-pager"); ferr == nil {
		var lufs []listUnitFile
		if json.Unmarshal(fout, &lufs) == nil {
			for _, f := range lufs {
				enabled[f.UnitFile] = f.State
			}
		}
	}

	units := make([]Unit, 0, len(lus))
	for _, u := range lus {
		if u.Load == "not-found" {
			continue
		}
		units = append(units, Unit{
			Name:        u.Unit,
			Description: u.Description,
			LoadState:   u.Load,
			ActiveState: u.Active,
			SubState:    u.Sub,
			Enabled:     enabled[u.Unit],
		})
	}
	return units, nil
}

// Get returns the detailed status of one unit.
func Get(ctx context.Context, name string) (*Detail, error) {
	if !ValidUnitName(name) {
		return nil, ErrInvalidName
	}
	props := []string{
		"Id", "Description", "LoadState", "ActiveState", "SubState", "UnitFileState",
		"FragmentPath", "MainPID", "MemoryCurrent", "ActiveEnterTimestamp",
	}
	args := []string{"show", name}
	for _, p := range props {
		args = append(args, "-p", p)
	}
	out, err := output(ctx, "systemctl", args...)
	if err != nil {
		return nil, err
	}
	m := parseKV(out)

	d := &Detail{
		Unit: Unit{
			Name:        firstNonEmpty(m["Id"], name),
			Description: m["Description"],
			LoadState:   m["LoadState"],
			ActiveState: m["ActiveState"],
			SubState:    m["SubState"],
			Enabled:     m["UnitFileState"],
		},
		FragmentPath: m["FragmentPath"],
		Since:        m["ActiveEnterTimestamp"],
	}
	if pid, perr := strconv.Atoi(strings.TrimSpace(m["MainPID"])); perr == nil {
		d.MainPID = pid
	}
	if mem, merr := strconv.ParseUint(strings.TrimSpace(m["MemoryCurrent"]), 10, 64); merr == nil {
		d.MemoryCurrent = mem
	}
	return d, nil
}

// Action performs a state-changing management command on a unit.
func Action(ctx context.Context, name, action string) error {
	if !ValidUnitName(name) {
		return ErrInvalidName
	}
	if !validActions[action] {
		return ErrInvalidAction
	}
	return runPrivileged(ctx, "systemctl", action, name)
}

// Logs returns the most recent journal lines for a unit.
func Logs(ctx context.Context, name string, lines int) ([]string, error) {
	if !ValidUnitName(name) {
		return nil, ErrInvalidName
	}
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	out, err := output(ctx, "journalctl", "-u", name, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso")
	if err != nil {
		return nil, err
	}
	var res []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		res = append(res, sc.Text())
	}
	return res, sc.Err()
}

// output runs a read-only command and returns stdout, surfacing stderr on error.
func output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("%s: %s", name, msg)
	}
	return stdout.Bytes(), nil
}

// runPrivileged runs a management command, escalating via non-interactive sudo
// when the daemon is not already root.
func runPrivileged(ctx context.Context, name string, args ...string) error {
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.CommandContext(ctx, name, args...)
	} else {
		cmd = exec.CommandContext(ctx, "sudo", append([]string{"-n", name}, args...)...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%w: %s", ErrActionFailed, msg)
	}
	return nil
}

func parseKV(b []byte) map[string]string {
	m := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '='); i >= 0 {
			m[line[:i]] = line[i+1:]
		}
	}
	return m
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
