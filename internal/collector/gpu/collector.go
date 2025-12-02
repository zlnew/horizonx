// Package gpu
package gpu

import "context"

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) Collect(ctx context.Context) (any, error) {
	usage := readUsage()
	temp, _ := readTemp()
	vramTotal := readVramTotal()
	vramUsed := readVramUsed()
	watt, _ := readPower()
	spec, _ := readSpec()

	return GPUMetric{
		Spec:      spec,
		Usage:     usage,
		Temp:      temp,
		VramTotal: vramTotal,
		VramUsed:  vramUsed,
		Watt:      watt,
	}, nil
}
