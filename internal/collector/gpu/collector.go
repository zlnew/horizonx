// Package gpu
package gpu

import (
	"context"

	"zlnew/monitor-agent/internal/core"
)

func NewCollector() *Collector {
	return &Collector{
		powerEMA: make(map[string]*core.EMA),
	}
}

func (c *Collector) Collect(ctx context.Context) ([]GPUMetric, error) {
	cards := detectGPUs()
	var outputs []GPUMetric

	for i, card := range cards {
		if _, ok := c.powerEMA[card]; !ok {
			c.powerEMA[card] = core.NewEMA(0.3)
		}

		vendor := readVendor(card)
		model := readModel(card)

		temp := readTemperature(card)
		usage := readCoreUsage(card)
		vramTotal, vramUsed, vramPercent := readVRAM(card)
		powerWatt := c.readPower(card)
		fanSpeed := readFanSpeedPercent(card)

		outputs = append(outputs, GPUMetric{
			ID:               i,
			Card:             card,
			Vendor:           vendor,
			Model:            model,
			Temperature:      temp,
			CoreUsagePercent: usage,
			VRAMTotalGB:      vramTotal,
			VRAMUsedGB:       vramUsed,
			VRAMPercent:      vramPercent,
			PowerWatt:        powerWatt,
			FanSpeedPercent:  fanSpeed,
		})
	}

	return outputs, nil
}
