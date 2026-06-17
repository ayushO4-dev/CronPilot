// Package config loads CronPilot daemon configuration from environment
// variables, with command-line flags taking precedence.
package config

import (
	"flag"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the daemon.
type Config struct {
	// Addr is the listen address, e.g. "127.0.0.1:8765". Defaults to loopback
	// so the agent is not world-exposed without an explicit choice.
	Addr string
	// DataDir holds the SQLite database and other persistent state.
	DataDir string
	// DBPath is derived from DataDir.
	DBPath string
	// Dev relaxes the Secure cookie flag and enables permissive CORS so the
	// Vite dev server can talk to the API. Never enable in production.
	Dev bool
	// ShellPath overrides the shell spawned for the web terminal. Empty means
	// "use the login user's shell, falling back to /bin/bash then /bin/sh".
	ShellPath string
	// SessionIdle is the inactivity timeout for a session.
	SessionIdle time.Duration
	// SessionMax is the absolute maximum lifetime of a session.
	SessionMax time.Duration
	// TLSCert / TLSKey enable built-in HTTPS when both are set.
	TLSCert string
	TLSKey  string
}

// Load reads configuration from the environment and flags. Flags win over env.
func Load() *Config {
	c := &Config{
		Addr:        env("CRONPILOT_ADDR", "127.0.0.1:8765"),
		DataDir:     env("CRONPILOT_DATA_DIR", defaultDataDir()),
		Dev:         envBool("CRONPILOT_DEV", false),
		ShellPath:   env("CRONPILOT_SHELL", ""),
		SessionIdle: envDuration("CRONPILOT_SESSION_IDLE", 2*time.Hour),
		SessionMax:  envDuration("CRONPILOT_SESSION_MAX", 24*time.Hour),
		TLSCert:     env("CRONPILOT_TLS_CERT", ""),
		TLSKey:      env("CRONPILOT_TLS_KEY", ""),
	}

	flag.StringVar(&c.Addr, "addr", c.Addr, "listen address (host:port)")
	flag.StringVar(&c.DataDir, "data-dir", c.DataDir, "directory for the database and state")
	flag.BoolVar(&c.Dev, "dev", c.Dev, "development mode (relaxes Secure cookies, enables CORS)")
	flag.StringVar(&c.ShellPath, "shell", c.ShellPath, "shell to spawn for the web terminal")
	flag.StringVar(&c.TLSCert, "tls-cert", c.TLSCert, "path to TLS certificate (enables HTTPS with -tls-key)")
	flag.StringVar(&c.TLSKey, "tls-key", c.TLSKey, "path to TLS private key")
	flag.Parse()

	c.DBPath = filepath.Join(c.DataDir, "cronpilot.db")
	return c
}

// TLSEnabled reports whether built-in HTTPS should be used.
func (c *Config) TLSEnabled() bool { return c.TLSCert != "" && c.TLSKey != "" }

// LoopbackOnly reports whether Addr binds only the loopback interface. An empty
// or wildcard host ("", "0.0.0.0", "::") is treated as non-loopback (exposed).
func (c *Config) LoopbackOnly() bool {
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil {
		host = c.Addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func defaultDataDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "cronpilot")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "cronpilot")
	}
	return filepath.Join(".", "data")
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
