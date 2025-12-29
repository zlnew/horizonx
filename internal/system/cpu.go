package system

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"horizonx-server/internal/pkg"
)

type CPUStat struct {
	User, Nice, System, Idle, Iowait, Irq, Softirq, Steal uint64
}

func (r *SystemReader) CPUCoreStats() map[string]CPUStat {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		r.log.Debug("failed to read /proc/stat", "error", err.Error())
		return nil
	}

	result := make(map[string]CPUStat)

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)

		if !strings.HasPrefix(line, "cpu") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		core := fields[0]

		user, _ := strconv.ParseUint(fields[1], 10, 64)
		nice, _ := strconv.ParseUint(fields[2], 10, 64)
		system, _ := strconv.ParseUint(fields[3], 10, 64)
		idle, _ := strconv.ParseUint(fields[4], 10, 64)
		iowatt, _ := strconv.ParseUint(fields[5], 10, 64)
		irq, _ := strconv.ParseUint(fields[6], 10, 64)
		softirq, _ := strconv.ParseUint(fields[7], 10, 64)
		steal, _ := strconv.ParseUint(fields[8], 10, 64)

		result[core] = CPUStat{
			User:    user,
			Nice:    nice,
			System:  system,
			Idle:    idle,
			Iowait:  iowatt,
			Irq:     irq,
			Softirq: softirq,
			Steal:   steal,
		}
	}

	return result
}

func (r *SystemReader) CPUTempC() float64 {
	entries, err := os.ReadDir("/sys/class/thermal")
	if err != nil {
		r.log.Debug("failed to read thermal directory", "error", err.Error())
	} else {
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "thermal_zone") {
				continue
			}

			base := "/sys/class/thermal/" + e.Name()

			t, err := os.ReadFile(base + "/type")
			if err != nil {
				r.log.Debug("failed to read thermal type", "zone", e.Name(), "error", err.Error())
				continue
			}

			typ := strings.ToLower(strings.TrimSpace(string(t)))
			if !strings.Contains(typ, "cpu") &&
				!strings.Contains(typ, "x86") &&
				!strings.Contains(typ, "package") {
				continue
			}

			data, err := os.ReadFile(base + "/temp")
			if err != nil {
				r.log.Debug("failed to read thermal temp", "zone", e.Name(), "error", err.Error())
				continue
			}

			val, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
			if err != nil {
				r.log.Debug("failed to parse thermal temp", "zone", e.Name(), "error", err.Error())
				continue
			}

			if val > 0 {
				temp := val / 1000
				r.log.Debug("cpu temp from thermal_zone", "zone", e.Name(), "temp_c", temp)
				return temp
			}
		}
	}

	r.log.Debug("thermal_zone cpu temp not found, fallback to hwmon")

	hwmons, err := os.ReadDir("/sys/class/hwmon")
	if err != nil {
		r.log.Debug("failed to read hwmon directory", "error", err.Error())
		return 0
	}

	for _, h := range hwmons {
		base := "/sys/class/hwmon/" + h.Name()

		nameBytes, err := os.ReadFile(base + "/name")
		if err != nil {
			continue
		}

		targetNames := []string{
			"coretemp",
			"k10temp",
			"zenpower",
			"zenpower3",
			"amd_smu",
			"ryzen_smu",
		}

		name := strings.ToLower(strings.TrimSpace(string(nameBytes)))
		if !pkg.ContainsAny(name, targetNames) {
			continue
		}

		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "temp") || !strings.HasSuffix(e.Name(), "_input") {
				continue
			}

			data, err := os.ReadFile(base + "/" + e.Name())
			if err != nil {
				continue
			}

			val, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
			if err != nil || val <= 0 {
				continue
			}

			temp := val / 1000
			return temp
		}
	}

	r.log.Debug("cpu temperature not found via thermal_zone or hwmon")
	return 0
}

func (r *SystemReader) CPUFreqMhz(core int) int {
	path := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpufreq/scaling_cur_freq", core)

	data, err := os.ReadFile(path)
	if err != nil {
		r.log.Debug("failed to read cpu core frequency", "core", core, "error", err.Error())
		return 0
	}

	freq, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return freq / 1000
}

func (r *SystemReader) CPUFreqAvgMhz() float64 {
	cores, _ := os.ReadDir("/sys/devices/system/cpu/")
	var sum, count int

	for _, c := range cores {
		if !strings.HasPrefix(c.Name(), "cpu") {
			continue
		}

		id, err := strconv.Atoi(strings.TrimPrefix(c.Name(), "cpu"))
		if err != nil {
			continue
		}

		f := r.CPUFreqMhz(id)
		if f > 0 {
			sum += f
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return float64(sum) / float64(count)
}

func (r *SystemReader) CPUEnergyUJ() uint64 {
	data, err := os.ReadFile("/sys/class/powercap/intel-rapl:0/energy_uj")
	if err != nil {
		r.log.Debug("failed to read intel rapl energy", "error", err.Error())
		return 0
	}

	val, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return val
}
