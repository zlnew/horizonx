// Package metrics
package metrics

import (
	"context"
	"sync"
	"time"

	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
	"horizonx-server/internal/logger"

	"github.com/google/uuid"
)

type Service struct {
	cfg  *config.Config
	repo domain.MetricsRepository
	bus  *event.Bus
	log  logger.Logger

	buffer   []domain.Metrics
	bufferMu sync.Mutex

	latest   map[uuid.UUID]domain.Metrics
	latestMu sync.Mutex

	flushMu sync.Mutex
}

func NewService(cfg *config.Config, repo domain.MetricsRepository, bus *event.Bus, log logger.Logger) domain.MetricsService {
	svc := &Service{
		cfg:    cfg,
		repo:   repo,
		bus:    bus,
		log:    log,
		buffer: make([]domain.Metrics, 0, cfg.MetricsBufferSize),
		latest: make(map[uuid.UUID]domain.Metrics),
	}

	go svc.backgroundFlusher()

	return svc
}

func (s *Service) Ingest(m domain.Metrics) error {
	s.latestMu.Lock()
	s.latest[m.ServerID] = m
	s.latestMu.Unlock()

	s.bufferMu.Lock()
	s.buffer = append(s.buffer, m)
	bufferSize := len(s.buffer)
	s.bufferMu.Unlock()

	s.log.Debug("metrics added to buffer", "buffer_size", bufferSize)

	if bufferSize >= s.cfg.MetricsBatchSize {
		s.log.Debug("buffer size reached, forcing flush", "size", bufferSize)
		go s.safeFlush()
	}

	if s.bus != nil {
		s.bus.Publish("server_metrics_received", m)
	}

	return nil
}

func (s *Service) Latest(serverID uuid.UUID) (*domain.Metrics, error) {
	s.latestMu.Lock()
	defer s.latestMu.Unlock()

	metrics, ok := s.latest[serverID]
	if !ok {
		return nil, domain.ErrMetricsNotFound
	}

	return &metrics, nil
}

func (s *Service) Cleanup(ctx context.Context, serverID uuid.UUID, cutoff time.Time) error {
	return s.repo.Cleanup(ctx, serverID, cutoff)
}

func (s *Service) backgroundFlusher() {
	ticker := time.NewTicker(s.cfg.MetricsFlushInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.safeFlush()
	}
}

func (s *Service) safeFlush() {
	if !s.flushMu.TryLock() {
		return
	}
	defer s.flushMu.Unlock()

	s.flush()
}

func (s *Service) flush() {
	s.bufferMu.Lock()
	if len(s.buffer) == 0 {
		s.bufferMu.Unlock()
		return
	}

	batch := s.buffer
	s.buffer = make([]domain.Metrics, 0, s.cfg.MetricsBatchSize)
	s.bufferMu.Unlock()

	s.log.Debug("flushing metrics to database", "count", len(batch))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.repo.BulkInsert(ctx, batch); err != nil {
		s.log.Error("failed to bulk insert metrics", "error", err, "count", len(batch))
		return
	}

	s.log.Debug("metrics flushed successfully", "count", len(batch))
}
