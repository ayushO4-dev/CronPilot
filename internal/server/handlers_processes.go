package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ayushkanoje/cronpilot/internal/auth"
	"github.com/ayushkanoje/cronpilot/internal/processes"
)

func (s *Server) handleProcessesList(w http.ResponseWriter, r *http.Request) {
	list, err := s.procs.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list processes: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleProcessGet(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 0 {
		writeError(w, http.StatusBadRequest, "invalid pid")
		return
	}
	d, err := processes.GetDetail(r.Context(), int32(pid))
	if err != nil {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleProcessSignal(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 0 {
		writeError(w, http.StatusBadRequest, "invalid pid")
		return
	}
	sig := r.PathValue("signal")
	user, _, _ := auth.UserFrom(r.Context())

	err = processes.Signal(r.Context(), pid, sig)
	s.audit(r, user.ID, user.Username, "process_signal_"+strings.ToLower(sig), strconv.Itoa(pid))

	if err != nil {
		switch {
		case errors.Is(err, processes.ErrInvalidPID):
			writeError(w, http.StatusBadRequest, "invalid pid")
		case errors.Is(err, processes.ErrInvalidSignal):
			writeError(w, http.StatusBadRequest, "invalid signal")
		default:
			writeError(w, http.StatusBadGateway, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
