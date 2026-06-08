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
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// The first tick (1s after priming) carries a real CPU measurement; the
	// client pre-seeds its window from it, so there is no early zero reading.
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sample, err := sampler.Sample()
			if err != nil {
				continue
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteJSON(sample) != nil {
				return
			}
		}
	}
}
