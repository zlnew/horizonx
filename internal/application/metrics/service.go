// Package metrics
package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"horizonx/internal/adapters/redis"
	"horizonx/internal/domain"
	"horizonx/internal/event"
	"horizonx/internal/logger"

	"github.com/google/uuid"
)

type Service struct {
	repo     domain.MetricsRepository
	registry *redis.Registry

	bus *event.Bus
	log logger.Logger

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

func NewService(repo domain.MetricsRepository, registry *redis.Registry, bus *event.Bus, log logger.Logger) domain.MetricsService {
	svc := &Service{
		repo:     repo,
		registry: registry,

		bus: bus,
		log: log,

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

func (s *Service) Ingest(ctx context.Context, m domain.Metrics) error {
	sid := m.ServerID
	at := m.RecordedAt

	s.recordLatest(ctx, m)
	s.recordCPUUsage(ctx, sid, m.CPU.Usage.EMA, at)
	s.recordNetSpeed(ctx, sid, m.Network.RXSpeedMBs.EMA, m.Network.TXSpeedMBs.EMA, at)

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

func (s *Service) Latest(ctx context.Context, serverID uuid.UUID) (*domain.Metrics, error) {
	s.latestMu.Lock()
	if metrics, ok := s.latest[serverID]; ok {
		s.latestMu.Unlock()
		return &metrics, nil
	}
	s.latestMu.Unlock()

	if msg, err := s.registry.GetLatest(ctx, fmt.Sprintf("metrics:server:%s:latest", serverID.String())); err == nil {
		if metrics, _, err := redis.ParseStreamMessages[domain.Metrics](msg); len(metrics) == 1 && err == nil {
			return &metrics[0], nil
		}
	}

	return nil, domain.ErrMetricsNotFound
}

func (s *Service) CPUUsageHistory(ctx context.Context, serverID uuid.UUID) ([]domain.CPUUsageSample, error) {
	s.cpuUsageMu.Lock()
	usages, ok := s.cpuUsageHistory[serverID]
	s.cpuUsageMu.Unlock()
	if ok {
		return usages, nil
	}

	msg, err := s.registry.GetRangeDesc(ctx, fmt.Sprintf("metrics:server:%s:cpu_usage", serverID.String()), 900)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU usage from registry: %w", err)
	}

	usages, _, err = redis.ParseStreamMessages[domain.CPUUsageSample](msg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CPU usage messages: %w", err)
	}
	if len(usages) == 0 {
		return nil, domain.ErrMetricsNotFound
	}

	s.cpuUsageMu.Lock()
	s.cpuUsageHistory[serverID] = usages
	s.cpuUsageMu.Unlock()

	return usages, nil
}

func (s *Service) NetSpeedHistory(ctx context.Context, serverID uuid.UUID) ([]domain.NetworkSpeedSample, error) {
	s.netSpeedMu.Lock()
	speeds, ok := s.netSpeedHistory[serverID]
	s.netSpeedMu.Unlock()
	if ok {
		return speeds, nil
	}

	msg, err := s.registry.GetRangeDesc(ctx, fmt.Sprintf("metrics:server:%s:net_speed", serverID.String()), 900)
	if err != nil {
		return nil, fmt.Errorf("failed to get net speed from registry: %w", err)
	}

	speeds, _, err = redis.ParseStreamMessages[domain.NetworkSpeedSample](msg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse net speed messages: %w", err)
	}
	if len(speeds) == 0 {
		return nil, domain.ErrMetricsNotFound
	}

	s.netSpeedMu.Lock()
	s.netSpeedHistory[serverID] = speeds
	s.netSpeedMu.Unlock()

	return speeds, nil
}

func (s *Service) Cleanup(ctx context.Context, serverID uuid.UUID, cutoff time.Time) error {
	return s.repo.Cleanup(ctx, serverID, cutoff)
}

func (s *Service) recordLatest(ctx context.Context, m domain.Metrics) {
	s.latestMu.Lock()
	defer s.latestMu.Unlock()

	s.latest[m.ServerID] = m
	s.registry.Append(ctx, fmt.Sprintf("metrics:server:%s:latest", m.ServerID.String()), m, 1)
}

func (s *Service) recordCPUUsage(ctx context.Context, serverID uuid.UUID, usage float64, at time.Time) {
	sample := domain.CPUUsageSample{
		UsagePercent: usage,
		At:           at,
	}

	s.cpuUsageMu.Lock()
	cpuPoints := s.cpuUsageHistory[serverID]
	cpuPoints = append(cpuPoints, sample)

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

	if _, err := s.registry.Append(ctx, fmt.Sprintf("metrics:server:%s:cpu_usage", serverID.String()), &sample, 900); err != nil {
		s.log.Error("failed to append CPU sample to registry", "serverID", serverID, "err", err)
	}
}

func (s *Service) recordNetSpeed(ctx context.Context, serverID uuid.UUID, rxMBs float64, txMBs float64, at time.Time) {
	sample := domain.NetworkSpeedSample{
		RXMBs: rxMBs,
		TXMBs: txMBs,
		At:    at,
	}

	s.netSpeedMu.Lock()
	netPoints := s.netSpeedHistory[serverID]
	netPoints = append(netPoints, sample)

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

	if _, err := s.registry.Append(ctx, fmt.Sprintf("metrics:server:%s:net_speed", serverID.String()), &sample, 900); err != nil {
		s.log.Error("failed to append net speed sample to registry", "serverID", serverID, "err", err)
	}
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
