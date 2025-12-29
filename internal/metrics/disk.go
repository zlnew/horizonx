package metrics

import (
	"time"

	"horizonx-server/internal/system"
)

func (c *Collector) calculateDiskDelta(name string, curr system.DiskIOStats, now time.Time) (readMBps, writeMBps, utilPct float64) {
	last, ok := c.lastDiskIO[name]

	c.lastDiskIO[name] = DiskIOState{
		ReadBytes:    curr.ReadBytes,
		WriteBytes:   curr.WriteBytes,
		IOTimeMillis: curr.IOTimeMillis,
		Time:         now,
	}

	if !ok {
		return 0, 0, 0
	}

	dt := now.Sub(last.Time).Seconds()
	if dt <= 0 {
		return 0, 0, 0
	}

	readMBps = float64(curr.ReadBytes-last.ReadBytes) / 1024 / 1024 / dt
	writeMBps = float64(curr.WriteBytes-last.WriteBytes) / 1024 / 1024 / dt

	utilPct = float64(curr.IOTimeMillis-last.IOTimeMillis) / (dt * 10)
	if utilPct > 100 {
		utilPct = 100
	}

	return
}
