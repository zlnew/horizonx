package gpu

import (
	"zlnew/monitor-agent/internal/core"
	"zlnew/monitor-agent/internal/infra/logger"
)

type Collector struct {
	log         logger.Logger
	powerEMA    map[string]*core.EMA
	usageEMA    map[string]*core.EMA
	fanSpeedEMA map[string]*core.EMA
}

type GPUMetric = core.GPUMetric
