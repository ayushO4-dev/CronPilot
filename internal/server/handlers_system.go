package server

import (
	"context"
	"net/http"
	"time"

	"github.com/ayushkanoje/cronpilot/internal/system"
)

func (s *Server) handleSystemSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := system.Collect()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to collect system metrics")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// handleSystemStream upgrades to a WebSocket and pushes a metrics Sample roughly
// every 1.5s until the client disconnects.
func (s *Server) handleSystemStream(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Reader goroutine: notice client close.
	go func() {
		conn.SetReadLimit(512)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	sampler := system.NewSampler()
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	send := func() bool {
		sample, err := sampler.Sample()
		if err != nil {
			return true
		}
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(sample) == nil
	}

	// Send one immediately so the UI populates without waiting a full tick.
	if !send() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}
