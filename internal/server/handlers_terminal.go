package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ayushkanoje/cronpilot/internal/auth"
	"github.com/ayushkanoje/cronpilot/internal/terminal"
)

// handleTerminalUsers lists the system accounts offered by the login picker.
func (s *Server) handleTerminalUsers(w http.ResponseWriter, r *http.Request) {
	users, err := terminal.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// handleTerminalSession validates an account choice (verifying the password
// when one is supplied — mandatory for root) and issues a short-lived one-time
// ticket that the terminal WebSocket exchanges for a shell.
func (s *Server) handleTerminalSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.User = strings.TrimSpace(req.User)

	users, err := terminal.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	known := false
	for _, u := range users {
		if u.Name == req.User {
			known = true
			break
		}
	}
	if !known {
		writeError(w, http.StatusBadRequest, "unknown user")
		return
	}
	if req.User == "root" && req.Password == "" {
		writeError(w, http.StatusBadRequest, "root requires a password")
		return
	}
	if req.Password != "" && !terminal.VerifyPassword(req.User, req.Password) {
		user, _, _ := auth.UserFrom(r.Context())
		s.audit(r, user.ID, user.Username, "terminal_auth_failed", req.User)
		writeError(w, http.StatusUnauthorized, "authentication failed")
		return
	}

	ticket, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.termMu.Lock()
	now := time.Now()
	for t, tk := range s.termTickets {
		if now.After(tk.exp) {
			delete(s.termTickets, t)
		}
	}
	s.termTickets[ticket] = termTicket{user: req.User, password: req.Password, exp: now.Add(30 * time.Second)}
	s.termMu.Unlock()

	user, _, _ := auth.UserFrom(r.Context())
	s.audit(r, user.ID, user.Username, "terminal_session", req.User)
	writeJSON(w, http.StatusOK, map[string]string{"ticket": ticket})
}

// handleTerminal upgrades to a WebSocket and bridges it to a persistent,
// PTY-backed shell. One shell is kept alive per logged-in user across tab
// switches and brief disconnects: a connection carrying a one-time ticket starts
// a fresh shell (replacing any existing one); a connection without a ticket
// reattaches to the running shell and replays its recent output. The shell ends
// only when it exits (the user typed `exit`) or the user logs out.
//
// Protocol: client text frames are JSON control messages ({type:"input"|"resize"});
// server frames are raw shell output (binary), plus a final {"type":"ended"} text
// frame when the shell itself exits.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	user, _, _ := auth.UserFrom(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	owner := user.ID
	cols := parseDim(r.URL.Query().Get("cols"), 80)
	rows := parseDim(r.URL.Query().Get("rows"), 24)

	// Resolve an optional one-time ticket to an account + password.
	shellUser, shellPassword, haveTicket := "", "", false
	if ticket := r.URL.Query().Get("ticket"); ticket != "" {
		s.termMu.Lock()
		tk, ok := s.termTickets[ticket]
		delete(s.termTickets, ticket)
		s.termMu.Unlock()
		if !ok || time.Now().After(tk.exp) {
			writeError(w, http.StatusForbidden, "invalid or expired terminal ticket")
			return
		}
		shellUser, shellPassword, haveTicket = tk.user, tk.password, true
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Find or create the live session for this user. A ticket always starts a
	// fresh shell; otherwise reuse a running one (reattach) or start a default.
	s.termMu.Lock()
	lt := s.termLive[owner]
	if haveTicket || lt == nil || lt.isEnded() {
		if lt != nil {
			lt.close()
		}
		var sess *terminal.Session
		if shellUser == "" {
			sess, err = terminal.Start(s.cfg.ShellPath, cols, rows)
		} else {
			sess, err = terminal.StartUser(shellUser, shellPassword, s.cfg.ShellPath, cols, rows)
		}
		if err != nil {
			s.termMu.Unlock()
			_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to start shell: "+err.Error()))
			return
		}
		lt = newLiveTerm(sess, shellUser)
		s.termLive[owner] = lt
		go func() { // reap from the registry when the shell exits
			lt.wait()
			s.termMu.Lock()
			if s.termLive[owner] == lt {
				delete(s.termLive, owner)
			}
			s.termMu.Unlock()
		}()
		s.audit(r, user.ID, user.Username, "terminal_open", shellUser)
	}
	s.termMu.Unlock()

	_ = lt.sess.Resize(cols, rows)

	// Attach: snapshot output so far, then receive new output via send. A single
	// writer goroutine owns all writes to conn.
	send := make(chan []byte, 256)
	stop := make(chan struct{})
	replay, ended := lt.attach(send)
	go func() {
		if len(replay) > 0 {
			if conn.WriteMessage(websocket.BinaryMessage, replay) != nil {
				return
			}
		}
		if ended {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ended"}`))
			return
		}
		for {
			select {
			case chunk, ok := <-send:
				if !ok { // pump closed it: the shell exited
					_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ended"}`))
					return
				}
				if conn.WriteMessage(websocket.BinaryMessage, chunk) != nil {
					return
				}
			case <-stop:
				return
			}
		}
	}()

	// Reader loop: client -> PTY, until the client disconnects.
	type clientMsg struct {
		Type string `json:"type"`
		Data string `json:"data"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch mt {
		case websocket.TextMessage:
			var msg clientMsg
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "input":
				_, _ = lt.sess.Write([]byte(msg.Data))
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					_ = lt.sess.Resize(msg.Cols, msg.Rows)
				}
			}
		case websocket.BinaryMessage:
			_, _ = lt.sess.Write(data)
		}
	}
	// Client gone: stop our writer and detach. The shell keeps running.
	close(stop)
	lt.detach(send)
}

// closeUserTerminal terminates a user's persistent shell (called on logout).
func (s *Server) closeUserTerminal(userID string) {
	s.termMu.Lock()
	lt := s.termLive[userID]
	delete(s.termLive, userID)
	s.termMu.Unlock()
	if lt != nil {
		lt.close()
	}
}

// termRingCap bounds the per-session output replay buffer.
const termRingCap = 256 << 10

// liveTerm is a PTY-backed shell that outlives individual WebSocket
// connections. A single pump goroutine reads the PTY into a ring buffer and
// forwards new output to the currently-attached connection (if any).
type liveTerm struct {
	sess *terminal.Session
	user string

	mu    sync.Mutex
	ring  []byte
	send  chan []byte // current subscriber, or nil when detached
	ended bool
	done  chan struct{}
}

func newLiveTerm(sess *terminal.Session, user string) *liveTerm {
	lt := &liveTerm{sess: sess, user: user, done: make(chan struct{})}
	go lt.pump()
	return lt
}

func (lt *liveTerm) pump() {
	buf := make([]byte, 4096)
	for {
		n, err := lt.sess.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			lt.mu.Lock()
			lt.ring = append(lt.ring, chunk...)
			if len(lt.ring) > termRingCap {
				lt.ring = lt.ring[len(lt.ring)-termRingCap:]
			}
			if lt.send != nil {
				select {
				case lt.send <- chunk:
				default: // slow client; terminal output is best-effort
				}
			}
			lt.mu.Unlock()
		}
		if err != nil {
			lt.mu.Lock()
			lt.ended = true
			if lt.send != nil {
				close(lt.send)
				lt.send = nil
			}
			lt.mu.Unlock()
			_ = lt.sess.Close()
			close(lt.done)
			return
		}
	}
}

// attach registers ch for new output and returns a snapshot of output so far
// plus whether the shell has already ended.
func (lt *liveTerm) attach(ch chan []byte) ([]byte, bool) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	snap := make([]byte, len(lt.ring))
	copy(snap, lt.ring)
	if lt.ended {
		return snap, true
	}
	lt.send = ch
	return snap, false
}

func (lt *liveTerm) detach(ch chan []byte) {
	lt.mu.Lock()
	if lt.send == ch {
		lt.send = nil
	}
	lt.mu.Unlock()
}

func (lt *liveTerm) isEnded() bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return lt.ended
}

func (lt *liveTerm) wait() { <-lt.done }

// close terminates the shell; the pump observes EOF and marks the session ended.
func (lt *liveTerm) close() { _ = lt.sess.Close() }

func parseDim(s string, def uint16) uint16 {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 1000 {
		return def
	}
	return uint16(n)
}
