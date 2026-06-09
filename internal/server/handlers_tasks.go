package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ayushkanoje/cronpilot/internal/auth"
	"github.com/ayushkanoje/cronpilot/internal/store"
	"github.com/ayushkanoje/cronpilot/internal/tasks"
)

func (s *Server) handleTasksList(w http.ResponseWriter, r *http.Request) {
	list, err := s.tasks.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks: "+err.Error())
		return
	}
	if list == nil {
		list = []*tasks.Task{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	t, err := s.tasks.Get(r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "task not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	var t tasks.Task
	if err := decodeJSON(r, &t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.tasks.Create(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid task: "+err.Error())
		return
	}
	user, _, _ := auth.UserFrom(r.Context())
	s.audit(r, user.ID, user.Username, "task_create", t.Name)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	var t tasks.Task
	if err := decodeJSON(r, &t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	t.ID = r.PathValue("id")
	if err := s.tasks.Update(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid task: "+err.Error())
		return
	}
	user, _, _ := auth.UserFrom(r.Context())
	s.audit(r, user.ID, user.Username, "task_update", t.Name)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.tasks.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	user, _, _ := auth.UserFrom(r.Context())
	s.audit(r, user.ID, user.Username, "task_delete", id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTaskRuns(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	runs, err := s.tasks.Runs(r.PathValue("id"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if runs == nil {
		runs = []store.TaskRun{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action := r.PathValue("action")
	user, _, _ := auth.UserFrom(r.Context())

	switch action {
	case "run":
		run, err := s.tasks.RunNow(id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "task not found")
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		s.audit(r, user.ID, user.Username, "task_run", id)
		writeJSON(w, http.StatusOK, run)
	case "enable", "disable":
		if err := s.tasks.SetEnabled(id, action == "enable"); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.audit(r, user.ID, user.Username, "task_"+action, id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusBadRequest, "unknown action")
	}
}
