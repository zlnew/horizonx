package system

import (
	"os"
	"strconv"
	"strings"
)

type MemoryStats struct {
	MemTotalKB     uint64
	MemAvailableKB uint64
	MemUsedKB      uint64

	SwapTotalKB uint64
	SwapFreeKB  uint64
	SwapUsedKB  uint64
}

func (r *SystemReader) Memory() MemoryStats {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		r.log.Debug("failed to read meminfo", "error", err.Error())
		return MemoryStats{}
	}

	stats := make(map[string]uint64)

	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		stats[key] = value
	}

	memTotal := stats["MemTotal"]
	memAvail := stats["MemAvailable"]

	swapTotal := stats["SwapTotal"]
	swapFree := stats["SwapFree"]

	return MemoryStats{
		MemTotalKB:     memTotal,
		MemAvailableKB: memAvail,
		MemUsedKB:      memTotal - memAvail,

		SwapTotalKB: swapTotal,
		SwapFreeKB:  swapFree,
		SwapUsedKB:  swapTotal - swapFree,
	}
}
