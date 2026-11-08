package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"horizonx/internal/adapters/http/middleware/ratelimit"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/config"
	"horizonx/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// C5: every middleware rejection must be the JSON envelope
// (`application/json` + non-empty `message`), never text/plain.

type stubMiddlewareLogger struct{}

func (stubMiddlewareLogger) Debug(msg string, args ...any) {}
func (stubMiddlewareLogger) Info(msg string, args ...any)  {}
func (stubMiddlewareLogger) Warn(msg string, args ...any)  {}
func (stubMiddlewareLogger) Error(msg string, args ...any) {}

// rejectingServerService fails agent authorization with invalid credentials.
type rejectingServerService struct {
	domain.ServerService
}

func (rejectingServerService) AuthorizeAgent(ctx context.Context, _ uuid.UUID, _ string) (*domain.Server, error) {
	return nil, domain.ErrInvalidCredentials
}

func envelopeBody(t *testing.T, rec *httptest.ResponseRecorder) response.Response {
	t.Helper()
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json",
		"rejection body must be JSON, got %q", rec.Body.String())
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body),
		"body must decode as the response envelope, got %q", rec.Body.String())
	require.NotEmpty(t, body.Message, "envelope message must not be empty")
	return body
}

func envelopeTestConfig() *config.Config {
	return &config.Config{JWTSecret: "test-secret", JWTExpiry: time.Hour}
}

func TestEnvelope_Agent(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("missing authorization header", func(t *testing.T) {
		rec := httptest.NewRecorder()
		Agent(&fakeServerService{}, stubMiddlewareLogger{})(ok).ServeHTTP(rec,
			httptest.NewRequest(http.MethodGet, "/agent/jobs", nil))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "missing authorization header", envelopeBody(t, rec).Message)
	})

	t.Run("malformed scheme", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/agent/jobs", nil)
		req.Header.Set("Authorization", "Basic abc")
		rec := httptest.NewRecorder()
		Agent(&fakeServerService{}, stubMiddlewareLogger{})(ok).ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "unauthorized", envelopeBody(t, rec).Message)
	})

	t.Run("malformed credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/agent/jobs", nil)
		req.Header.Set("Authorization", "Bearer not-a-uuid.secret")
		rec := httptest.NewRecorder()
		Agent(&fakeServerService{}, stubMiddlewareLogger{})(ok).ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		envelopeBody(t, rec)
	})

	t.Run("invalid credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/agent/jobs", nil)
		req.Header.Set("Authorization", "Bearer 00000000-0000-0000-0000-000000000000.secret")
		rec := httptest.NewRecorder()
		Agent(rejectingServerService{}, stubMiddlewareLogger{})(ok).ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "invalid credentials", envelopeBody(t, rec).Message)
	})
}

func TestEnvelope_CSRF(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("missing cookie", func(t *testing.T) {
		rec := httptest.NewRecorder()
		CSRF(envelopeTestConfig(), stubMiddlewareLogger{})(ok).ServeHTTP(rec,
			httptest.NewRequest(http.MethodPost, "/", nil))
		require.Equal(t, http.StatusForbidden, rec.Code)
		require.Equal(t, "Missing CSRF cookie", envelopeBody(t, rec).Message)
	})

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.AddCookie(&http.Cookie{Name: "horizonx_csrf_token", Value: "abc"})
		rec := httptest.NewRecorder()
		CSRF(envelopeTestConfig(), stubMiddlewareLogger{})(ok).ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
		require.Equal(t, "Missing CSRF token header", envelopeBody(t, rec).Message)
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.AddCookie(&http.Cookie{Name: "horizonx_csrf_token", Value: "abc"})
		req.Header.Set("X-CSRF-Token", "different")
		rec := httptest.NewRecorder()
		CSRF(envelopeTestConfig(), stubMiddlewareLogger{})(ok).ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
		require.Equal(t, "Invalid CSRF token", envelopeBody(t, rec).Message)
	})
}

func TestEnvelope_JWT(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("no token", func(t *testing.T) {
		rec := httptest.NewRecorder()
		JWT(envelopeTestConfig(), newMemSessionStore(), stubMiddlewareLogger{})(ok).ServeHTTP(rec,
			httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "Unauthorized: No token found", envelopeBody(t, rec).Message)
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "horizonx_access_token", Value: "bogus"})
		rec := httptest.NewRecorder()
		JWT(envelopeTestConfig(), newMemSessionStore(), stubMiddlewareLogger{})(ok).ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "Unauthorized: Invalid token", envelopeBody(t, rec).Message)
	})

	t.Run("revoked session", func(t *testing.T) {
		cfg := envelopeTestConfig()
		claims := domain.AuthClaims{
			UserID: 7, Role: "admin", SessionID: "sess-dead",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(cfg.JWTSecret))
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "horizonx_access_token", Value: signed})
		rec := httptest.NewRecorder()
		JWT(cfg, newMemSessionStore(), stubMiddlewareLogger{})(ok).ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "Unauthorized: Session revoked", envelopeBody(t, rec).Message)
	})

	t.Run("session mismatch", func(t *testing.T) {
		cfg := envelopeTestConfig()
		store := newMemSessionStore()
		store.sessions["sess-live"] = &domain.Session{ID: "sess-live", UserID: 99}
		claims := domain.AuthClaims{
			UserID: 7, Role: "admin", SessionID: "sess-live",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(cfg.JWTSecret))
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "horizonx_access_token", Value: signed})
		rec := httptest.NewRecorder()
		JWT(cfg, store, stubMiddlewareLogger{})(ok).ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "Unauthorized: Session mismatch", envelopeBody(t, rec).Message)
	})
}

func TestEnvelope_RateLimit(t *testing.T) {
	l := ratelimit.New(1, time.Minute, stubMiddlewareLogger{})
	handler := l.Middleware(func(r *http.Request) string { return ratelimit.ClientIP(r) })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	first := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	first.RemoteAddr = "10.0.0.1:54321"
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)
	require.Equal(t, http.StatusOK, firstRec.Code)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "too many requests", envelopeBody(t, rec).Message)
}
