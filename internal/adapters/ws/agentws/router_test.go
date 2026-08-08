package agentws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopLog satisfies logger.Logger without producing output.
type noopLog struct{}

func (noopLog) Debug(string, ...any) {}
func (noopLog) Info(string, ...any)  {}
func (noopLog) Warn(string, ...any)  {}
func (noopLog) Error(string, ...any) {}

func newTestRouter(t *testing.T) *Router {
	t.Helper()

	router := NewRouter(context.Background(), noopLog{})
	go router.Run()
	t.Cleanup(router.Stop)
	return router
}

// registerFakeAgent registers a bare client (only the send channel matters
// for the send path) under the given server ID and blocks until the router
// loop has actually processed the registration — otherwise SendCommand can
// race ahead and report ErrAgentOffline.
func registerFakeAgent(t *testing.T, r *Router, id uuid.UUID) <-chan []byte {
	t.Helper()

	send := make(chan []byte, 8)
	client := &Client{
		ctx:  context.Background(),
		send: send,
		log:  noopLog{},
		ID:   id,
	}
	r.register <- client

	// Poll with a ping until the router map contains the agent. The
	// successful ping leaves one message in the send channel — drain it so
	// the caller's own reads start clean.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := r.SendCommand(context.Background(), id, domain.AgentCommand{Type: "ping"})
		if err == nil {
			for {
				select {
				case <-send:
				default:
					goto registered
				}
			}
		}
		if !errors.Is(err, domain.ErrAgentOffline) {
			t.Fatalf("unexpected SendCommand error during registration wait: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}
registered:

	t.Cleanup(func() {
		r.unregister <- client
	})
	return send
}

func TestSendCommand_Offline_ReturnsErrAgentOffline(t *testing.T) {
	r := newTestRouter(t)

	err := r.SendCommand(context.Background(), uuid.New(), domain.AgentCommand{Type: "ping"})

	assert.ErrorIs(t, err, domain.ErrAgentOffline)
}

func TestSendCommand_Online_DeliversEnvelope(t *testing.T) {
	r := newTestRouter(t)
	serverID := uuid.New()
	send := registerFakeAgent(t, r, serverID)

	cmd := domain.AgentCommand{ID: "cmd-1", Type: "ping", Payload: []byte(`{"t":1}`)}
	err := r.SendCommand(context.Background(), serverID, cmd)
	assert.NoError(t, err)

	// Router wraps the command in the WsServerMessage envelope the agent
	// actually reads on the wire.
	select {
	case raw := <-send:
		var msg domain.WsServerMessage
		require.NoError(t, json.Unmarshal(raw, &msg))
		assert.Equal(t, serverID, msg.TargetServerID)

		var got domain.AgentCommand
		require.NoError(t, json.Unmarshal(msg.Payload, &got))
		assert.Equal(t, "cmd-1", got.ID)
		assert.Equal(t, "ping", got.Type)
		assert.Equal(t, `{"t":1}`, string(got.Payload))
	default:
		t.Fatal("expected command to be delivered to the agent's send channel")
	}
}

func TestSendCommand_ConcurrentWithRegisterUnregister(t *testing.T) {
	r := newTestRouter(t)

	// Hammer SendCommand (mostly offline misses) while agents register and
	// unregister concurrently. Run with -race; the agents map must never be
	// touched outside the router loop.
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				serverID := uuid.New()
				_ = r.SendCommand(context.Background(), serverID, domain.AgentCommand{Type: "ping"})
			}
		}()
	}

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				serverID := uuid.New()
				client := &Client{
					ctx:  context.Background(),
					send: make(chan []byte, 8),
					log:  noopLog{},
					ID:   serverID,
				}
				r.register <- client
				r.unregister <- client
			}
		}()
	}

	wg.Wait()
}

func TestSendCommand_ContextCancelled(t *testing.T) {
	r := newTestRouter(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.SendCommand(ctx, uuid.New(), domain.AgentCommand{Type: "ping"})
	assert.Error(t, err)
	assert.False(t, errors.Is(err, domain.ErrAgentOffline))
}

var _ = logger.Logger(noopLog{})
