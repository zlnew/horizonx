package system

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type GPUMetrics struct {
	UtilizationGPU int
	MemTotalMB     int
	MemUsedMB      int
	MemFreeMB      int
	TemperatureC   int
	PowerDrawW     float64
	PowerLimitW    float64
	ClockMHz       int
}

func (r *SystemReader) ListDRMCards() []string {
	entries, _ := os.ReadDir("/sys/class/drm")
	var cards []string

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "card") && !strings.Contains(name, "-") {
			cards = append(cards, name)
		}
	}
	return cards
}

func (r *SystemReader) NvidiaGPU() *GPUMetrics {
	cmd := exec.Command(
		"nvidia-smi",
		"--query-gpu=utilization.gpu,memory.total,memory.used,memory.free,temperature.gpu,power.draw,power.limit,clocks.gr",
		"--format=csv,noheader,nounits",
	)

	out, err := cmd.Output()
	if err != nil {
		r.log.Debug("nvidia-smi not available", "error", err.Error())
		return nil
	}

	fields := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(fields) < 8 {
		return nil
	}

	utilizationGPU, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
	memTotalMB, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
	memUsedMB, _ := strconv.Atoi(strings.TrimSpace(fields[2]))
	memFreeMB, _ := strconv.Atoi(strings.TrimSpace(fields[3]))
	temperatureC, _ := strconv.Atoi(strings.TrimSpace(fields[4]))
	powerDrawW, _ := strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
	powerLimitW, _ := strconv.ParseFloat(strings.TrimSpace(fields[6]), 64)
	clockMhz, _ := strconv.Atoi(strings.TrimSpace(fields[7]))

	return &GPUMetrics{
		UtilizationGPU: utilizationGPU,
		MemTotalMB:     memTotalMB,
		MemUsedMB:      memUsedMB,
		MemFreeMB:      memFreeMB,
		TemperatureC:   temperatureC,
		PowerDrawW:     powerDrawW,
		PowerLimitW:    powerLimitW,
		ClockMHz:       clockMhz,
	}
}

func (r *SystemReader) AMDGPU(card string) *GPUMetrics {
	base := "/sys/class/drm/" + card + "/device"

	var utilizationGPU int
	var memTotalMB int
	var memUsedMB int
	var powerDrawW float64
	var clockMhz int

	// GPU utilization
	util, err := os.ReadFile(base + "/gpu_busy_percent")
	if err != nil {
		r.log.Debug("failed to read gpu_busy_percent", "error", err.Error())
	} else {
		utilizationGPU, _ = strconv.Atoi(strings.TrimSpace(string(util)))
	}

	// GPU Vram Total
	memTotal, err := os.ReadFile(base + "/mem_info_vram_total")
	if err != nil {
		r.log.Debug("failed to read gpu mem_info_vram_total", "error", err.Error())
	} else {
		memTotalUint, _ := strconv.ParseUint(strings.TrimSpace(string(memTotal)), 10, 64)
		memTotalMB = int(memTotalUint / 1024 / 1024)
	}

	// GPU Vram Used
	memUsed, err := os.ReadFile(base + "/mem_info_vram_used")
	if err != nil {
		r.log.Debug("failed to read gpu mem_info_vram_used", "error", err.Error())
	} else {
		memUsedUint, _ := strconv.ParseUint(strings.TrimSpace(string(memUsed)), 10, 64)
		memUsedMB = int(memUsedUint / 1024 / 1024)
	}

	// GPU Temperature
	temperatureC := r.ReadHwmonTempC(base)

	// GPU Power Draw
	hwmonBase := base + "/hwmon"
	entries, err := os.ReadDir(hwmonBase)
	if err != nil {
		r.log.Debug("failed to read hwmon dir", "error", err.Error())
	} else {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "hwmon") {
				continue
			}

			p := hwmonBase + "/" + e.Name() + "/power1_average"
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}

			v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			if err != nil {
				continue
			}

			powerDrawW = v / 1_000_000
			break
		}
	}

	// GPU Clock
	clock, err := os.ReadFile(base + "/pp_dpm_sclk")
	if err != nil {
		r.log.Debug("failed to read gpu pp_dpm_sclk", "error", err.Error())
		clockMhz = 0
	} else {
		lines := strings.SplitSeq(string(clock), "\n")
		for l := range lines {
			if strings.Contains(l, "*") {
				fields := strings.Fields(l)
				if len(fields) >= 2 {
					mhz := strings.TrimSuffix(fields[1], "Mhz")
					if v, err := strconv.Atoi(mhz); err == nil {
						clockMhz = v
					}
				}
			}
		}
	}

	if utilizationGPU == 0 && memTotalMB == 0 {
		return nil
	}

	return &GPUMetrics{
		UtilizationGPU: utilizationGPU,
		MemTotalMB:     memTotalMB,
		MemUsedMB:      memUsedMB,
		MemFreeMB:      memTotalMB - memUsedMB,
		TemperatureC:   temperatureC,
		PowerDrawW:     powerDrawW,
		ClockMHz:       clockMhz,
	}
}

func (r *SystemReader) IntelGPU(card string) *GPUMetrics {
	base := "/sys/class/drm/" + card + "/device"

	var utilizationGPU int
	var clockMhz int

	util, err := os.ReadFile(base + "/gt_busy_percent")
	if err != nil {
		r.log.Debug("failed to read gpu gt_busy_percent", "error", err.Error())
		utilizationGPU = 0
	} else {
		v, _ := strconv.Atoi(strings.TrimSpace(string(util)))
		utilizationGPU = v
	}

	clock, err := os.ReadFile(base + "/gt_cur_freq_mhz")
	if err != nil {
		r.log.Debug("failed to read gpu gt_cur_freq_mhz", "error", err.Error())
		clockMhz = 0
	} else {
		v, _ := strconv.Atoi(strings.TrimSpace(string(clock)))
		clockMhz = v
	}

	if utilizationGPU == 0 && clockMhz == 0 {
		return nil
	}

	temperatureC := r.ReadHwmonTempC(base)

	return &GPUMetrics{
		UtilizationGPU: utilizationGPU,
		TemperatureC:   temperatureC,
		ClockMHz:       clockMhz,
	}
}
