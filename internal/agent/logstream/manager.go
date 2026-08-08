// Package logstream owns the agent's `docker compose logs` streams (Plane B:
// container runtime logs). The host is the log store — HorizonX is the viewer.
//
// Safety model (non-negotiable):
//   - max 5 concurrent tails per agent, 1 per application (multiple viewers
//     of the same app share one --follow process via the hub's fan-out)
//   - 30-minute hard TTL per stream (backstop; the server also stops streams
//     when its app_logs channel empties)
//   - bounded backlog (query tail default 1000, hard max 5000 — an unbounded
//     `docker compose logs` on a month-old container would OOM the agent)
//   - batches flush every 250ms or 4KB, whichever first, with a monotonic Seq
//     per stream so the client can render a "N chunks dropped" marker on gaps
//     (the userws hub drops events silently under load by design)
package logstream

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"horizonx/internal/agent/command"
	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/google/uuid"
)

const (
	maxConcurrentTails = 5
	maxStreamTTL       = 30 * time.Minute
	flushInterval      = 250 * time.Millisecond
	flushSizeBytes     = 4 * 1024
	defaultBacklog     = 200
	maxQueryBacklog    = 5000
	defaultQueryTail   = 1000
)

// DockerRunner is the subset of docker.Manager the logstream needs.
type DockerRunner interface {
	GetDockerComposeFile(workDir string) (string, error)
}

// composeRunner runs the actual docker command. The default implementation
// shells out to docker; tests inject a fake to avoid exec + daemon timing.
type composeRunner func(ctx context.Context, workDir string, args []string, s *stream) error

type Manager struct {
	serverID uuid.UUID
	workDir  string
	docker   DockerRunner
	log      logger.Logger
	send     func(msg *domain.WsAgentMessage) error

	runCompose composeRunner

	mu        sync.Mutex
	streams   map[string]*stream // by stream ID
	appTails  map[int64]string   // applicationID -> streamID (1 tail per app)
	tailCount int
}

// NewManager creates the log stream manager. send is the callback that
// delivers an agent→server WS message (the agent's conn.sendMessage).
func NewManager(serverID uuid.UUID, workDir string, docker DockerRunner, log logger.Logger, send func(msg *domain.WsAgentMessage) error) *Manager {
	m := &Manager{
		serverID: serverID,
		workDir:  workDir,
		docker:   docker,
		log:      log,
		send:     send,
		streams:  make(map[string]*stream),
		appTails: make(map[int64]string),
	}
	m.runCompose = m.composeWithDocker
	return m
}

// StartTail launches a `docker compose logs --follow` for an application.
// Idempotent per application: if a tail for the same app is already running,
// it returns the existing stream ID (viewers share the one process).
func (m *Manager) StartTail(req domain.LogsTailStartPayload) (string, error) {
	m.mu.Lock()
	if existing, ok := m.appTails[req.ApplicationID]; ok {
		m.mu.Unlock()
		return existing, nil
	}
	if m.tailCount >= maxConcurrentTails {
		m.mu.Unlock()
		return "", fmt.Errorf("max concurrent tails reached (%d)", maxConcurrentTails)
	}

	streamID := req.StreamID
	if streamID == "" {
		streamID = fmt.Sprintf("tail-%d-%d", req.ApplicationID, time.Now().UnixNano())
	}

	s := newStream(streamID, req.ApplicationID, req.Service, m)
	m.streams[streamID] = s
	m.appTails[req.ApplicationID] = streamID
	m.tailCount++
	m.mu.Unlock()

	backlog := req.Tail
	if backlog <= 0 {
		backlog = defaultBacklog
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxStreamTTL)
	s.cancel = cancel
	s.startFlusher(ctx)

	go func() {
		args := []string{"logs", "--follow", "--timestamps", "--tail", fmt.Sprintf("%d", backlog)}
		if req.Service != "" {
			args = append(args, req.Service)
		}
		err := m.runCompose(ctx, req.DirName, args, s)
		s.finish(err)
	}()

	return streamID, nil
}

// StopTail cancels a running tail stream by ID (no-op if already gone).
func (m *Manager) StopTail(streamID string) {
	m.mu.Lock()
	s, ok := m.streams[streamID]
	m.mu.Unlock()
	if !ok {
		return
	}
	s.cancel()
}

// Query runs a one-shot `docker compose logs` (no --follow) and streams the
// result with EOF:true when done. Bound by Tail (default 1000, hard max 5000).
func (m *Manager) Query(req domain.LogsQueryPayload) (string, error) {
	queryID := req.QueryID
	if queryID == "" {
		queryID = fmt.Sprintf("query-%d-%d", req.ApplicationID, time.Now().UnixNano())
	}

	backlog := req.Tail
	if backlog <= 0 {
		backlog = defaultQueryTail
	}
	if backlog > maxQueryBacklog {
		backlog = maxQueryBacklog
	}

	s := newStream(queryID, req.ApplicationID, req.Service, m)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.cancel = cancel
	s.startFlusher(ctx)

	go func() {
		args := []string{"logs", "--timestamps", "--tail", fmt.Sprintf("%d", backlog)}
		if req.Since != nil && *req.Since != "" {
			args = append(args, "--since", *req.Since)
		}
		if req.Until != nil && *req.Until != "" {
			args = append(args, "--until", *req.Until)
		}
		if req.Service != "" {
			args = append(args, req.Service)
		}
		err := m.runCompose(ctx, req.DirName, args, s)
		s.finish(err)
	}()

	return queryID, nil
}

// deregister removes a finished stream from the manager's bookkeeping so the
// caps and maps don't leak. Called from stream.finish (once).
func (m *Manager) deregister(s *stream) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.streams[s.id]; !ok {
		return
	}
	delete(m.streams, s.id)
	if m.appTails[s.appID] == s.id {
		delete(m.appTails, s.appID)
	}
	m.tailCount--
}

func (m *Manager) composeWithDocker(ctx context.Context, dirName string, args []string, s *stream) error {
	workDir := filepath.Join(m.workDir, dirName)
	composeFile, err := m.docker.GetDockerComposeFile(workDir)
	if err != nil {
		return fmt.Errorf("resolve compose file: %w", err)
	}

	fullArgs := append([]string{"compose", "-f", composeFile}, args...)
	cmd := command.NewCommand(workDir, "docker", fullArgs...)

	return cmd.Stream(ctx, func(line string, stream domain.LogStream, level domain.LogLevel) {
		s.addLine(parseLogLine(line, string(stream)))
	})
}

// parseLogLine parses `docker compose logs --timestamps` output:
//
//	app-1  | 2023-05-01T12:00:00.000000000Z hello world
//
// It tolerates lines without the service prefix (docker compose emits some
// startup noise outside the prefix format).
func parseLogLine(line, stream string) domain.ContainerLogLine {
	out := domain.ContainerLogLine{Stream: stream, Text: line}

	// Split the compose "service  | " prefix.
	if idx := strings.Index(line, " | "); idx >= 0 {
		out.Service = strings.TrimSpace(line[:idx])
		rest := line[idx+3:]

		// Timestamp is the first whitespace-delimited token when present.
		rest = strings.TrimSpace(rest)
		if sp := strings.IndexAny(rest, " \t"); sp > 0 {
			maybe := rest[:sp]
			if looksLikeTimestamp(maybe) {
				out.Timestamp = maybe
				out.Text = strings.TrimSpace(rest[sp:])
				return out
			}
		}
		out.Text = rest
	}
	return out
}

func looksLikeTimestamp(s string) bool {
	if len(s) < 11 {
		return false
	}
	// RFC3339 / Docker's timestamp form: YYYY-MM-DD...
	return s[4] == '-' && s[7] == '-' && (s[10] == 'T' || s[10] == ' ')
}

// stream is one running log stream with its batcher + seq counter.
type stream struct {
	id      string
	appID   int64
	service string

	manager *Manager

	cancel context.CancelFunc
	done   chan struct{}

	mu        sync.Mutex
	batch     []domain.ContainerLogLine
	batchSize int
	seq       uint64
	finished  bool
}

func newStream(id string, appID int64, service string, manager *Manager) *stream {
	return &stream{
		id:      id,
		appID:   appID,
		service: service,
		manager: manager,
		done:    make(chan struct{}),
	}
}

func (s *stream) addLine(line domain.ContainerLogLine) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.batch = append(s.batch, line)
	s.batchSize += len(line.Text)

	if s.batchSize >= flushSizeBytes {
		s.flushLocked()
	}
}

// startFlusher runs the time half of the flush policy (250ms ticker, size
// half handled in addLine). Exits when the stream finishes or ctx is done.
func (s *stream) startFlusher(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.done:
				return
			case <-ticker.C:
				s.mu.Lock()
				s.flushLocked()
				s.mu.Unlock()
			}
		}
	}()
}

// finish flushes any remaining lines, sends the EOF chunk, and deregisters.
func (s *stream) finish(err error) {
	s.mu.Lock()
	s.flushLocked()

	chunk := domain.ContainerLogChunk{
		StreamID:      s.id,
		ApplicationID: s.appID,
		Seq:           s.seq,
		EOF:           true,
	}
	if err != nil {
		chunk.Error = err.Error()
		s.manager.log.Warn("log stream finished with error", "stream_id", s.id, "error", err)
	}
	s.sendChunkLocked(chunk)
	s.finished = true
	s.mu.Unlock()

	s.manager.deregister(s)
	close(s.done)
}

func (s *stream) flushLocked() {
	if len(s.batch) == 0 {
		return
	}

	s.seq++
	s.sendChunkLocked(domain.ContainerLogChunk{
		StreamID:      s.id,
		ApplicationID: s.appID,
		Seq:           s.seq,
		Lines:         s.batch,
	})

	s.batch = nil
	s.batchSize = 0
}

func (s *stream) sendChunkLocked(chunk domain.ContainerLogChunk) {
	payload, err := json.Marshal(chunk)
	if err != nil {
		s.manager.log.Error("failed to marshal log chunk", "error", err)
		return
	}

	if s.manager.send == nil {
		return
	}
	_ = s.manager.send(&domain.WsAgentMessage{
		ServerID: s.manager.serverID,
		Event:    "container_log_chunk",
		Payload:  payload,
	})
}
