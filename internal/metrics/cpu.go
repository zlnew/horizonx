package metrics

import (
	"maps"
	"strconv"
	"strings"
	"time"

	"horizonx-server/internal/system"
)

func calculateCPUUsage(state *CPUUsageState, curr map[string]system.CPUStat) (total float64, perCore []float64) {
	if state.Last == nil {
		state.Last = make(map[string]system.CPUStat, len(curr))
		maps.Copy(state.Last, curr)
		return 0, nil
	}

	maxCore := -1
	for name := range curr {
		if idx, ok := cpuCoreIndex(name); ok && idx > maxCore {
			maxCore = idx
		}
	}

	if maxCore < 0 {
		return 0, nil
	}

	perCore = make([]float64, maxCore+1)

	for name, currStat := range curr {
		idx, ok := cpuCoreIndex(name)
		if !ok {
			continue
		}

		prev, ok := state.Last[name]
		if !ok {
			continue
		}

		prevTotal := sumCPU(prev)
		currTotal := sumCPU(currStat)

		prevIdle := prev.Idle + prev.Iowait
		currIdle := currStat.Idle + currStat.Iowait

		deltaTotal := float64(currTotal - prevTotal)
		deltaIdle := float64(currIdle - prevIdle)

		if deltaTotal <= 0 {
			continue
		}

		usage := (deltaTotal - deltaIdle) / deltaTotal * 100
		perCore[idx] = usage
	}

	if currCPU, ok := curr["cpu"]; ok {
		if prevCPU, ok := state.Last["cpu"]; ok {
			pt := sumCPU(prevCPU)
			ct := sumCPU(currCPU)

			pi := prevCPU.Idle + prevCPU.Iowait
			ci := currCPU.Idle + currCPU.Iowait

			dt := float64(ct - pt)
			di := float64(ci - pi)

			if dt > 0 {
				total = (dt - di) / dt * 100
			}
		}
	}

	state.Last = make(map[string]system.CPUStat, len(curr))
	maps.Copy(state.Last, curr)

	return total, perCore
}

func calculateCPUPowerWatt(state *CPUPowerState, currEnergyUJ uint64, currTime time.Time) float64 {
	if state.LastTime.IsZero() {
		state.LastEnergyUJ = currEnergyUJ
		state.LastTime = currTime
		return 0
	}

	if currEnergyUJ < state.LastEnergyUJ {
		state.LastEnergyUJ = currEnergyUJ
		state.LastTime = currTime
		return 0
	}

	deltaEnergyJ := float64(currEnergyUJ-state.LastEnergyUJ) / 1_000_000
	deltaTime := currTime.Sub(state.LastTime).Seconds()

	if deltaTime <= 0 {
		return 0
	}

	power := deltaEnergyJ / deltaTime

	state.LastEnergyUJ = currEnergyUJ
	state.LastTime = currTime

	return power
}

func sumCPU(s system.CPUStat) uint64 {
	return s.User + s.Nice + s.System + s.Idle +
		s.Iowait + s.Irq + s.Softirq + s.Steal
}

func cpuCoreIndex(name string) (int, bool) {
	if !strings.HasPrefix(name, "cpu") || name == "cpu" {
		return -1, false
	}

	id, err := strconv.Atoi(strings.TrimPrefix(name, "cpu"))
	if err != nil {
		return 0, false
	}
	return id, true
}
