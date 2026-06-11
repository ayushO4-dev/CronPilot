package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ayushkanoje/cronpilot/internal/auth"
	"github.com/ayushkanoje/cronpilot/internal/store"
)

type userView struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	MustChangePassword bool   `json:"mustChangePassword"`
	TOTPEnabled        bool   `json:"totpEnabled"`
}

func toUserView(u *store.User) userView {
	return userView{
		ID:                 u.ID,
		Username:           u.Username,
		MustChangePassword: u.MustChangePassword,
		TOTPEnabled:        u.TOTPEnabled,
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
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

	sess, user, err := s.auth.Login(req.Username, req.Password, req.Code, clientIP(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRateLimited):
			s.audit(r, "", req.Username, "login_rate_limited", "")
			writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		case errors.Is(err, auth.ErrTOTPRequired):
			// Password was valid; the client must now supply a 2FA code. The
			// totpRequired flag keeps the login form in code-entry mode.
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "two-factor code required", "totpRequired": true})
		case errors.Is(err, auth.ErrInvalidTOTP):
			s.audit(r, "", req.Username, "login_2fa_failed", "")
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "invalid two-factor code", "totpRequired": true})
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
	writeJSON(w, http.StatusOK, toUserView(user))
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
	writeJSON(w, http.StatusOK, toUserView(user))
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

// handleTOTPSetup generates a fresh (pending) secret and returns the secret,
// otpauth URL, and a scannable QR data URI. 2FA is not enforced until confirmed.
func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	user, _, _ := auth.UserFrom(r.Context())
	secret, url, err := s.auth.SetupTOTP(user.ID)
	if err != nil {
		s.log.Error("totp setup error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	qr, err := auth.QRCodeDataURI(url)
	if err != nil {
		s.log.Error("totp qr error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.audit(r, user.ID, user.Username, "totp_setup", "")
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "url": url, "qr": qr})
}

// handleTOTPEnable confirms a code against the pending secret and turns on 2FA.
func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	user, _, _ := auth.UserFrom(r.Context())
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.auth.EnableTOTP(user.ID, req.Code); err != nil {
		if errors.Is(err, auth.ErrInvalidTOTP) {
			writeError(w, http.StatusBadRequest, "invalid two-factor code")
			return
		}
		s.log.Error("totp enable error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.audit(r, user.ID, user.Username, "totp_enabled", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleTOTPDisable turns off 2FA after re-verifying the current password.
func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	user, _, _ := auth.UserFrom(r.Context())
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.auth.DisableTOTP(user.ID, req.Password); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusBadRequest, "current password is incorrect")
			return
		}
		s.log.Error("totp disable error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.audit(r, user.ID, user.Username, "totp_disabled", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
