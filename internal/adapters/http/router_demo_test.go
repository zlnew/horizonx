package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"horizonx/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestConfigEndpoints_DemoMode(t *testing.T) {
	t.Run("DemoMode false", func(t *testing.T) {
		cfg := &config.Config{
			DemoMode:       false,
			AllowedOrigins: []string{},
		}
		router := NewRouter(cfg, &RouterDeps{})

		for _, path := range []string{"/config", "/auth/config"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			var resp map[string]any
			err := json.Unmarshal(rec.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, false, resp["demo_mode"])
		}
	})

	t.Run("DemoMode true", func(t *testing.T) {
		cfg := &config.Config{
			DemoMode:       true,
			AllowedOrigins: []string{},
		}
		router := NewRouter(cfg, &RouterDeps{})

		for _, path := range []string{"/config", "/auth/config"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			var resp map[string]any
			err := json.Unmarshal(rec.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, true, resp["demo_mode"])
		}
	})
}
