// Package metrics
package metrics

import (
	"context"
	"sync"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
	"horizonx-server/internal/logger"
)

type Service struct {
	repo domain.MetricsRepository
	bus  *event.Bus
	log  logger.Logger

	buffer   []domain.Metrics
	bufferMu sync.Mutex

	flushInterval time.Duration
	maxBatchSize  int
}

func NewService(repo domain.MetricsRepository, bus *event.Bus, log logger.Logger) domain.MetricsService {
	svc := &Service{
		repo:          repo,
		bus:           bus,
		log:           log,
		buffer:        make([]domain.Metrics, 0, 100),
		flushInterval: 5 * time.Second,
		maxBatchSize:  50,
	}

	go svc.backgroundFlusher()

	return svc
}

func (s *Service) Ingest(m domain.Metrics) error {
	s.bufferMu.Lock()
	s.buffer = append(s.buffer, m)
	bufferSize := len(s.buffer)
	s.bufferMu.Unlock()

	s.log.Debug("metric added to buffer", "buffer_size", bufferSize)

	if bufferSize >= s.maxBatchSize {
		s.log.Debug("buffer size reached, forcing flush", "size", bufferSize)
		go s.flush()
	}

	if s.bus != nil {
		s.bus.Publish("server_metrics_received", m)
	}

	return nil
}

func (s *Service) backgroundFlusher() {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.flush()
	}
}

func (s *Service) flush() {
	s.bufferMu.Lock()
	if len(s.buffer) == 0 {
		s.bufferMu.Unlock()
		return
	}

	batch := make([]domain.Metrics, len(s.buffer))
	copy(batch, s.buffer)
	s.buffer = s.buffer[:0]
	s.bufferMu.Unlock()

	s.log.Info("flushing metrics to database", "count", len(batch))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.repo.BulkInsert(ctx, batch); err != nil {
		s.log.Error("failed to bulk insert metrics", "error", err, "count", len(batch))
		return
	}

	s.log.Debug("metrics flushed successfully", "count", len(batch))
}
