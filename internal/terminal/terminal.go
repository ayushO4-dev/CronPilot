// Package terminal spawns interactive shell sessions backed by a PTY. Each web
// terminal connection owns one Session and closes it on disconnect, so there is
// no shared global shell.
package terminal

import (
	"os"
	"os/exec"

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
