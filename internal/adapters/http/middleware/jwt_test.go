package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"horizonx/internal/config"
	"horizonx/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// memSessionStore is a tiny in-memory session store for middleware tests.
type memSessionStore struct {
	sessions map[string]*domain.Session
}

func newMemSessionStore() *memSessionStore {
	return &memSessionStore{sessions: map[string]*domain.Session{}}
}

func (m *memSessionStore) Create(ctx context.Context, s *domain.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *memSessionStore) Get(ctx context.Context, sessionID string) (*domain.Session, error) {
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *memSessionStore) Delete(ctx context.Context, sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *memSessionStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	for id, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, id)
		}
	}
	return nil
}

func (m *memSessionStore) ListForUser(ctx context.Context, userID int64) ([]*domain.Session, error) {
	var out []*domain.Session
	for _, s := range m.sessions {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func makeToken(t *testing.T, cfg *config.Config, claims domain.AuthClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte(cfg.JWTSecret))
	assert.NoError(t, err)
	return s
}

func testConfig() *config.Config {
	return &config.Config{JWTSecret: "test-secret"}
}

func TestJWT_ValidSessionPasses(t *testing.T) {
	cfg := testConfig()
	store := newMemSessionStore()
	store.sessions["sess-live"] = &domain.Session{ID: "sess-live", UserID: 7}

	claims := domain.AuthClaims{
		UserID:    7,
		Role:      "admin",
		SessionID: "sess-live",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := makeToken(t, cfg, claims)

	handler := JWT(cfg, store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userCtx, ok := domain.GetUserContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, int64(7), userCtx.ID)
		assert.Equal(t, "sess-live", userCtx.SessionID)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "horizonx_access_token", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestJWT_RevokedSessionRejected(t *testing.T) {
	cfg := testConfig()
	store := newMemSessionStore() // empty — session was revoked

	claims := domain.AuthClaims{
		UserID:    7,
		Role:      "admin",
		SessionID: "sess-dead",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := makeToken(t, cfg, claims)

	handler := JWT(cfg, store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "horizonx_access_token", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWT_SessionUserMismatchRejected(t *testing.T) {
	cfg := testConfig()
	store := newMemSessionStore()
	// Session belongs to user 99, token claims user 7.
	store.sessions["sess-live"] = &domain.Session{ID: "sess-live", UserID: 99}

	claims := domain.AuthClaims{
		UserID:    7,
		Role:      "admin",
		SessionID: "sess-live",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := makeToken(t, cfg, claims)

	handler := JWT(cfg, store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "horizonx_access_token", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWT_NoSessionIDSkipsCheck(t *testing.T) {
	// Legacy tokens (pre-session) have no sid claim. The middleware must
	// still accept them — otherwise every existing login breaks on upgrade.
	cfg := testConfig()
	store := newMemSessionStore() // no sessions at all

	claims := domain.AuthClaims{
		UserID: 7,
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := makeToken(t, cfg, claims)

	handler := JWT(cfg, store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "horizonx_access_token", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
