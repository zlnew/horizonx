// Package metrics
package metrics

import (
	"context"
	"sync"
	"time"

	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
	"horizonx-server/internal/system"
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

	buffer   []domain.Metrics
	bufferMu sync.Mutex

	stateMu sync.Mutex

	maxSamples int
	interval   time.Duration

	reader *system.SystemReader

	cpuPowerState CPUPowerState
	cpuUsageState CPUUsageState
	lastDiskIO    map[string]DiskIOState
	lastNet       map[string]NetState

	cachedIface string
}

func NewCollector(cfg *config.Config, log logger.Logger) *Collector {
	return &Collector{
		cfg: cfg,
		log: log,

		buffer:     make([]domain.Metrics, 0, 10),
		maxSamples: 10,
		interval:   5 * time.Second,

		reader: system.NewReader(log),

		lastDiskIO: make(map[string]DiskIOState),
		lastNet:    make(map[string]NetState),
	}
}

func (c *Collector) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.log.Info("metrics collector started")

	for {
		select {
		case <-ctx.Done():
			c.log.Info("metrics collector stropping...")
			return ctx.Err()
		case <-ticker.C:
			c.collect()
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

func (c *Collector) collect() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	var metrics domain.Metrics

	metrics.ServerID = c.cfg.AgentServerID
	metrics.CPU = c.getCPUMetric()
	metrics.GPU = c.getGPUMetrics()
	metrics.Memory = c.getMemoryMetric()
	metrics.Disk = c.getDiskMetrics()
	metrics.Network = c.getNetworkMetric()
	metrics.UptimeSeconds = c.reader.Uptime()
	metrics.RecordedAt = time.Now().UTC()

	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	if len(c.buffer) >= c.maxSamples {
		c.buffer = c.buffer[1:]
	}

	c.buffer = append(c.buffer, metrics)
}

func (c *Collector) getCPUMetric() domain.CPUMetric {
	var cpu domain.CPUMetric

	stats := c.reader.CPUCoreStats()

	cpu.Usage, cpu.PerCore = calculateCPUUsage(&c.cpuUsageState, stats)
	cpu.Temperature = c.reader.CPUTempC()
	cpu.Frequency = c.reader.CPUFreqAvgMhz()

	watt := calculateCPUPowerWatt(&c.cpuPowerState, c.reader.CPUEnergyUJ(), time.Now())
	if watt < 0 || watt > 300 {
		watt = 0
	}
	cpu.PowerWatt = watt

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
	var memory domain.MemoryMetric

	stats := c.reader.Memory()

	const kbToGB = 1024 * 1024

	memory.TotalGB = float64(stats.MemTotalKB) / kbToGB
	memory.AvailableGB = float64(stats.MemAvailableKB) / kbToGB
	memory.UsedGB = float64(stats.MemUsedKB) / kbToGB

	if memory.TotalGB > 0 {
		memory.UsagePercent = memory.UsedGB / memory.TotalGB * 100
	}

	memory.SwapTotalGB = float64(stats.SwapTotalKB) / kbToGB
	memory.SwapFreeGB = float64(stats.SwapFreeKB) / kbToGB
	memory.SwapUsedGB = float64(stats.SwapUsedKB) / kbToGB

	return memory
}

func (c *Collector) getDiskMetrics() []domain.DiskMetric {
	rawDisks := c.reader.Disks()
	result := make([]domain.DiskMetric, 0, len(rawDisks))

	now := time.Now()

	for _, d := range rawDisks {
		dm := domain.DiskMetric{
			Name:        d.Name,
			RawSizeGB:   float64(d.RawBytes) / (1024 * 1024 * 1024),
			Temperature: d.Temperature,
			Filesystems: []domain.FilesystemUsage{},
		}

		io := c.reader.DiskIO(d.Name)
		dm.ReadMBps, dm.WriteMBps, dm.UtilPct = c.calculateDiskDelta(d.Name, io, now)

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
	if c.cachedIface == "" {
		c.cachedIface = c.reader.DefaultInterface()
	}

	iface := c.cachedIface
	if iface == "" {
		return domain.NetworkMetric{}
	}

	curr := c.reader.NetBytes(iface)
	now := time.Now()

	last, ok := c.lastNet[iface]

	if curr.RxBytes == 0 && curr.TxBytes == 0 {
		c.cachedIface = ""
		return domain.NetworkMetric{}
	}

	c.lastNet[iface] = NetState{
		RxBytes: curr.RxBytes,
		TxBytes: curr.TxBytes,
		Time:    now,
	}

	metric := domain.NetworkMetric{
		RXBytes: curr.RxBytes,
		TXBytes: curr.TxBytes,
	}
	if !ok {
		return metric
	}

	dt := now.Sub(last.Time).Seconds()
	if dt <= 0 {
		return metric
	}

	metric.RXSpeedMBs = float64(curr.RxBytes-last.RxBytes) / 1024 / 1024 / dt
	metric.TXSpeedMBs = float64(curr.TxBytes-last.TxBytes) / 1024 / 1024 / dt

	if metric.RXSpeedMBs < 0 {
		metric.RXSpeedMBs = 0
	}

	if metric.TXSpeedMBs < 0 {
		metric.TXSpeedMBs = 0
	}

	return metric
}
