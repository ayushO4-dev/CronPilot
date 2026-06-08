package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/ayushkanoje/cronpilot/internal/config"
	"github.com/ayushkanoje/cronpilot/internal/store"
)

// SessionCookieName is the cookie carrying the opaque session token.
const SessionCookieName = "cronpilot_session"

var (
	// ErrInvalidCredentials is returned for a bad username/password.
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrRateLimited is returned when login attempts are temporarily locked.
	ErrRateLimited = errors.New("too many attempts, try again later")
	// ErrWeakPassword is returned when a new password fails the policy.
	ErrWeakPassword = errors.New("password does not meet strength requirements")
)

// dummyHash is verified against when a username is unknown, to flatten the
// timing difference between "no such user" and "wrong password".
var dummyHash string

func init() {
	if h, err := HashPassword("cronpilot-timing-equalizer"); err == nil {
		dummyHash = h
	}
}

// Manager coordinates authentication against the store.
type Manager struct {
	store *store.Store
	cfg   *config.Config
	lim   *limiter
}

// NewManager builds a Manager. Login is locked for the window after maxAttempts.
func NewManager(st *store.Store, cfg *config.Config) *Manager {
	return &Manager{store: st, cfg: cfg, lim: newLimiter(5, 15*time.Minute)}
}

// Login validates credentials and starts a session. key (usually the client IP)
// is the rate-limit bucket.
func (m *Manager) Login(username, password, key string) (*store.Session, *store.User, error) {
	if !m.lim.allow(key) {
		return nil, nil, ErrRateLimited
	}
	user, err := m.store.GetUserByUsername(username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_, _ = VerifyPassword(dummyHash, password) // equalize timing
			m.lim.fail(key)
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}
	ok, err := VerifyPassword(user.PasswordHash, password)
	if err != nil || !ok {
		m.lim.fail(key)
		return nil, nil, ErrInvalidCredentials
	}
	m.lim.reset(key)
	sess, err := m.StartSession(user.ID)
	if err != nil {
		return nil, nil, err
	}
	return sess, user, nil
}

// StartSession creates and persists a fresh session for a user.
func (m *Manager) StartSession(userID string) (*store.Session, error) {
	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &store.Session{
		Token:     token,
		UserID:    userID,
		CreatedAt: now,
		LastSeen:  now,
		ExpiresAt: now.Add(m.cfg.SessionIdle),
	}
	if err := m.store.CreateSession(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Authenticate validates a token, enforces idle/absolute expiry, slides the
// idle window, and returns the owning user.
func (m *Manager) Authenticate(token string) (*store.User, *store.Session, error) {
	sess, err := m.store.GetSession(token)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	if now.After(sess.ExpiresAt) || now.Sub(sess.CreatedAt) > m.cfg.SessionMax {
		_ = m.store.DeleteSession(token)
		return nil, nil, store.ErrNotFound
	}
	user, err := m.store.GetUserByID(sess.UserID)
	if err != nil {
		return nil, nil, err
	}
	newExpiry := now.Add(m.cfg.SessionIdle)
	if hardMax := sess.CreatedAt.Add(m.cfg.SessionMax); newExpiry.After(hardMax) {
		newExpiry = hardMax
	}
	sess.LastSeen = now
	sess.ExpiresAt = newExpiry
	_ = m.store.TouchSession(token, now, newExpiry)
	return user, sess, nil
}

// Logout deletes a single session.
func (m *Manager) Logout(token string) error {
	return m.store.DeleteSession(token)
}

// ChangePassword verifies the current password, applies the new one, and
// invalidates all of the user's sessions.
func (m *Manager) ChangePassword(userID, oldPassword, newPassword string) error {
	user, err := m.store.GetUserByID(userID)
	if err != nil {
		return err
	}
	ok, err := VerifyPassword(user.PasswordHash, oldPassword)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	if err := ValidatePasswordStrength(newPassword); err != nil {
		return ErrWeakPassword
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := m.store.UpdateUserPassword(userID, hash, false); err != nil {
		return err
	}
	return m.store.DeleteUserSessions(userID)
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- request context plumbing ---

type ctxKey int

const (
	userCtxKey ctxKey = iota
	sessionCtxKey
)

// WithUser returns a context carrying the authenticated user and session.
func WithUser(ctx context.Context, u *store.User, s *store.Session) context.Context {
	ctx = context.WithValue(ctx, userCtxKey, u)
	return context.WithValue(ctx, sessionCtxKey, s)
}

// UserFrom extracts the authenticated user and session from a context.
func UserFrom(ctx context.Context) (*store.User, *store.Session, bool) {
	u, ok := ctx.Value(userCtxKey).(*store.User)
	if !ok {
		return nil, nil, false
	}
	s, _ := ctx.Value(sessionCtxKey).(*store.Session)
	return u, s, true
}

// --- in-memory login rate limiter ---

type limiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	attempts map[string]*attempt
}

type attempt struct {
	count     int
	first     time.Time
	lockUntil time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, attempts: make(map[string]*attempt)}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.attempts[key]
	if a == nil {
		return true
	}
	return time.Now().After(a.lockUntil)
}

func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	a := l.attempts[key]
	if a == nil || (now.Sub(a.first) > l.window && now.After(a.lockUntil)) {
		a = &attempt{first: now}
		l.attempts[key] = a
	}
	a.count++
	if a.count >= l.max {
		a.lockUntil = now.Add(l.window)
		a.count = 0
		a.first = now
	}
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
