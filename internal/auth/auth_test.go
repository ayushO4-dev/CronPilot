package auth

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ayushkanoje/cronpilot/internal/config"
	"github.com/ayushkanoje/cronpilot/internal/store"
)

func newTestManager(t *testing.T) (*Manager, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := &config.Config{SessionIdle: time.Hour, SessionMax: 24 * time.Hour}
	return NewManager(st, cfg), st
}

func createUser(t *testing.T, st *store.Store, username, pw string) *store.User {
	t.Helper()
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := &store.User{ID: "u-" + username, Username: username, PasswordHash: hash}
	if err := st.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func TestLoginAndAuthenticate(t *testing.T) {
	m, st := newTestManager(t)
	createUser(t, st, "alice", "Sup3rSecret!pw")

	sess, user, err := m.Login("alice", "Sup3rSecret!pw", "127.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if user.Username != "alice" {
		t.Fatalf("unexpected user %q", user.Username)
	}

	got, _, err := m.Authenticate(sess.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.ID != user.ID {
		t.Fatalf("authenticate returned wrong user")
	}

	if _, _, err := m.Login("alice", "nope", "127.0.0.1"); err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}

func TestUnknownUserLogin(t *testing.T) {
	m, _ := newTestManager(t)
	if _, _, err := m.Login("ghost", "whatever", "127.0.0.1"); err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials for unknown user, got %v", err)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	m, st := newTestManager(t)
	createUser(t, st, "bob", "Sup3rSecret!pw")
	sess, _, _ := m.Login("bob", "Sup3rSecret!pw", "ip")

	if err := m.Logout(sess.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, err := m.Authenticate(sess.Token); err == nil {
		t.Fatal("expected session invalid after logout")
	}
}

func TestRateLimit(t *testing.T) {
	m, st := newTestManager(t)
	createUser(t, st, "carol", "Sup3rSecret!pw")
	const ip = "10.0.0.1"

	for i := 0; i < 5; i++ {
		_, _, _ = m.Login("carol", "wrong", ip)
	}
	// Even with the correct password, the bucket is now locked.
	if _, _, err := m.Login("carol", "Sup3rSecret!pw", ip); err != ErrRateLimited {
		t.Fatalf("expected rate limited, got %v", err)
	}
	// A different IP is unaffected.
	if _, _, err := m.Login("carol", "Sup3rSecret!pw", "10.0.0.2"); err != nil {
		t.Fatalf("different IP should not be limited, got %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	m, st := newTestManager(t)
	u := createUser(t, st, "dave", "OldPassw0rd!")

	if err := m.ChangePassword(u.ID, "OldPassw0rd!", "N3wPassword!x"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, _, err := m.Login("dave", "N3wPassword!x", "ip"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}

	u2 := createUser(t, st, "erin", "OldPassw0rd!")
	if err := m.ChangePassword(u2.ID, "OldPassw0rd!", "weak"); err != ErrWeakPassword {
		t.Fatalf("expected weak password rejection, got %v", err)
	}
	if err := m.ChangePassword(u2.ID, "wrong-old", "N3wPassword!x"); err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials for wrong old password, got %v", err)
	}
}
