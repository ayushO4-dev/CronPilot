package server

import (
	"errors"
	"net/http"
	"strconv"

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
