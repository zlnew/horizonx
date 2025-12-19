package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type MetricsReporter struct {
	client  *http.Client
	baseURL string
	token   string
	log     logger.Logger

	buffer    []domain.Metrics
	bufferMu  sync.Mutex
	maxBuffer int
}

func NewMetricsReporter(baseURL, token string, log logger.Logger) *MetricsReporter {
	return &MetricsReporter{
		client:    &http.Client{Timeout: 10 * time.Second},
		baseURL:   baseURL,
		token:     token,
		log:       log,
		buffer:    make([]domain.Metrics, 0, 5),
		maxBuffer: 5,
	}
}

func (r *MetricsReporter) Add(metrics domain.Metrics) {
	r.bufferMu.Lock()
	defer r.bufferMu.Unlock()

	r.buffer = append(r.buffer, metrics)
	r.log.Debug("metrics buffered", "buffer_size", len(r.buffer))
}

func (r *MetricsReporter) ShouldFlush() bool {
	r.bufferMu.Lock()
	defer r.bufferMu.Unlock()

	return len(r.buffer) >= r.maxBuffer
}

func (r *MetricsReporter) Flush(ctx context.Context) error {
	r.bufferMu.Lock()
	if len(r.buffer) == 0 {
		r.bufferMu.Unlock()
		r.log.Debug("no metrics to flush")
		return nil
	}

	batch := make([]domain.Metrics, len(r.buffer))
	copy(batch, r.buffer)
	r.buffer = r.buffer[:0]
	r.bufferMu.Unlock()

	r.log.Info("flushing metrics batch", "size", len(batch))

	return r.sendBatch(ctx, batch)
}

func (r *MetricsReporter) sendBatch(ctx context.Context, batch []domain.Metrics) error {
	payload := map[string]any{
		"metrics": batch,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		r.log.Error("failed to marshal metrics", "error", err)
		return err
	}

	maxRetries := 3
	backoff := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			r.log.Debug("retrying metrics send", "attempt", attempt+1)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, "POST", r.baseURL+"/agent/metrics", bytes.NewReader(body))
		if err != nil {
			r.log.Error("failed to create request", "error", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+r.token)

		resp, err := r.client.Do(req)
		if err != nil {
			r.log.Error("failed to send metrics", "error", err, "attempt", attempt+1)
			continue
		}

		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			r.log.Info("metrics sent successfully", "count", len(batch), "status", resp.StatusCode)
			return nil
		}

		r.log.Warn("server returned error", "status", resp.StatusCode, "attempt", attempt+1)

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("authentication failed: %d", resp.StatusCode)
		}
	}

	return fmt.Errorf("failed to send metrics after %d attempts", maxRetries)
}

func (r *MetricsReporter) BufferSize() int {
	r.bufferMu.Lock()
	defer r.bufferMu.Unlock()
	return len(r.buffer)
}
