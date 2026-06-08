package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ayushkanoje/cronpilot/internal/auth"
)

type userView struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	MustChangePassword bool   `json:"mustChangePassword"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	sess, user, err := s.auth.Login(req.Username, req.Password, clientIP(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRateLimited):
			s.audit(r, "", req.Username, "login_rate_limited", "")
			writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		case errors.Is(err, auth.ErrInvalidCredentials):
			s.audit(r, "", req.Username, "login_failed", "")
			writeError(w, http.StatusUnauthorized, "invalid username or password")
		default:
			s.log.Error("login error", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	s.setSessionCookie(w, sess.Token)
	s.audit(r, user.ID, user.Username, "login", "")
	writeJSON(w, http.StatusOK, userView{ID: user.ID, Username: user.Username, MustChangePassword: user.MustChangePassword})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	user, sess, _ := auth.UserFrom(r.Context())
	if sess != nil {
		_ = s.auth.Logout(sess.Token)
	}
	s.clearSessionCookie(w)
	if user != nil {
		s.audit(r, user.ID, user.Username, "logout", "")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _, _ := auth.UserFrom(r.Context())
	writeJSON(w, http.StatusOK, userView{ID: user.ID, Username: user.Username, MustChangePassword: user.MustChangePassword})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, _, _ := auth.UserFrom(r.Context())
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.auth.ChangePassword(user.ID, req.OldPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(w, http.StatusBadRequest, "current password is incorrect")
		case errors.Is(err, auth.ErrWeakPassword):
			writeError(w, http.StatusBadRequest,
				"new password must be at least 12 characters and include at least 3 of: uppercase, lowercase, digit, symbol")
		default:
			s.log.Error("change password error", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// ChangePassword invalidates all sessions; issue a fresh one to keep the
	// user logged in.
	sess, err := s.auth.StartSession(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setSessionCookie(w, sess.Token)
	s.audit(r, user.ID, user.Username, "change_password", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
