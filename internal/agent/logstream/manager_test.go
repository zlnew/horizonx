package logstream

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"horizonx/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopLogger satisfies the logger.Logger interface used by the manager.
type noopLogger struct{}

func (noopLogger) Debug(msg string, kv ...any) {}
func (noopLogger) Info(msg string, kv ...any)  {}
func (noopLogger) Warn(msg string, kv ...any)  {}
func (noopLogger) Error(msg string, kv ...any) {}

// fakeDocker returns a fixed compose file path (never invoked in unit tests
// that stop before the command runs — StartTail with a 0-arg compose still
// calls GetDockerComposeFile via runCompose; tests that must not touch the
// fs use StartTail only up to the cap check).
type fakeDocker struct{}

func (fakeDocker) GetDockerComposeFile(workDir string) (string, error) {
	return "/nonexistent/docker-compose.yml", nil
}

// collectSend returns a send callback that records every WsAgentMessage.
func collectSend() (func(*domain.WsAgentMessage) error, *[]*domain.WsAgentMessage, *sync.Mutex) {
	var mu sync.Mutex
	var msgs []*domain.WsAgentMessage
	send := func(m *domain.WsAgentMessage) error {
		mu.Lock()
		defer mu.Unlock()
		msgs = append(msgs, m)
		return nil
	}
	return send, &msgs, &mu
}

func TestParseLogLine(t *testing.T) {
	line := parseLogLine("app-1  | 2023-05-01T12:00:00.000000000Z hello world", "stdout")
	assert.Equal(t, "app-1", line.Service)
	assert.Equal(t, "2023-05-01T12:00:00.000000000Z", line.Timestamp)
	assert.Equal(t, "hello world", line.Text)
	assert.Equal(t, "stdout", line.Stream)

	// No timestamp (startup noise outside the prefix format).
	line2 := parseLogLine("app-1  | plain message", "stderr")
	assert.Equal(t, "app-1", line2.Service)
	assert.Equal(t, "plain message", line2.Text)
	assert.Equal(t, "", line2.Timestamp)

	// No service prefix at all.
	line3 := parseLogLine("raw noise line", "stdout")
	assert.Equal(t, "raw noise line", line3.Text)
	assert.Equal(t, "", line3.Service)
}

func TestParseLogLine_TimestampVariants(t *testing.T) {
	// docker compose logs --timestamps emits RFC3339 with 'T'. Variants in
	// precision all parse the same way.
	for _, ts := range []string{"2023-05-01T12:00:00Z", "2023-05-01T12:00:00.123456789Z"} {
		line := parseLogLine(fmt.Sprintf("svc  | %s message", ts), "stdout")
		assert.Equal(t, ts, line.Timestamp, "timestamp %q should parse", ts)
		assert.Equal(t, "message", line.Text)
	}
}

// TestStartTail_CapReached verifies the 5-concurrent-tails cap. The
// injected runner blocks on ctx.Done so no docker exec happens.
func TestStartTail_CapReached(t *testing.T) {
	send, _, _ := collectSend()
	m := NewManager(uuid.New(), "/tmp/apps", fakeDocker{}, noopLogger{}, send)
	m.runCompose = func(ctx context.Context, workDir string, args []string, s *stream) error {
		<-ctx.Done()
		return ctx.Err()
	}

	for i := int64(1); i <= maxConcurrentTails; i++ {
		_, err := m.StartTail(domain.LogsTailStartPayload{StreamID: fmt.Sprintf("s%d", i), ApplicationID: i, DirName: "app"})
		require.NoError(t, err, "tail %d should start", i)
	}

	_, err := m.StartTail(domain.LogsTailStartPayload{StreamID: "overflow", ApplicationID: 99, DirName: "app"})
	assert.ErrorContains(t, err, "max concurrent tails")
}

// TestStartTail_IdempotentPerApp verifies one --follow process per app:
// a second StartTail for the same application returns the existing ID.
func TestStartTail_IdempotentPerApp(t *testing.T) {
	send, _, _ := collectSend()
	m := NewManager(uuid.New(), "/tmp/apps", fakeDocker{}, noopLogger{}, send)
	m.runCompose = func(ctx context.Context, workDir string, args []string, s *stream) error {
		<-ctx.Done()
		return ctx.Err()
	}

	first, err := m.StartTail(domain.LogsTailStartPayload{StreamID: "a1", ApplicationID: 1, DirName: "app"})
	require.NoError(t, err)

	second, err := m.StartTail(domain.LogsTailStartPayload{StreamID: "a2", ApplicationID: 1, DirName: "app"})
	require.NoError(t, err)

	assert.Equal(t, first, second, "second tail for same app should reuse the stream")
}

// TestStopTail_DeregistersAndFreesSlot verifies StopTail + finish clean up
// the cap slot so a new tail can start. The injected runner blocks until
// ctx is cancelled — deterministic, no docker exec.
func TestStopTail_DeregistersAndFreesSlot(t *testing.T) {
	send, _, _ := collectSend()
	m := NewManager(uuid.New(), "/tmp/apps", fakeDocker{}, noopLogger{}, send)
	m.runCompose = func(ctx context.Context, workDir string, args []string, s *stream) error {
		<-ctx.Done()
		return ctx.Err()
	}

	for i := int64(1); i <= maxConcurrentTails; i++ {
		_, err := m.StartTail(domain.LogsTailStartPayload{StreamID: fmt.Sprintf("s%d", i), ApplicationID: i, DirName: "app"})
		require.NoError(t, err)
	}

	// Stop one tail; its goroutine unblocks on ctx.Done and deregisters.
	m.StopTail("s1")
	require.Eventually(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.tailCount == maxConcurrentTails-1
	}, 2*time.Second, 10*time.Millisecond, "tail count should drop after stop")

	_, err := m.StartTail(domain.LogsTailStartPayload{StreamID: "new", ApplicationID: 50, DirName: "app"})
	assert.NoError(t, err, "freed slot should accept a new tail")
}

// TestQueryStarts verifies Query returns an ID and starts a stream.
func TestQueryStarts(t *testing.T) {
	send, _, _ := collectSend()
	m := NewManager(uuid.New(), "/tmp/apps", fakeDocker{}, noopLogger{}, send)

	req := domain.LogsQueryPayload{ApplicationID: 1, DirName: "app", Tail: 100}
	queryID, err := m.Query(req)
	require.NoError(t, err)
	assert.NotEmpty(t, queryID)
}

// TestBatcherSizeFlush verifies the 4KB size half of the flush policy: a
// single 5KB line triggers an immediate chunk (no 250ms wait).
func TestBatcherSizeFlush(t *testing.T) {
	send, msgs, mu := collectSend()
	m := NewManager(uuid.New(), "/tmp/apps", fakeDocker{}, noopLogger{}, send)

	s := newStream("size-test", 1, "", m)
	big := domain.ContainerLogLine{Service: "svc", Text: string(make([]byte, flushSizeBytes+10))}
	s.addLine(big)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *msgs, 1, "size flush should emit immediately")
	assert.Equal(t, "container_log_chunk", (*msgs)[0].Event)
	var chunk domain.ContainerLogChunk
	require.NoError(t, json.Unmarshal((*msgs)[0].Payload, &chunk))
	assert.Len(t, chunk.Lines, 1)
	assert.Equal(t, "size-test", chunk.StreamID)
	assert.Equal(t, uint64(1), chunk.Seq)
}

// TestBatcherTimeFlush verifies the 250ms time half: lines below the size
// threshold still flush on the ticker.
func TestBatcherTimeFlush(t *testing.T) {
	send, msgs, mu := collectSend()
	m := NewManager(uuid.New(), "/tmp/apps", fakeDocker{}, noopLogger{}, send)

	s := newStream("time-test", 1, "", m)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	s.startFlusher(ctx)

	s.addLine(domain.ContainerLogLine{Service: "svc", Text: "small line"})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(*msgs) == 1
	}, 2*time.Second, 20*time.Millisecond, "time flush should emit within ~250ms")
}
