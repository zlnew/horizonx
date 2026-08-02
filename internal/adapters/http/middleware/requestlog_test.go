package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"horizonx/internal/logger"
)

type captureLogger struct {
	msg  string
	args []any
}

func (c *captureLogger) Debug(msg string, args ...any) {}
func (c *captureLogger) Info(msg string, args ...any) {
	c.msg = msg
	c.args = args
}
func (c *captureLogger) Warn(msg string, args ...any) {}
func (c *captureLogger) Error(msg string, args ...any) {}

func TestRequestLogLogsRequest(t *testing.T) {
	cl := &captureLogger{}
	handler := RequestLog(logger.Logger(cl))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://example.com/applications/5/deploy", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if cl.msg != "http: request" {
		t.Fatalf("expected http: request log, got %q", cl.msg)
	}

	args := map[string]any{}
	for i := 0; i+1 < len(cl.args); i += 2 {
		if key, ok := cl.args[i].(string); ok {
			args[key] = cl.args[i+1]
		}
	}
	if args["method"] != http.MethodPost {
		t.Fatalf("expected method POST, got %v", args["method"])
	}
	if args["status"] != http.StatusCreated {
		t.Fatalf("expected status 201, got %v", args["status"])
	}
	if args["path"] != "/applications/5/deploy" {
		t.Fatalf("expected path, got %v", args["path"])
	}
	if _, ok := args["duration_ms"]; !ok {
		t.Fatal("expected duration_ms key")
	}
}

func TestRequestLogDefaultsStatusTo200(t *testing.T) {
	cl := &captureLogger{}
	handler := RequestLog(logger.Logger(cl))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	args := map[string]any{}
	for i := 0; i+1 < len(cl.args); i += 2 {
		if key, ok := cl.args[i].(string); ok {
			args[key] = cl.args[i+1]
		}
	}
	if args["status"] != http.StatusOK {
		t.Fatalf("expected default status 200, got %v", args["status"])
	}
}
