// Package metrics
package metrics

import (
	"context"
	"sync"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
	"horizonx-server/internal/logger"

	"github.com/google/uuid"
)

type Service struct {
	repo domain.MetricsRepository
	bus  *event.Bus
	log  logger.Logger

	buffer []domain.Metrics
	latest map[uuid.UUID]domain.Metrics

	bufferMu sync.Mutex
	latestMu sync.Mutex
	flushMu  sync.Mutex

	cpuUsageHistory map[uuid.UUID][]domain.CPUUsageSample
	netSpeedHistory map[uuid.UUID][]domain.NetworkSpeedSample

	cpuUsageMu sync.RWMutex
	netSpeedMu sync.RWMutex

	cpuUsageHistoryRetention time.Duration
	netSpeedHistoryRetention time.Duration

	flushInterval     time.Duration
	broadcastInterval time.Duration

	batchSize int
}

func NewService(repo domain.MetricsRepository, bus *event.Bus, log logger.Logger) domain.MetricsService {
	svc := &Service{
		repo: repo,
		bus:  bus,
		log:  log,

		buffer: make([]domain.Metrics, 0, 50),
		latest: make(map[uuid.UUID]domain.Metrics),

		cpuUsageHistory: make(map[uuid.UUID][]domain.CPUUsageSample),
		netSpeedHistory: make(map[uuid.UUID][]domain.NetworkSpeedSample),

		cpuUsageHistoryRetention: 15 * time.Minute,
		netSpeedHistoryRetention: 15 * time.Minute,

		flushInterval:     15 * time.Second,
		broadcastInterval: 10 * time.Second,

		batchSize: 10,
	}

	go svc.backgroundFlusher()
	go svc.backgroundBroadcaster()

	return svc
}

func (s *Service) Ingest(m domain.Metrics) error {
	sid := m.ServerID
	at := m.RecordedAt

	s.updateLatest(m)
	s.recordCPUUsage(sid, m.CPU.Usage.EMA, at)
	s.recordNetSpeed(sid, m.Network.RXSpeedMBs.EMA, m.Network.TXSpeedMBs.EMA, at)

	s.bufferMu.Lock()
	s.buffer = append(s.buffer, m)
	bufferSize := len(s.buffer)
	s.bufferMu.Unlock()

	s.log.Debug("metrics added to buffer", "buffer_size", bufferSize)

	if bufferSize >= s.batchSize {
		s.log.Debug("buffer size reached, forcing flush", "size", bufferSize)
		go s.safeFlush()
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

func (s *Service) CPUUsageHistory(serverID uuid.UUID) ([]domain.CPUUsageSample, error) {
	s.cpuUsageMu.Lock()
	defer s.cpuUsageMu.Unlock()

	usages, ok := s.cpuUsageHistory[serverID]
	if !ok {
		return []domain.CPUUsageSample{}, domain.ErrMetricsNotFound
	}

	return usages, nil
}

func (s *Service) NetSpeedHistory(serverID uuid.UUID) ([]domain.NetworkSpeedSample, error) {
	s.netSpeedMu.Lock()
	defer s.netSpeedMu.Unlock()

	speeds, ok := s.netSpeedHistory[serverID]
	if !ok {
		return []domain.NetworkSpeedSample{}, domain.ErrMetricsNotFound
	}

	return speeds, nil
}

func (s *Service) Cleanup(ctx context.Context, serverID uuid.UUID, cutoff time.Time) error {
	return s.repo.Cleanup(ctx, serverID, cutoff)
}

func (s *Service) updateLatest(m domain.Metrics) {
	s.latestMu.Lock()
	s.latest[m.ServerID] = m
	s.latestMu.Unlock()
}

func (s *Service) recordCPUUsage(serverID uuid.UUID, usage float64, at time.Time) {
	s.cpuUsageMu.Lock()
	cpuPoints := s.cpuUsageHistory[serverID]
	cpuPoints = append(cpuPoints, domain.CPUUsageSample{
		UsagePercent: usage,
		At:           at,
	})

	cutoff := at.Add(-s.cpuUsageHistoryRetention)
	i := 0
	for ; i < len(cpuPoints); i++ {
		if cpuPoints[i].At.After(cutoff) {
			break
		}
	}
	cpuPoints = cpuPoints[i:]
	s.cpuUsageHistory[serverID] = cpuPoints
	s.cpuUsageMu.Unlock()
}

func (s *Service) recordNetSpeed(serverID uuid.UUID, rxMBs float64, txMBs float64, at time.Time) {
	s.netSpeedMu.Lock()
	netPoints := s.netSpeedHistory[serverID]
	netPoints = append(netPoints, domain.NetworkSpeedSample{
		RXMBs: rxMBs,
		TXMBs: txMBs,
		At:    at,
	})

	cutoff := at.Add(-s.netSpeedHistoryRetention)
	i := 0
	for ; i < len(netPoints); i++ {
		if netPoints[i].At.After(cutoff) {
			break
		}
	}
	netPoints = netPoints[i:]
	s.netSpeedHistory[serverID] = netPoints
	s.netSpeedMu.Unlock()
}

func (s *Service) backgroundFlusher() {
	ticker := time.NewTicker(s.flushInterval)
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
	s.buffer = make([]domain.Metrics, 0, s.batchSize)
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

func (s *Service) backgroundBroadcaster() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.broadcastLatest()
	}
}

func (s *Service) broadcastLatest() {
	s.latestMu.Lock()
	defer s.latestMu.Unlock()

	if s.bus == nil || len(s.latest) == 0 {
		return
	}

	for _, m := range s.latest {
		s.bus.Publish("server_metrics_received", m)
	}
	s.log.Debug("broadcasted latest metrics", "count", len(s.latest))
}
