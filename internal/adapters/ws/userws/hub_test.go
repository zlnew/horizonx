package userws

import (
	"context"
	"testing"

	"horizonx/internal/domain"
)

type noopLogger struct{}

func (noopLogger) Debug(msg string, args ...any) {}
func (noopLogger) Info(msg string, args ...any)  {}
func (noopLogger) Warn(msg string, args ...any)  {}
func (noopLogger) Error(msg string, args ...any) {}

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	return NewHub(context.Background(), noopLogger{})
}

// TestHandleEventDoesNotKillSlowClient verifies the hardening: a client whose
// send buffer is full must NOT be unregistered/killed. Previously the hub
// force-unregistered slow clients, which made the dashboard "just die" under
// verbose log floods.
func TestHandleEventDoesNotKillSlowClient(t *testing.T) {
	h := newTestHub(t)

	// Register a client with a tiny send buffer (1) so one event fills it.
	c := &Client{
		ctx:  context.Background(),
		hub:  h,
		send: make(chan []byte, 1),
		log:  noopLogger{},
		ID:   "client-1",
	}
	h.clients[c] = true

	// Fill the buffer with a first event.
	h.handleEvent(&domain.WsServerEvent{
		Channel: "",
		Event:   "fill",
		Payload: map[string]any{"n": 1},
	})

	// A second event would previously hit the default branch and unregister.
	h.handleEvent(&domain.WsServerEvent{
		Channel: "",
		Event:   "overflow",
		Payload: map[string]any{"n": 2},
	})

	if _, ok := h.clients[c]; !ok {
		t.Fatal("client was unregistered on full send buffer; expected drop-not-kill")
	}
	if h.dropped == 0 {
		t.Fatal("expected dropped counter to increment")
	}

	// The first event must still be delivered.
	select {
	case <-c.send:
	default:
		t.Fatal("expected first event to be in the send buffer")
	}
}

// TestHandleEventChannelScoping verifies events are only delivered to
// subscribers of the event's channel (and to everyone when channel is empty).
func TestHandleEventChannelScoping(t *testing.T) {
	h := newTestHub(t)

	sub := NewClient(h, nil, noopLogger{}, "sub")
	other := NewClient(h, nil, noopLogger{}, "other")
	h.clients[sub] = true
	h.clients[other] = true
	h.channels["deployment:7"] = map[*Client]bool{sub: true}

	// Channel-scoped event: only `sub` gets it.
	h.handleEvent(&domain.WsServerEvent{
		Channel: "deployment:7",
		Event:   "log_received",
		Payload: map[string]any{"n": 1},
	})

	select {
	case <-sub.send:
	default:
		t.Fatal("expected scoped event delivered to channel subscriber")
	}
	select {
	case <-other.send:
		t.Fatal("scoped event leaked to non-subscriber")
	default:
	}
}
