package domain

import (
	"context"
	"time"
)

// Session represents a single authenticated login. Sessions are stored
// server-side (Redis) so that logout, password change, and admin revocation
// actually kill a token before its JWT expiry — the JWT alone is stateless
// and can't be revoked. Multi-user/multi-server scale makes revocation a
// requirement, not a nicety.
type Session struct {
	ID        string
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
	IP        string
	UserAgent string
}

// SessionStore persists sessions. Implementations must enforce TTL expiry
// (Redis TTL = JWT expiry) so stale sessions age out on their own.
type SessionStore interface {
	Create(ctx context.Context, s *Session) error
	// Get returns nil session when the session does not exist or is expired.
	Get(ctx context.Context, sessionID string) (*Session, error)
	Delete(ctx context.Context, sessionID string) error
	// DeleteAllForUser revokes every session for a user (password change,
	// admin kick).
	DeleteAllForUser(ctx context.Context, userID int64) error
	ListForUser(ctx context.Context, userID int64) ([]*Session, error)
}
