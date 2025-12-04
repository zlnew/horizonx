// Package cpu
package cpu

import (
	"context"

	"zlnew/monitor-agent/internal/core"
)

func NewCollector() *Collector {
	return &Collector{
		powerEMA: core.NewEMA(0.3),
	}
}

func (c *Collector) Collect(ctx context.Context) (CPUMetric, error) {
	usage, perCore := readUsage()
	temperature := readTemperature()
	frequency := readFrequency()
	powerWatt := c.readPowerWatt()

	return CPUMetric{
		Usage:       usage,
		PerCore:     perCore,
		Temperature: temperature,
		Frequency:   frequency,
		PowerWatt:   powerWatt,
	}, nil
}
