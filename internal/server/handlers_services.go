package server

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ayushkanoje/cronpilot/internal/auth"
	"github.com/ayushkanoje/cronpilot/internal/services"
)

func (s *Server) handleServicesList(w http.ResponseWriter, r *http.Request) {
	units, err := services.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list services: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, units)
}

func (s *Server) handleServiceGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !services.ValidUnitName(name) {
		writeError(w, http.StatusBadRequest, "invalid unit name")
		return
	}
	d, err := services.Get(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !services.ValidUnitName(name) {
		writeError(w, http.StatusBadRequest, "invalid unit name")
		return
	}
	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			lines = n
		}
	}
	logs, err := services.Logs(r.Context(), name, lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": logs})
}

func (s *Server) handleServiceFileGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !services.ValidUnitName(name) {
		writeError(w, http.StatusBadRequest, "invalid unit name")
		return
	}
	path, content, writable, err := services.ReadUnitFile(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "content": content, "writable": writable})
}

func (s *Server) handleServiceFilePut(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !services.ValidUnitName(name) {
		writeError(w, http.StatusBadRequest, "invalid unit name")
		return
	}
	var req struct {
		Content  string `json:"content"`
		Reload   bool   `json:"reload"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, _, _ := auth.UserFrom(r.Context())
	path, err := services.WriteUnitFile(r.Context(), name, req.Content, req.Reload, req.Password)
	s.audit(r, user.ID, user.Username, "service_edit_unit", name)
	if err != nil {
		if errors.Is(err, services.ErrAuth) {
			writeError(w, http.StatusUnauthorized,
				"sudo rejected the password for the account the server runs as ("+services.DaemonUser()+")")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func (s *Server) handleServiceSudoCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := services.VerifyRoot(r.Context(), req.Password)
	if err != nil {
		who := services.DaemonUser()
		s.log.Warn("service sudo-check failed",
			"daemon_user", who, "euid", os.Geteuid(), "sudo_output", strings.TrimSpace(out))
		if errors.Is(err, services.ErrAuth) {
			// Name the account so it's obvious whose password sudo wants — if it
			// says e.g. "cronpilot" (a passwordless system account), that, not a
			// typo, is the problem.
			writeError(w, http.StatusUnauthorized,
				"sudo rejected the password for the account the server runs as ("+who+")")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	action := r.PathValue("action")
	user, _, _ := auth.UserFrom(r.Context())

	err := services.Action(r.Context(), name, action)
	s.audit(r, user.ID, user.Username, "service_"+action, name)

	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidName):
			writeError(w, http.StatusBadRequest, "invalid unit name")
		case errors.Is(err, services.ErrInvalidAction):
			writeError(w, http.StatusBadRequest, "invalid action")
		default:
			// e.g. permission denied (sudo needs a password) or unit failed.
			writeError(w, http.StatusBadGateway, err.Error())
		}
		return
	}

	// Return the refreshed unit so the UI updates immediately.
	d, _ := services.Get(r.Context(), name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unit": d})
}
