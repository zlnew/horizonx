package network

import (
	"time"

	"zlnew/monitor-agent/internal/core"
	"zlnew/monitor-agent/internal/infra/logger"
)

type Collector struct {
	log         logger.Logger
	lastRxBytes uint64
	lastTxBytes uint64
	lastTime    time.Time
	rxSpeedEMA  *core.EMA
	txSpeedEMA  *core.EMA
}

type NetworkMetric = core.NetworkMetric
