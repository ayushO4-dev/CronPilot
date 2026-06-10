package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
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

// handleTerminal upgrades to a WebSocket and bridges it to a PTY-backed shell.
// Protocol: client text frames are JSON control messages ({type:"input"|"resize"});
// server frames are raw shell output (binary).
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	user, _, _ := auth.UserFrom(r.Context())

	cols := parseDim(r.URL.Query().Get("cols"), 80)
	rows := parseDim(r.URL.Query().Get("rows"), 24)

	// Resolve the one-time ticket to an account; no ticket means the daemon's
	// own account.
	shellUser, shellPassword := "", ""
	if ticket := r.URL.Query().Get("ticket"); ticket != "" {
		s.termMu.Lock()
		tk, ok := s.termTickets[ticket]
		delete(s.termTickets, ticket)
		s.termMu.Unlock()
		if !ok || time.Now().After(tk.exp) {
			writeError(w, http.StatusForbidden, "invalid or expired terminal ticket")
			return
		}
		shellUser, shellPassword = tk.user, tk.password
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var sess *terminal.Session
	if shellUser == "" {
		sess, err = terminal.Start(s.cfg.ShellPath, cols, rows)
	} else {
		sess, err = terminal.StartUser(shellUser, shellPassword, s.cfg.ShellPath, cols, rows)
	}
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to start shell: "+err.Error()))
		return
	}
	defer sess.Close()

	if user != nil {
		s.audit(r, user.ID, user.Username, "terminal_open", shellUser)
	}

	// PTY output -> client.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := sess.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				conn.Close()
				return
			}
		}
	}()

	// Client -> PTY.
	type clientMsg struct {
		Type string `json:"type"`
		Data string `json:"data"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.TextMessage:
			var msg clientMsg
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "input":
				_, _ = sess.Write([]byte(msg.Data))
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					_ = sess.Resize(msg.Cols, msg.Rows)
				}
			}
		case websocket.BinaryMessage:
			_, _ = sess.Write(data)
		}
	}
}

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
