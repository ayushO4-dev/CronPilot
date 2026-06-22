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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ayushkanoje/cronpilot/internal/terminal"
)

var (
	// ErrInvalidName is returned for a unit name failing validation.
	ErrInvalidName = errors.New("invalid unit name")
	// ErrInvalidAction is returned for an unsupported action.
	ErrInvalidAction = errors.New("invalid action")
	// ErrActionFailed wraps a failed management command (includes its output).
	ErrActionFailed = errors.New("action failed")
	// ErrAuth is returned when a supplied root password is incorrect.
	ErrAuth = errors.New("incorrect root password")
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

// systemdUnitDirs are the directories a unit fragment file may legitimately
// live in; edits are confined to these.
var systemdUnitDirs = []string{"/etc/systemd/", "/run/systemd/", "/lib/systemd/", "/usr/lib/systemd/"}

func validUnitPath(p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	p = filepath.Clean(p)
	for _, d := range systemdUnitDirs {
		if strings.HasPrefix(p, d) {
			return true
		}
	}
	return false
}

// fragmentPath returns the on-disk unit file backing a unit (per systemd).
func fragmentPath(ctx context.Context, name string) (string, error) {
	if !ValidUnitName(name) {
		return "", ErrInvalidName
	}
	out, err := output(ctx, "systemctl", "show", name, "-p", "FragmentPath", "--value")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func fileWritable(path string) bool {
	if os.Geteuid() == 0 {
		return true
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0) // no O_TRUNC: does not modify
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// ReadUnitFile returns the unit's on-disk file path, its contents, and whether
// the daemon can write to it.
func ReadUnitFile(ctx context.Context, name string) (path, content string, writable bool, err error) {
	path, err = fragmentPath(ctx, name)
	if err != nil {
		return "", "", false, err
	}
	if !validUnitPath(path) {
		return path, "", false, fmt.Errorf("no editable unit file for %s", name)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return path, "", false, err
	}
	return path, string(b), fileWritable(path), nil
}

// WriteUnitFile overwrites the unit's on-disk file and, when reload is set, runs
// daemon-reload. When password is non-empty the write is performed as root via
// `su` (for files the daemon cannot write directly); otherwise it writes
// directly, which requires the daemon to already have write access. Returns the
// path written.
func WriteUnitFile(ctx context.Context, name, content string, reload bool, password string) (string, error) {
	path, err := fragmentPath(ctx, name)
	if err != nil {
		return "", err
	}
	if !validUnitPath(path) {
		return path, fmt.Errorf("no editable unit file for %s", name)
	}
	if password != "" {
		return path, writeViaRoot(ctx, path, content, reload, password)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		if os.IsPermission(err) {
			return path, fmt.Errorf("cannot write %s: the daemon needs write access (use Sudo, or run as root)", path)
		}
		return path, err
	}
	if reload {
		if err := Reload(ctx); err != nil {
			return path, fmt.Errorf("saved %s, but daemon-reload failed: %w", path, err)
		}
	}
	return path, nil
}

// writeViaRoot writes content to a daemon-owned temp file, then installs it into
// place as root (via su with the supplied password), optionally reloading.
func writeViaRoot(ctx context.Context, path, content string, reload bool, password string) error {
	if strings.ContainsAny(path, "'\n") {
		return fmt.Errorf("unsafe unit path")
	}
	tmp, err := os.CreateTemp("", "cronpilot-unit-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmpName, 0o644) // root must be able to read it

	command := fmt.Sprintf("install -m 0644 '%s' '%s'", tmpName, path)
	if reload {
		command += " && systemctl daemon-reload"
	}
	out, err := terminal.RunRoot(ctx, password, command)
	if err != nil {
		if isAuthFailure(out) {
			return ErrAuth
		}
		if msg := strings.TrimSpace(out); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

// VerifyRoot checks the root password (a no-op when the daemon is already root).
func VerifyRoot(ctx context.Context, password string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	out, err := terminal.RunRoot(ctx, password, "true")
	if err != nil {
		if isAuthFailure(out) {
			return ErrAuth
		}
		if msg := strings.TrimSpace(out); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

func isAuthFailure(out string) bool {
	l := strings.ToLower(out)
	return strings.Contains(l, "authentication failure") || strings.Contains(l, "incorrect password")
}

// Reload runs `systemctl daemon-reload` (privileged) so edited unit files take
// effect.
func Reload(ctx context.Context) error {
	return runPrivileged(ctx, "systemctl", "daemon-reload")
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
