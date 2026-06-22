// Package terminal spawns interactive shell sessions backed by a PTY. Each web
// terminal connection owns one Session and closes it on disconnect, so there is
// no shared global shell. Sessions can run as the daemon's own user (direct
// shell) or as another account via `su -`, with the password either fed from a
// pre-verified login or typed interactively in the terminal.
package terminal

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// Session is a running shell attached to a pseudo-terminal.
type Session struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

// Start launches a login shell in a PTY of the given size. If shell is empty it
// is auto-detected. The session inherits the daemon's environment plus a sane
// TERM and starts in the user's home directory when available.
func Start(shell string, cols, rows uint16) (*Session, error) {
	if shell == "" {
		shell = detectShell()
	}
	cmd := exec.Command(shell, "-l")
	cmd.Env = withTerm(os.Environ())
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	return &Session{ptmx: ptmx, cmd: cmd}, nil
}

// Read reads shell output from the PTY.
func (s *Session) Read(p []byte) (int, error) { return s.ptmx.Read(p) }

// Write sends user input to the PTY.
func (s *Session) Write(p []byte) (int, error) { return s.ptmx.Write(p) }

// Resize updates the PTY window size.
func (s *Session) Resize(cols, rows uint16) error {
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// Close kills the shell process and releases the PTY.
func (s *Session) Close() error {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	err := s.ptmx.Close()
	_ = s.cmd.Wait()
	return err
}

// SystemUser is a login-capable account offered by the web-terminal picker.
type SystemUser struct {
	Name    string `json:"name"`
	UID     int    `json:"uid"`
	Shell   string `json:"shell"`
	Current bool   `json:"current"` // the account the daemon itself runs as
}

// ListUsers returns root plus regular accounts (uid >= 1000) that have a real
// login shell, from /etc/passwd.
func ListUsers() ([]SystemUser, error) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	euid := os.Geteuid()
	var out []SystemUser
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		shell := fields[6]
		if strings.HasSuffix(shell, "nologin") || strings.HasSuffix(shell, "false") {
			continue
		}
		if uid != 0 && (uid < 1000 || uid >= 65534) {
			continue
		}
		out = append(out, SystemUser{Name: fields[0], UID: uid, Shell: shell, Current: uid == euid})
	}
	return out, sc.Err()
}

// StartUser launches a shell session for the given account. The daemon's own
// account gets a direct shell; any other account goes through `su - <user>`.
// When password is non-empty (a pre-verified login), it is fed to su's prompt
// so the user lands straight in the shell; when empty, su's prompt is left for
// the user to answer interactively in the terminal.
func StartUser(username, password, shell string, cols, rows uint16) (*Session, error) {
	if username == "" || isCurrentUser(username) {
		return Start(shell, cols, rows)
	}
	cmd := exec.Command("su", "-", username)
	cmd.Env = withTerm(os.Environ())
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	s := &Session{ptmx: ptmx, cmd: cmd}
	// As root, su never prompts — writing the password would inject it into
	// the shell as a command, so only feed when a prompt will actually appear.
	if password != "" && os.Geteuid() != 0 {
		if waitForPrompt(ptmx, 2500*time.Millisecond) {
			_, _ = ptmx.Write([]byte(password + "\n"))
		}
	}
	return s, nil
}

// VerifyPassword checks an account password by driving `su <user> -c true`
// through a PTY. When the daemon runs as root (where su skips the prompt), it
// drops to nobody via runuser so the prompt is enforced; if that tool is
// missing, verification is skipped (returns true) rather than locking out.
func VerifyPassword(username, password string) bool {
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		if _, err := exec.LookPath("runuser"); err != nil {
			return true
		}
		cmd = exec.Command("runuser", "-u", "nobody", "--", "su", username, "-c", "true")
	} else {
		cmd = exec.Command("su", username, "-c", "true")
	}
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return false
	}
	defer ptmx.Close()

	go func() {
		if waitForPrompt(ptmx, 2500*time.Millisecond) {
			_, _ = ptmx.Write([]byte(password + "\n"))
		}
		// Keep draining so su can write its result and exit.
		_ = ptmx.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 256)
		for {
			if _, err := ptmx.Read(buf); err != nil {
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err == nil
	case <-time.After(6 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return false
	}
}

// RunRoot runs a shell command as root via `su`, supplying root's password at
// the prompt and capturing the command's combined output. If the daemon already
// runs as root, the command runs directly (the password is ignored). A wrong
// password yields a non-nil error and "Authentication failure" in the output.
func RunRoot(ctx context.Context, password, command string) (string, error) {
	if os.Geteuid() == 0 {
		out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
		return string(out), err
	}
	cmd := exec.CommandContext(ctx, "su", "-c", command)
	cmd.Env = withTerm(os.Environ())
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", err
	}
	defer ptmx.Close()

	var mu sync.Mutex
	var out strings.Builder
	go func() {
		if waitForPrompt(ptmx, 3*time.Second) {
			_, _ = ptmx.Write([]byte(password + "\n"))
		}
		buf := make([]byte, 4096)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				mu.Lock()
				out.Write(buf[:n])
				mu.Unlock()
			}
			if rerr != nil {
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	read := func() string { mu.Lock(); defer mu.Unlock(); return out.String() }
	select {
	case err := <-done:
		return read(), err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return read(), ctx.Err()
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return read(), errors.New("timed out")
	}
}

// waitForPrompt reads PTY output until a password prompt appears or the
// deadline passes.
func waitForPrompt(ptmx *os.File, d time.Duration) bool {
	_ = ptmx.SetReadDeadline(time.Now().Add(d))
	defer func() { _ = ptmx.SetReadDeadline(time.Time{}) }()
	buf := make([]byte, 256)
	var acc strings.Builder
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			acc.WriteString(strings.ToLower(string(buf[:n])))
			if strings.Contains(acc.String(), "assword") {
				return true
			}
		}
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return false
			}
			return false
		}
	}
}

func isCurrentUser(username string) bool {
	cur, err := user.Current()
	return err == nil && cur.Username == username
}

func detectShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "/bin/sh"
}

func withTerm(env []string) []string {
	for _, e := range env {
		if len(e) >= 5 && e[:5] == "TERM=" {
			return env
		}
	}
	return append(env, "TERM=xterm-256color")
}
