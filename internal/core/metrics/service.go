package metrics

import (
	"context"
	"fmt"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
	"horizonx-server/internal/storage/snapshot"
	"horizonx-server/internal/transport/websocket"
)

type Service struct {
	repo      domain.MetricsRepository
	hub       *websocket.Hub
	snapshot  *snapshot.MetricsStore
	log       logger.Logger
	saveQueue chan domain.Metrics
}

func NewService(repo domain.MetricsRepository, snapshot *snapshot.MetricsStore, hub *websocket.Hub, log logger.Logger) domain.MetricsService {
	s := &Service{
		repo:      repo,
		hub:       hub,
		snapshot:  snapshot,
		log:       log,
		saveQueue: make(chan domain.Metrics, 1000),
	}

	go s.worker()
	return s
}

func (s *Service) Ingest(ctx context.Context, m domain.Metrics) error {
	if m.RecordedAt.IsZero() {
		m.RecordedAt = time.Now().UTC()
	}

	s.snapshot.Set(m.ServerID, m)

	channel := fmt.Sprintf("server:%d:metrics", m.ServerID)
	event := "metrics.updated"

	s.hub.Emit(channel, event, m)

	select {
	case s.saveQueue <- m:
	default:
		s.log.Warn("metrics queue full! dropping data.")
	}

	return nil
}

func (s *Service) worker() {
	buffer := make([]domain.Metrics, 0, 100)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	flush := func() {
		if len(buffer) > 0 {
			if err := s.repo.BulkInsert(context.Background(), buffer); err != nil {
				s.log.Error("failed to flush metrics to DB", err)
			}
			buffer = buffer[:0]
		}
	}

	for {
		select {
		case m := <-s.saveQueue:
			buffer = append(buffer, m)
			if len(buffer) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
