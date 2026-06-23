// Package server wires the HTTP/WebSocket API, authentication middleware, and
// the embedded single-page app into one http.Handler.
package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ayushkanoje/cronpilot/internal/auth"
	"github.com/ayushkanoje/cronpilot/internal/config"
	"github.com/ayushkanoje/cronpilot/internal/processes"
	"github.com/ayushkanoje/cronpilot/internal/store"
	"github.com/ayushkanoje/cronpilot/internal/tasks"
)

// Version is the daemon version, surfaced in settings/health.
const Version = "0.2.4"

// termTicket is a short-lived, one-time grant to open a terminal as a given
// account (issued by handleTerminalSession after password verification).
type termTicket struct {
	user     string
	password string
	exp      time.Time
}

// Server holds shared dependencies for the HTTP handlers.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	auth     *auth.Manager
	webFS    fs.FS
	log      *slog.Logger
	upgrader websocket.Upgrader
	procs    *processes.Sampler
	tasks    *tasks.Manager

	termMu      sync.Mutex
	termTickets map[string]termTicket
	termLive    map[string]*liveTerm // persistent shells keyed by app user id

	upd updateState // in-progress self-update state
}

// New constructs a Server.
func New(cfg *config.Config, st *store.Store, am *auth.Manager, tm *tasks.Manager, webFS fs.FS, log *slog.Logger) *Server {
	s := &Server{
		cfg: cfg, store: st, auth: am, tasks: tm, webFS: webFS, log: log,
		procs:       processes.NewSampler(),
		termTickets: map[string]termTicket{},
		termLive:    map[string]*liveTerm{},
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     s.checkWSOrigin,
	}
	return s
}

// Handler returns the fully-wrapped HTTP handler (routes + middleware).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Auth
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/auth/me", s.requireAuth(s.handleMe))
	mux.HandleFunc("POST /api/auth/change-password", s.requireAuth(s.handleChangePassword))
	mux.HandleFunc("POST /api/auth/totp/setup", s.requireAuth(s.handleTOTPSetup))
	mux.HandleFunc("POST /api/auth/totp/enable", s.requireAuth(s.handleTOTPEnable))
	mux.HandleFunc("POST /api/auth/totp/disable", s.requireAuth(s.handleTOTPDisable))

	// System monitor
	mux.HandleFunc("GET /api/system/summary", s.requireAuth(s.handleSystemSummary))
	mux.HandleFunc("GET /api/system/stream", s.requireAuth(s.handleSystemStream))

	// Terminal
	mux.HandleFunc("GET /api/terminal", s.requireAuth(s.handleTerminal))
	mux.HandleFunc("GET /api/terminal/users", s.requireAuth(s.handleTerminalUsers))
	mux.HandleFunc("POST /api/terminal/session", s.requireAuth(s.handleTerminalSession))

	// Services (systemd)
	mux.HandleFunc("GET /api/services", s.requireAuth(s.handleServicesList))
	mux.HandleFunc("GET /api/services/{name}", s.requireAuth(s.handleServiceGet))
	mux.HandleFunc("GET /api/services/{name}/logs", s.requireAuth(s.handleServiceLogs))
	mux.HandleFunc("GET /api/services/{name}/file", s.requireAuth(s.handleServiceFileGet))
	mux.HandleFunc("PUT /api/services/{name}/file", s.requireAuth(s.handleServiceFilePut))
	mux.HandleFunc("POST /api/services/{name}/sudo-check", s.requireAuth(s.handleServiceSudoCheck))
	mux.HandleFunc("POST /api/services/{name}/{action}", s.requireAuth(s.handleServiceAction))

	// Processes (running applications)
	mux.HandleFunc("GET /api/processes", s.requireAuth(s.handleProcessesList))
	mux.HandleFunc("GET /api/processes/{pid}", s.requireAuth(s.handleProcessGet))
	mux.HandleFunc("POST /api/processes/{pid}/{signal}", s.requireAuth(s.handleProcessSignal))

	// Tasks (ladder-logic automation)
	mux.HandleFunc("GET /api/tasks", s.requireAuth(s.handleTasksList))
	mux.HandleFunc("POST /api/tasks", s.requireAuth(s.handleTaskCreate))
	mux.HandleFunc("GET /api/tasks/{id}", s.requireAuth(s.handleTaskGet))
	mux.HandleFunc("PUT /api/tasks/{id}", s.requireAuth(s.handleTaskUpdate))
	mux.HandleFunc("DELETE /api/tasks/{id}", s.requireAuth(s.handleTaskDelete))
	mux.HandleFunc("GET /api/tasks/{id}/runs", s.requireAuth(s.handleTaskRuns))
	mux.HandleFunc("POST /api/tasks/{id}/{action}", s.requireAuth(s.handleTaskAction))

	// Settings
	mux.HandleFunc("GET /api/settings", s.requireAuth(s.handleGetSettings))
	mux.HandleFunc("PUT /api/settings", s.requireAuth(s.handlePutSettings))

	// Self-update (status is unauthenticated so the post-logout "updating"
	// screen can keep polling progress through the restart).
	mux.HandleFunc("GET /api/update/check", s.requireAuth(s.handleUpdateCheck))
	mux.HandleFunc("POST /api/update/apply", s.requireAuth(s.handleUpdateApply))

	// Health + update status (unauthenticated)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/update/status", s.handleUpdateStatus)

	// SPA + static assets (catch-all)
	mux.Handle("/", s.spaHandler())

	// Outer-to-inner: recover, security headers, logging, CSRF origin check.
	return s.recoverMW(s.securityHeadersMW(s.loggingMW(s.originCheckMW(mux))))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": Version})
}

// EnsureAdmin creates an initial admin account on first run. The password comes
// from CRONPILOT_ADMIN_PASSWORD if set, otherwise a strong random one is
// generated and logged (with the must-change flag set).
func EnsureAdmin(st *store.Store, log *slog.Logger) error {
	n, err := st.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	username := os.Getenv("CRONPILOT_ADMIN_USER")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("CRONPILOT_ADMIN_PASSWORD")
	generated := password == ""
	if generated {
		password = generatePassword()
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	if err := st.CreateUser(&store.User{
		ID:                 id,
		Username:           username,
		PasswordHash:       hash,
		MustChangePassword: generated,
	}); err != nil {
		return err
	}

	if generated {
		log.Warn("════════ initial admin account created ════════")
		log.Warn("login with these credentials, then change the password immediately",
			"username", username, "password", password)
		log.Warn("═══════════════════════════════════════════════")
	} else {
		log.Info("initial admin account created from environment", "username", username)
	}
	return nil
}

// --- small helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) audit(r *http.Request, userID, username, action, detail string) {
	if err := s.store.AddAudit(store.AuditEntry{
		UserID:   userID,
		Username: username,
		Action:   action,
		Detail:   detail,
		IP:       clientIP(r),
	}); err != nil {
		s.log.Error("audit write failed", "action", action, "err", err)
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   !s.cfg.Dev,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   !s.cfg.Dev,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generatePassword() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		// extremely unlikely; fall back to a fixed-but-must-change value
		return "ChangeMe!123456"
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
