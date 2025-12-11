package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func NewService(repo domain.MetricsRepository, snapshot *snapshot.MetricsStore, hub *websocket.Hub, log logger.Logger) *Service {
	s := &Service{
		repo:      repo,
		hub:       hub,
		snapshot:  snapshot,
		log:       log,
		saveQueue: make(chan domain.Metrics, 1000),
	}

	go s.worker()
	go s.startEventProcessor()

	return s
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

func (s *Service) startEventProcessor() {
	s.log.Info("metrics event processor started, listening to hub events")

	for event := range s.hub.Events() {
		if !strings.HasSuffix(event.Channel, ":metrics") {
			continue
		}

		if event.Event != domain.EventMetricsReport {
			s.log.Info("ignoring non-report metrics event", "event", event.Event)
			continue
		}

		rawJSON, ok := event.Payload.(json.RawMessage)
		if !ok {
			s.log.Error("metric payload is not json.RawMessage", "type", fmt.Sprintf("%T", event.Payload))
			continue
		}

		rawPayload := []byte(rawJSON)

		var m domain.Metrics
		if err := json.Unmarshal(rawPayload, &m); err != nil {
			s.log.Error("failed to unmarshal domain.Metrics payload", "error", err)
			continue
		}

		if err := s.ingest(m); err != nil {
			s.log.Error("failed to process ingested metric", "error", err)
		}
	}
}

func (s *Service) ingest(m domain.Metrics) error {
	if m.RecordedAt.IsZero() {
		m.RecordedAt = time.Now().UTC()
	}

	s.snapshot.Set(m.ServerID, m)

	channel := domain.GetServerMetricsChannel(m.ServerID)
	s.hub.Emit(channel, domain.EventMetricsReceived, m)

	select {
	case s.saveQueue <- m:
	default:
		s.log.Warn("metrics queue full! dropping data.")
	}

	return nil
}
