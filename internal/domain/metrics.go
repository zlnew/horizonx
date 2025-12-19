package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Metrics struct {
	ServerID      uuid.UUID     `json:"server_id"`
	OSInfo        OSInfo        `json:"os_info"`
	CPU           CPUMetric     `json:"cpu"`
	GPU           []GPUMetric   `json:"gpu"`
	Memory        MemoryMetric  `json:"memory"`
	Disk          []DiskMetric  `json:"disk"`
	Network       NetworkMetric `json:"network"`
	UptimeSeconds float64       `json:"uptime_seconds"`
	RecordedAt    time.Time     `json:"recorded_at"`
}

type OSInfo struct {
	Hostname      string `json:"hostname"`
	Name          string `json:"name"`
	KernelVersion string `json:"kernel_version"`
	Arch          string `json:"arch"`
}

type CPUMetric struct {
	Usage       float64   `json:"usage"`
	PerCore     []float64 `json:"per_core"`
	Temperature float64   `json:"temperature"`
	Frequency   float64   `json:"frequency"`
	PowerWatt   float64   `json:"power_watt"`
}

type GPUMetric struct {
	ID               int     `json:"id"`
	Card             string  `json:"card"`
	Vendor           string  `json:"vendor"`
	Model            string  `json:"model"`
	Temperature      float64 `json:"temperature"`
	CoreUsagePercent float64 `json:"core_usage_percent"`
	VRAMTotalGB      float64 `json:"vram_total_gb"`
	VRAMUsedGB       float64 `json:"vram_used_gb"`
	VRAMPercent      float64 `json:"vram_percent"`
	PowerWatt        float64 `json:"power_watt"`
	FanSpeedPercent  float64 `json:"fan_speed_percent"`
}

type MemoryMetric struct {
	TotalGB      float64 `json:"total_gb"`
	UsedGB       float64 `json:"used_gb"`
	UsagePercent float64 `json:"usage_percent"`
	AvailableGB  float64 `json:"available_gb"`
	SwapTotalGB  float64 `json:"swap_total_gb"`
	SwapFreeGB   float64 `json:"swap_free_gb"`
	SwapUsedGB   float64 `json:"swap_used_gb"`
}

type DiskMetric struct {
	Name        string            `json:"name"`
	RawSizeGB   float64           `json:"raw_size_gb"`
	Temperature float64           `json:"temperature"`
	Filesystems []FilesystemUsage `json:"filesystems"`
}

type FilesystemUsage struct {
	Device     string  `json:"device"`
	Mountpoint string  `json:"mountpoint"`
	TotalGB    float64 `json:"total_gb"`
	UsedGB     float64 `json:"used_gb"`
	FreeGB     float64 `json:"free_gb"`
	Percent    float64 `json:"percent"`
}

type NetworkMetric struct {
	RXBytes uint64  `json:"rx_bytes"`
	TXBytes uint64  `json:"tx_bytes"`
	RXSpeed float64 `json:"rx_speed"`
	TXSpeed float64 `json:"tx_speed"`
}

type MetricsPayload struct {
	Metrics []Metrics `json:"metrics" validate:"required,dive"`
}

type MetricsService interface {
	Ingest(m Metrics) error
}

type MetricsRepository interface {
	BulkInsert(ctx context.Context, metrics []Metrics) error
}
