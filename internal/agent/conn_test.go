package agent

import (
	"testing"

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
