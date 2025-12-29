package metrics

import (
	"horizonx-server/internal/domain"
	"horizonx-server/internal/system"
)

func calculateGPUMetric(card string, vendor string, m *system.GPUMetrics) domain.GPUMetric {
	var gpu domain.GPUMetric

	if m == nil {
		return gpu
	}

	gpu.Card = card
	gpu.Vendor = vendor

	gpu.Temperature = m.TemperatureC
	gpu.CoreUsagePercent = m.UtilizationGPU
	gpu.FrequencyMhz = m.ClockMHz
	gpu.PowerWatt = m.PowerDrawW

	if m.MemTotalMB > 0 {
		gpu.VRAMTotalGB = float64(m.MemTotalMB) / 1024
		gpu.VRAMUsedGB = float64(m.MemUsedMB) / 1024
		gpu.VRAMPercent = float64(m.MemUsedMB) / float64(m.MemTotalMB) * 100
	}

	return gpu
}
