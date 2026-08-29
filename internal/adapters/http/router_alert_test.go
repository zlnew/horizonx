package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"horizonx/internal/config"

	"github.com/stretchr/testify/assert"
)

// TestAlertRoutesRegistered proves the 10 alert endpoints are wired into the
// mux whenever the Alert handler is provided. Unauthenticated requests hit
// the JWT gate first (401); an UNREGISTERED path falls through to the
// ServeMux 404. So: registered → 401, missing → 404.
func TestAlertRoutesRegistered(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:      "test-secret",
		AllowedOrigins: []string{},
	}

	router := NewRouter(cfg, &RouterDeps{
		Alert: &AlertHandler{}, // non-nil enables the alerts block
	})

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/alerts/rules"},
		{http.MethodPost, "/alerts/rules"},
		{http.MethodGet, "/alerts/rules/1"},
		{http.MethodPut, "/alerts/rules/1"},
		{http.MethodDelete, "/alerts/rules/1"},
		{http.MethodGet, "/alerts/active"},
		{http.MethodGet, "/alerts/history"},
		{http.MethodGet, "/alerts/history/1"},
		{http.MethodPost, "/alerts/1/ack"},
		{http.MethodPost, "/alerts/1/silence"},
	}

	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.NotEqual(t, http.StatusNotFound, rec.Code,
			"route %s %s must be registered", r.method, r.path)
	}

	// A path outside the alerts block must still 404.
	req := httptest.NewRequest(http.MethodGet, "/alerts/not-a-route", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code, "unregistered alert path must 404")
}

// TestAlertRoutesSkippedWithoutHandler proves the alerts block is inert when
// the Alert handler is nil (deps.Alert == nil guard).
func TestAlertRoutesSkippedWithoutHandler(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:      "test-secret",
		AllowedOrigins: []string{},
	}

	router := NewRouter(cfg, &RouterDeps{})

	req := httptest.NewRequest(http.MethodGet, "/alerts/rules", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code, "alerts routes must not exist without the handler")
}
