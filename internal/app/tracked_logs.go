package app

import (
	"context"
	"sync"

	"horizonx/internal/domain"
)

// trackedContainerLogs wraps the application service's ContainerLogService
// methods and records the stream ID each tail started. The server's hub
// channel-empty handler (app_logs:{id}) uses the map to stop the right
// stream when the browser tab closes.
type trackedContainerLogs struct {
	inner domain.ContainerLogService

	mu      *sync.Mutex
	streams map[int64]string
}

func (t *trackedContainerLogs) TailLogs(ctx context.Context, appID int64, req domain.LogsTailRequest) (string, error) {
	streamID, err := t.inner.TailLogs(ctx, appID, req)
	if err != nil {
		return "", err
	}

	t.mu.Lock()
	t.streams[appID] = streamID
	t.mu.Unlock()
	return streamID, nil
}

func (t *trackedContainerLogs) StopTailLogs(ctx context.Context, appID int64, streamID string) error {
	t.mu.Lock()
	delete(t.streams, appID)
	t.mu.Unlock()
	return t.inner.StopTailLogs(ctx, appID, streamID)
}

func (t *trackedContainerLogs) QueryLogs(ctx context.Context, appID int64, req domain.LogsQueryRequest) (string, error) {
	return t.inner.QueryLogs(ctx, appID, req)
}
