package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// P1-10: requests within the limit pass, requests over it get 429.
func TestLimiterBlocksOverLimit(t *testing.T) {
	l := New(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("4th request should be blocked")
	}
}

// P1-10: different clients are independent.
func TestLimiterIsPerClient(t *testing.T) {
	l := New(2, time.Minute)

	l.Allow("1.2.3.4")
	l.Allow("1.2.3.4")

	if !l.Allow("5.6.7.8") {
		t.Fatal("different client must not be blocked by another client's limit")
	}
}

// P1-10: the window slides — old hits expire.
func TestLimiterWindowSlides(t *testing.T) {
	l := New(1, 50*time.Millisecond)

	if !l.Allow("1.2.3.4") {
		t.Fatal("first request should be allowed")
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("second request within window should be blocked")
	}

	time.Sleep(60 * time.Millisecond)

	if !l.Allow("1.2.3.4") {
		t.Fatal("request after window expiry should be allowed")
	}
}

// P1-10: middleware returns 429 with the configured limit.
func TestMiddlewareReturns429(t *testing.T) {
	l := New(2, time.Minute)

	handler := l.Middleware(func(r *http.Request) string { return ClientIP(r) })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = "10.0.0.1:54321"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

// P1-10: ClientIP strips the port.
func TestClientIPStripsPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = "192.168.1.50:9999"
	if got := ClientIP(req); got != "192.168.1.50" {
		t.Fatalf("expected 192.168.1.50, got %q", got)
	}
}
