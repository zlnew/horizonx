package cpu

import (
	"os"
	"strconv"
	"strings"
)

func (c *Collector) readUsage() (float64, []float64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		c.log.Warn("failed to read /proc/stat", "error", err)
		return 0, nil
	}

	lines := strings.Split(string(data), "\n")
	newStats := make(map[string]cpuStat)

	var totalUsage float64
	var perCore []float64

	for _, line := range lines {
		if !strings.HasPrefix(line, "cpu") {
			continue
		}

		fields := strings.Fields(line)
		cpuName := fields[0]

		if cpuName == "cpu" || (strings.HasPrefix(cpuName, "cpu") && len(fields) > 1) {
			currentStat, err := c.parseCPUStat(fields[1:])
			if err != nil {
				c.log.Warn("failed to parse cpu stat", "error", err, "line", line)
				continue
			}
			newStats[cpuName] = currentStat

			if prevStat, ok := c.prevCPUStats[cpuName]; ok {
				usage := calculateUsage(prevStat, currentStat)
				if cpuName == "cpu" {
					totalUsage = usage
				} else {
					perCore = append(perCore, usage)
				}
			}
		}
	}

	c.prevCPUStats = newStats
	return totalUsage, perCore
}

func (c *Collector) parseCPUStat(fields []string) (cpuStat, error) {
	var stat cpuStat
	var err error
	var user, nice, system, idle, iowait, irq, softirq, steal uint64

	user, err = strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return stat, err
	}
	nice, err = strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return stat, err
	}
	system, err = strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return stat, err
	}
	idle, err = strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return stat, err
	}
	if len(fields) > 4 {
		iowait, err = strconv.ParseUint(fields[4], 10, 64)
		if err != nil {
			return stat, err
		}
	}
	if len(fields) > 5 {
		irq, err = strconv.ParseUint(fields[5], 10, 64)
		if err != nil {
			return stat, err
		}
	}
	if len(fields) > 6 {
		softirq, err = strconv.ParseUint(fields[6], 10, 64)
		if err != nil {
			return stat, err
		}
	}
	if len(fields) > 7 {
		steal, err = strconv.ParseUint(fields[7], 10, 64)
		if err != nil {
			return stat, err
		}
	}

	stat.idle = idle + iowait
	stat.total = user + nice + system + stat.idle + irq + softirq + steal
	return stat, nil
}

func calculateUsage(prev, current cpuStat) float64 {
	totalDiff := float64(current.total - prev.total)
	idleDiff := float64(current.idle - prev.idle)

	if totalDiff == 0 {
		return 0
	}

	usage := (1 - idleDiff/totalDiff) * 100
	if usage < 0 {
		usage = 0
	}
	return usage
}
