package gpu

import "zlnew/monitor-agent/internal/core"

type Collector struct {
	powerEMA map[string]*core.EMA
}

type GPUMetric = core.GPUMetric
