package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/ayushkanoje/cronpilot/internal/auth"
	"github.com/ayushkanoje/cronpilot/internal/update"
)

// updateState tracks an in-progress self-update for the status endpoint.
type updateState struct {
	mu         sync.Mutex
	running    bool
	state      string // idle|downloading|applying|restarting|error
	downloaded int64
	total      int64
	latest     string
	errMsg     string
}

func (u *updateState) snapshot() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	state := u.state
	if state == "" {
		state = "idle"
	}
	return map[string]any{
		"state":      state,
		"downloaded": u.downloaded,
		"total":      u.total,
		"latest":     u.latest,
		"error":      u.errMsg,
	}
}

func (u *updateState) begin(latest string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.running {
		return false
	}
	u.running, u.state, u.downloaded, u.total, u.latest, u.errMsg = true, "downloading", 0, 0, latest, ""
	return true
}

func (u *updateState) progress(done, total int64) {
	u.mu.Lock()
	u.downloaded, u.total = done, total
	u.mu.Unlock()
}

func (u *updateState) set(state string) {
	u.mu.Lock()
	u.state = state
	u.mu.Unlock()
}

func (u *updateState) fail(msg string) {
	u.mu.Lock()
	u.running, u.state, u.errMsg = false, "error", msg
	u.mu.Unlock()
}

// handleUpdateCheck reports whether a newer release is available.
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	res, _, err := update.Check(ctx, Version)
	if err != nil {
		s.log.Warn("update check failed", "err", err)
		writeError(w, http.StatusBadGateway, "could not check for updates: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleUpdateStatus returns the current self-update progress.
func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.upd.snapshot())
}

// handleUpdateApply downloads the latest release, swaps in the new binary, and
// restarts the daemon. It returns immediately; clients poll the status endpoint.
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	user, _, _ := auth.UserFrom(r.Context())

	// Resolve the release first so we can fail fast with a clear error.
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	res, rel, err := update.Check(ctx, Version)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not check for updates: "+err.Error())
		return
	}
	dir, err := update.TargetDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot locate executable: "+err.Error())
		return
	}
	if !s.upd.begin(res.Latest) {
		writeError(w, http.StatusConflict, "an update is already in progress")
		return
	}
	if user != nil {
		s.audit(r, user.ID, user.Username, "update_apply", res.Latest)
	}

	go func() {
		dctx, dcancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer dcancel()
		tmp, err := rel.Download(dctx, dir, s.upd.progress)
		if err != nil {
			s.log.Error("update download", "err", err)
			s.upd.fail(err.Error())
			return
		}
		s.upd.set("applying")
		if err := update.Apply(tmp); err != nil {
			s.log.Error("update apply", "err", err)
			s.upd.fail(err.Error())
			return
		}
		s.log.Warn("update applied — restarting", "version", res.Latest)
		s.upd.set("restarting")
		// Give clients a moment to observe "restarting" and log out before the
		// process is replaced.
		time.Sleep(1500 * time.Millisecond)
		update.Restart()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"state": "downloading", "latest": res.Latest})
}
