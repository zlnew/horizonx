package network

import (
	"context"

	"zlnew/monitor-agent/internal/core"
	"zlnew/monitor-agent/internal/infra/logger"
)

func NewCollector(log logger.Logger) *Collector {
	return &Collector{
		log:        log,
		rxSpeedEMA: core.NewEMA(0.5),
		txSpeedEMA: core.NewEMA(0.5),
	}
}
func (c *Collector) Collect(ctx context.Context) (NetworkMetric, error) {
	return c.collectMetric()
}
