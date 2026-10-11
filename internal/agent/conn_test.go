package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"horizonx/internal/config"
	"horizonx/internal/domain"

	"github.com/stretchr/testify/require"
)

type noopTestLogger struct{}

func (noopTestLogger) Debug(msg string, kv ...any) {}
func (noopTestLogger) Info(msg string, kv ...any)  {}
func (noopTestLogger) Warn(msg string, kv ...any)  {}
func (noopTestLogger) Error(msg string, kv ...any) {}

func TestSendBufferDropCountedAndReported(t *testing.T) {
	a := NewAgent(&config.Config{}, noopTestLogger{})
	// Saturate the 256-slot send channel without a live WebSocket.
	for range cap(a.send) {
		a.send <- []byte("fill")
	}

	err := a.sendMessage(&domain.WsAgentMessage{Event: "ping"})
	require.ErrorIs(t, err, ErrSendBufferFull)
	require.Equal(t, int64(1), a.DroppedMessages())

	a.sendServerOSInfo()
	require.Equal(t, int64(2), a.DroppedMessages())
}

func TestSendBufferDropLogThrottled(t *testing.T) {
	a := NewAgent(&config.Config{}, &countWarnLogger{})
	for range cap(a.send) {
		a.send <- []byte("fill")
	}

	for range 3 * dropLogEvery {
		_ = a.sendMessage(&domain.WsAgentMessage{Event: "ping"})
	}

	require.Equal(t, int64(3*dropLogEvery), a.DroppedMessages())
	// First drop + one line per dropLogEvery drops = 4 lines for 300 drops.
	require.Equal(t, 4, a.log.(*countWarnLogger).warns)
	require.ErrorIs(t, a.sendMessage(&domain.WsAgentMessage{Event: "ping"}), ErrSendBufferFull)
}

type countWarnLogger struct {
	noopTestLogger
	warns int
}

func (l *countWarnLogger) Warn(msg string, kv ...any) { l.warns++ }

type countErrorLogger struct {
	noopTestLogger
	errors int
}

func (l *countErrorLogger) Error(msg string, kv ...any) { l.errors++ }

type orderLogger struct {
	noopTestLogger
	mu    sync.Mutex
	types []string
}

func (l *orderLogger) Warn(msg string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if msg == "unknown server command" {
		for i := 0; i+1 < len(kv); i += 2 {
			if k, ok := kv[i].(string); ok && k == "type" {
				if s, ok := kv[i+1].(string); ok {
					l.types = append(l.types, s)
				}
			}
		}
	}
}

func (l *orderLogger) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.types...)
}

func TestDispatchNonPingIsEnqueueOnly(t *testing.T) {
	a := NewAgent(&config.Config{}, noopTestLogger{})
	ctx := context.Background()

	require.NoError(t, a.dispatchCommand(ctx, domain.AgentCommand{Type: "logs_tail_start"}))
	require.Len(t, a.commands, 1, "non-ping command must be enqueued, not executed")

	require.NoError(t, a.dispatchCommand(ctx, domain.AgentCommand{Type: "logs_tail_stop"}))
	require.NoError(t, a.dispatchCommand(ctx, domain.AgentCommand{Type: "logs_query"}))
	require.Len(t, a.commands, 3, "dispatch must be enqueue-only so a slow handler never runs on the read loop")
}

func TestDispatchPingAnsweredInlineWhenQueueFull(t *testing.T) {
	a := NewAgent(&config.Config{}, noopTestLogger{})
	ctx := context.Background()

	for range cap(a.commands) {
		require.NoError(t, a.dispatchCommand(ctx, domain.AgentCommand{Type: "logs_tail_start"}))
	}
	require.Len(t, a.commands, cap(a.commands))

	require.NoError(t, a.dispatchCommand(ctx, domain.AgentCommand{Type: "logs_tail_start"}))
	require.Len(t, a.commands, cap(a.commands), "full queue must drop, not block")

	require.NoError(t, a.dispatchCommand(ctx, domain.AgentCommand{ID: "p1", Type: "ping"}))

	select {
	case raw := <-a.send:
		var reply domain.WsAgentMessage
		require.NoError(t, json.Unmarshal(raw, &reply))
		require.Equal(t, "pong", reply.Event)
	case <-time.After(2 * time.Second):
		t.Fatal("ping must be answered inline even when the command queue is full")
	}
}

func TestRunCommandsPreservesOrder(t *testing.T) {
	log := &orderLogger{}
	a := NewAgent(&config.Config{}, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- a.runCommands(ctx) }()

	want := []string{"cmd-a", "cmd-b", "cmd-c"}
	for _, typ := range want {
		a.commands <- domain.AgentCommand{Type: typ}
	}

	require.Eventually(t, func() bool {
		return len(log.snapshot()) == len(want)
	}, 5*time.Second, 5*time.Millisecond)
	require.Equal(t, want, log.snapshot())

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("runCommands did not return after cancellation")
	}
}

func TestDispatchQueueFullDropsWithoutBlocking(t *testing.T) {
	log := &countErrorLogger{}
	a := NewAgent(&config.Config{}, log)
	ctx := context.Background()

	for range cap(a.commands) {
		require.NoError(t, a.dispatchCommand(ctx, domain.AgentCommand{Type: "logs_tail_start"}))
	}

	finished := make(chan error, 1)
	go func() { finished <- a.dispatchCommand(ctx, domain.AgentCommand{Type: "logs_tail_start"}) }()

	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch blocked on a full queue; enqueue must be non-blocking")
	}
	require.Len(t, a.commands, cap(a.commands))
	require.Equal(t, 1, log.errors, "a dropped command must be logged via Error")
}

func TestRunCommandsShutdown(t *testing.T) {
	a := NewAgent(&config.Config{}, noopTestLogger{})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- a.runCommands(ctx) }()

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("runCommands did not return after context cancellation")
	}
}

func TestJobWorkerStampDroppedMessages(t *testing.T) {
	w := &JobWorker{}
	w.SetDroppedMessagesFunc(func() int64 { return 7 })

	m := &domain.Metrics{}
	w.stampDroppedMessages(m)
	require.Equal(t, int64(7), m.DroppedMessages)
}

func TestJobWorkerStampDroppedMessagesNilSafe(t *testing.T) {
	w := &JobWorker{}

	m := &domain.Metrics{DroppedMessages: 99}
	w.stampDroppedMessages(m)
	require.Equal(t, int64(0), m.DroppedMessages)

	require.Equal(t, int64(0), w.droppedCount())
	require.NotPanics(t, func() { w.stampDroppedMessages(nil) })
}
