// Package metrics
package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"horizonx/internal/adapters/redis"
	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/logger"
	"horizonx/internal/system"
)

type CPUPowerState struct {
	LastEnergyUJ uint64
	LastTime     time.Time
}

type CPUUsageState struct {
	Last map[string]system.CPUStat
}

type DiskIOState struct {
	ReadBytes    uint64
	WriteBytes   uint64
	IOTimeMillis uint64
	Time         time.Time
}

type NetState struct {
	RxBytes uint64
	TxBytes uint64
	Time    time.Time
}

type Collector struct {
	cfg *config.Config
	log logger.Logger

	registry       *redis.Registry
	registryKey    string
	registryMaxLen int64

	buffer   []domain.Metrics
	bufferMu sync.Mutex

	maxSamples int
	interval   time.Duration

	reader *system.SystemReader

	cpuPowerState CPUPowerState
	cpuUsageState CPUUsageState
	lastDiskIO    map[string]DiskIOState
	lastNet       map[string]NetState

	cpuUsageEMA   *EMA
	cpuFreqEMA    *EMA
	cpuPowerEMA   *EMA
	cpuTempEMA    *EMA
	cpuPerCoreEMA map[string]*EMA

	gpuUsageEMA map[string]*EMA
	gpuClockEMA map[string]*EMA
	gpuPowerEMA map[string]*EMA
	gpuTempEMA  map[string]*EMA

	diskReadEMA  map[string]*EMA
	diskWriteEMA map[string]*EMA
	diskUtilEMA  map[string]*EMA
	diskTempEMA  map[string]*EMA

	netRxEMA map[string]*EMA
	netTxEMA map[string]*EMA

	iface string
}

func NewCollector(cfg *config.Config, log logger.Logger, registry *redis.Registry) *Collector {
	return &Collector{
		cfg: cfg,
		log: log,

		registry:       registry,
		registryKey:    fmt.Sprintf("metrics:agent:%s:stream", cfg.AgentServerID.String()),
		registryMaxLen: 5000,

		buffer:     make([]domain.Metrics, 0, 10),
		maxSamples: 10,
		interval:   5 * time.Second,

		reader: system.NewReader(log),

		lastDiskIO: make(map[string]DiskIOState),
		lastNet:    make(map[string]NetState),

		cpuUsageEMA:   NewEMA(15 * time.Second),
		cpuFreqEMA:    NewEMA(20 * time.Second),
		cpuPowerEMA:   NewEMA(20 * time.Second),
		cpuTempEMA:    NewEMA(30 * time.Second),
		cpuPerCoreEMA: map[string]*EMA{},

		gpuUsageEMA: map[string]*EMA{},
		gpuClockEMA: map[string]*EMA{},
		gpuPowerEMA: map[string]*EMA{},
		gpuTempEMA:  map[string]*EMA{},

		diskReadEMA:  map[string]*EMA{},
		diskWriteEMA: map[string]*EMA{},
		diskUtilEMA:  map[string]*EMA{},
		diskTempEMA:  map[string]*EMA{},

		netRxEMA: map[string]*EMA{},
		netTxEMA: map[string]*EMA{},
	}
}

func (c *Collector) Start(ctx context.Context) error {
	if err := c.loadBufferFromRegistry(ctx); err != nil {
		c.log.Error("failed to load initial metrics from registry", "err", err)
	}

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.log.Info("metrics collector started")

	for {
		select {
		case <-ctx.Done():
			c.flushBufferToRegistry()
			c.log.Info("metrics collector stopped")
			return nil
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

func (c *Collector) Latest() *domain.Metrics {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	if len(c.buffer) == 0 {
		return &domain.Metrics{}
	}

	m := c.buffer[len(c.buffer)-1]
	return &m
}

func (c *Collector) flushBufferToRegistry() {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	if len(c.buffer) == 0 {
		return
	}

	c.log.Info("flushing buffered metrics to registry")

	for _, m := range c.buffer {
		if _, err := c.registry.Append(context.Background(), c.registryKey, &m, 5000); err != nil {
			c.log.Error("failed to flush buffered metrics to registry", "err", err)
		}
	}

	c.buffer = c.buffer[:0]
}

func (c *Collector) loadBufferFromRegistry(ctx context.Context) error {
	msg, err := c.registry.GetRangeDesc(ctx, c.registryKey, int64(c.maxSamples))
	if err != nil {
		return err
	}

	metrics, _, err := redis.ParseStreamMessages[domain.Metrics](msg)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		return nil
	}

	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	if len(metrics) > c.maxSamples {
		metrics = metrics[len(metrics)-c.maxSamples:]
	}
	c.buffer = append(c.buffer, metrics...)
	c.log.Info("loaded initial metrics from registry", "count", len(c.buffer))

	return nil
}

func (c *Collector) collect(ctx context.Context) {
	c.bufferMu.Lock()

	var metrics domain.Metrics

	metrics.ServerID = c.cfg.AgentServerID
	metrics.CPU = c.getCPUMetric()
	metrics.GPU = c.getGPUMetrics()
	metrics.Memory = c.getMemoryMetric()
	metrics.Disk = c.getDiskMetrics()
	metrics.Network = c.getNetworkMetric()
	metrics.UptimeSeconds = c.reader.Uptime()
	metrics.RecordedAt = time.Now().UTC()

	c.ApplyEMA(&metrics)

	if len(c.buffer) >= c.maxSamples {
		c.buffer = c.buffer[1:]
	}
	c.buffer = append(c.buffer, metrics)

	c.bufferMu.Unlock()

	if _, err := c.registry.Append(ctx, c.registryKey, &metrics, 5000); err != nil {
		c.log.Error("failed to append metrics", "err", err)
	}
}

func (c *Collector) getCPUMetric() domain.CPUMetric {
	now := time.Now()

	stats := c.reader.CPUCoreStats()

	coreAvg, perCore := calculateCPUUsage(&c.cpuUsageState, stats)

	temp := c.reader.CPUTempC()
	freq := c.reader.CPUFreqAvgMhz()

	watt := calculateCPUPowerWatt(
		&c.cpuPowerState,
		c.reader.CPUEnergyUJ(),
		now,
	)

	if watt < 0 || watt > 300 {
		watt = 0
	}

	cpu := domain.CPUMetric{
		PerCore: make([]domain.Signal, len(perCore)),
	}

	cpu.Usage.Raw = coreAvg
	cpu.Temperature.Raw = temp
	cpu.Frequency.Raw = freq
	cpu.PowerWatt.Raw = watt

	for i, v := range perCore {
		cpu.PerCore[i].Raw = v
	}

	return cpu
}

func (c *Collector) getGPUMetrics() []domain.GPUMetric {
	var gpus []domain.GPUMetric

	if m := c.reader.NvidiaGPU(); m != nil {
		gpus = append(gpus, calculateGPUMetric("gpu0", "nvidia", m))
	}

	for _, card := range c.reader.ListDRMCards() {
		if m := c.reader.AMDGPU(card); m != nil {
			gpus = append(gpus, calculateGPUMetric(card, "amd", m))
			continue
		}

		if m := c.reader.IntelGPU(card); m != nil {
			gpus = append(gpus, calculateGPUMetric(card, "intel", m))
		}
	}

	return gpus
}

func (c *Collector) getMemoryMetric() domain.MemoryMetric {
	stats := c.reader.Memory()

	const kbToGB = 1024 * 1024

	m := domain.MemoryMetric{
		TotalGB:     float64(stats.MemTotalKB) / kbToGB,
		AvailableGB: float64(stats.MemAvailableKB) / kbToGB,
		UsedGB:      float64(stats.MemUsedKB) / kbToGB,
		SwapTotalGB: float64(stats.SwapTotalKB) / kbToGB,
		SwapFreeGB:  float64(stats.SwapFreeKB) / kbToGB,
		SwapUsedGB:  float64(stats.SwapUsedKB) / kbToGB,
	}

	if m.TotalGB > 0 {
		m.UsagePercent = m.UsedGB / m.TotalGB * 100
	}

	return m
}

func (c *Collector) getDiskMetrics() []domain.DiskMetric {
	now := time.Now()

	rawDisks := c.reader.Disks()
	result := make([]domain.DiskMetric, 0, len(rawDisks))

	for _, d := range rawDisks {
		dm := domain.DiskMetric{
			Name:        d.Name,
			RawSizeGB:   float64(d.RawBytes) / (1024 * 1024 * 1024),
			Filesystems: []domain.FilesystemUsage{},
		}

		dm.Temperature.Raw = d.Temperature

		io := c.reader.DiskIO(d.Name)

		dm.ReadMBps.Raw,
			dm.WriteMBps.Raw,
			dm.UtilPct.Raw = c.calculateDiskDelta(d.Name, io, now)

		for _, fs := range d.Filesystems {
			totalGB := float64(fs.TotalBytes) / (1024 * 1024 * 1024)
			usedGB := float64(fs.UsedBytes) / (1024 * 1024 * 1024)
			freeGB := float64(fs.FreeBytes) / (1024 * 1024 * 1024)

			percent := 0.0
			if totalGB > 0 {
				percent = usedGB / totalGB * 100
			}

			dm.Filesystems = append(dm.Filesystems, domain.FilesystemUsage{
				Device:     fs.Device,
				Mountpoint: fs.Mountpoint,
				TotalGB:    totalGB,
				UsedGB:     usedGB,
				FreeGB:     freeGB,
				Percent:    percent,
			})
		}

		result = append(result, dm)
	}

	return result
}

func (c *Collector) getNetworkMetric() domain.NetworkMetric {
	if c.iface == "" {
		c.iface = c.reader.DefaultInterface()
	}

	if c.iface == "" {
		return domain.NetworkMetric{}
	}

	now := time.Now()
	curr := c.reader.NetBytes(c.iface)

	last, ok := c.lastNet[c.iface]

	c.lastNet[c.iface] = NetState{
		RxBytes: curr.RxBytes,
		TxBytes: curr.TxBytes,
		Time:    now,
	}

	m := domain.NetworkMetric{
		RXBytes: curr.RxBytes,
		TXBytes: curr.TxBytes,
	}

	if !ok {
		return m
	}

	dt := now.Sub(last.Time).Seconds()
	if dt <= 0 {
		return m
	}

	m.RXSpeedMBs.Raw = float64(curr.RxBytes-last.RxBytes) / 1024 / 1024 / dt
	m.TXSpeedMBs.Raw = float64(curr.TxBytes-last.TxBytes) / 1024 / 1024 / dt

	if m.RXSpeedMBs.Raw < 0 {
		m.RXSpeedMBs.Raw = 0
	}
	if m.TXSpeedMBs.Raw < 0 {
		m.TXSpeedMBs.Raw = 0
	}

	return m
}
