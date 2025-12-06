package cpu

import (
	"time"

	"zlnew/monitor-agent/internal/core"
	"zlnew/monitor-agent/internal/infra/logger"
)

type Collector struct {
	log          logger.Logger
	lastEnergy   uint64
	lastTime     time.Time
	powerEMA     *core.EMA
	prevCPUStats map[string]cpuStat
	usageEMA     *core.EMA
	perCoreEMA   []*core.EMA
}

type cpuStat struct {
	total uint64
	idle  uint64
}

type CPUMetric = core.CPUMetric
