// Command cronpilotd is the CronPilot daemon: a single binary that serves the
// embedded web UI and the management API for the local Linux host.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	cronpilot "github.com/ayushkanoje/cronpilot"
	"github.com/ayushkanoje/cronpilot/internal/auth"
	"github.com/ayushkanoje/cronpilot/internal/config"
	"github.com/ayushkanoje/cronpilot/internal/server"
	"github.com/ayushkanoje/cronpilot/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		log.Error("create data dir", "dir", cfg.DataDir, "err", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := server.EnsureAdmin(st, log); err != nil {
		log.Error("ensure admin account", "err", err)
		os.Exit(1)
	}

	webFS, err := cronpilot.WebFS()
	if err != nil {
		log.Error("load embedded web assets", "err", err)
		os.Exit(1)
	}

	am := auth.NewManager(st, cfg)
	srv := server.New(cfg, st, am, webFS, log)

	// Background janitor: purge expired sessions.
	stopJanitor := make(chan struct{})
	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-stopJanitor:
				return
			case <-t.C:
				if n, err := st.DeleteExpiredSessions(time.Now()); err == nil && n > 0 {
					log.Info("purged expired sessions", "count", n)
				}
			}
		}
	}()

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		scheme := "http"
		if cfg.TLSEnabled() {
			scheme = "https"
		}
		log.Info("CronPilot listening",
			"url", scheme+"://"+cfg.Addr, "dev", cfg.Dev, "version", server.Version)
		var serveErr error
		if cfg.TLSEnabled() {
			serveErr = httpServer.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			serveErr = httpServer.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error("server error", "err", serveErr)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Info("shutting down")
	close(stopJanitor)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}
