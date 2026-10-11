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

func TestReconnectDelayBackoffGrowsAndIsCapped(t *testing.T) {
	// Highest in-range draw: randIntN(n) == n-1.
	top := func(n int64) int64 { return n - 1 }

	base := int64(reconnectBackoffBase)
	capN := int64(reconnectBackoffCap)

	want := []time.Duration{
		5*time.Second - 1,   // attempt 0: base
		10*time.Second - 1,  // attempt 1
		20*time.Second - 1,  // attempt 2
		40*time.Second - 1,  // attempt 3
		80*time.Second - 1,  // attempt 4
		120*time.Second - 1, // attempt 5: 160s clamped to the 2m cap
		120*time.Second - 1, // attempt 6
		120*time.Second - 1, // attempt 7
		120*time.Second - 1, // attempt 8
	}

	var prev time.Duration
	for attempt, expected := range want {
		got := reconnectDelay(attempt, top)
		require.Equal(t, expected, got, "attempt %d", attempt)
		require.LessOrEqual(t, prev, got, "attempt %d must not decrease", attempt)
		require.LessOrEqual(t, got, time.Duration(capN-1), "attempt %d must stay capped", attempt)
		prev = got
	}

	// A very large attempt must saturate at the cap, not overflow.
	for _, attempt := range []int{9, 16, 64, 1024} {
		got := reconnectDelay(attempt, top)
		require.Equal(t, time.Duration(capN-1), got, "attempt %d", attempt)
	}

	// A negative attempt is treated as 0 (base interval).
	require.Equal(t, time.Duration(base-1), reconnectDelay(-1, top))
}

func TestReconnectDelayFullJitterSpansRange(t *testing.T) {
	low := func(int64) int64 { return 0 }
	top := func(n int64) int64 { return n - 1 }

	for attempt := 0; attempt <= 8; attempt++ {
		gotLow := reconnectDelay(attempt, low)
		require.Equal(t, time.Duration(0), gotLow, "attempt %d low end", attempt)

		gotHigh := reconnectDelay(attempt, top)
		require.Less(t, gotHigh, reconnectBackoffCap, "attempt %d must stay below cap", attempt)

		// The high end must be strictly below the computed delay (half-open
		// interval [0, computed)), never equal to or over the cap.
		computed := reconnectBackoffBase
		for i := 0; i < attempt; i++ {
			computed *= 2
			if computed >= reconnectBackoffCap {
				computed = reconnectBackoffCap
				break
			}
		}
		require.Equal(t, computed-1, gotHigh, "attempt %d high end", attempt)
	}

	// Even a saturated attempt draws below the cap.
	for _, attempt := range []int{16, 64} {
		require.Less(t, reconnectDelay(attempt, top), reconnectBackoffCap, "attempt %d", attempt)
	}
}

func TestNextAttemptResetsOnlyOnHealthySession(t *testing.T) {
	require.Equal(t, 3, nextAttempt(2, 29*time.Second), "flapping session keeps backing off")
	require.Equal(t, 0, nextAttempt(2, healthySessionDuration), "healthy session resets")
	require.Equal(t, 1, nextAttempt(0, 0), "first failure advances the counter")
	require.Equal(t, 0, nextAttempt(5, time.Hour), "a longer session also resets")
}
