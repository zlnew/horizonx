package gpu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readVendor(card string) string {
	b, err := os.ReadFile("/sys/class/drm/" + card + "/device/vendor")
	if err != nil {
		return "unknown"
	}

	val := strings.TrimSpace(string(b))
	switch val {
	case "0x1002":
		return "AMD"
	case "0x10de":
		return "NVIDIA"
	case "0x8086":
		return "INTEL"
	}
	return val
}

func readModel(card string) string {
	b, err := os.ReadFile("/sys/class/drm/" + card + "/device/product_name")
	if err == nil {
		return strings.TrimSpace(string(b))
	}

	b, err = os.ReadFile("/sys/class/drm/" + card + "/device/device")
	if err == nil {
		return strings.TrimSpace(string(b))
	}

	return "unknown"
}

func readTemperature(card string) float64 {
	hwmonRoot := "/sys/class/drm/" + card + "/device/hwmon"

	hwmons, _ := os.ReadDir(hwmonRoot)
	for _, hw := range hwmons {
		file := hwmonRoot + "/" + hw.Name() + "/temp1_input"
		b, err := os.ReadFile(file)
		if err == nil {
			v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			return v / 1000
		}
	}
	return 0
}

func readCoreUsage(card string) float64 {
	path := "/sys/class/drm/" + card + "/device/gpu_busy_percent"
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	return v
}

func readVRAM(card string) (total float64, used float64, percent float64) {
	base := "/sys/class/drm/" + card + "/device/"

	t, err := os.ReadFile(base + "mem_info_vram_total")
	if err != nil {
		return
	}

	u, err := os.ReadFile(base + "mem_info_vram_used")
	if err != nil {
		return
	}

	totalB, _ := strconv.ParseFloat(strings.TrimSpace(string(t)), 64)
	usedB, _ := strconv.ParseFloat(strings.TrimSpace(string(u)), 64)

	total = totalB / (1024 * 1024 * 1024)
	used = usedB / (1024 * 1024 * 1024)
	percent = (usedB / totalB) * 100

	return
}

func readPower(card string) float64 {
	hwmon := "/sys/class/drm/" + card + "/device/hwmon"
	hwmons, _ := os.ReadDir(hwmon)

	for _, hw := range hwmons {
		file := hwmon + "/" + hw.Name() + "/power1_input"
		b, err := os.ReadFile(file)
		if err == nil {
			v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			return v / 1_000_000
		}
	}

	return 0
}

func readFanSpeedPercent(card string) float64 {
	hwmonRoot := "/sys/class/drm/" + card + "/device/hwmon"
	hwmons, _ := os.ReadDir(hwmonRoot)

	for _, hw := range hwmons {
		base := filepath.Join(hwmonRoot, hw.Name())

		pwmFile := filepath.Join(base, "pwm1")
		maxFile := filepath.Join(base, "pwm1_max")
		fanRpmFile := filepath.Join(base, "fan1_input")

		if pwm, err := os.ReadFile(pwmFile); err == nil {
			val, _ := strconv.ParseFloat(strings.TrimSpace(string(pwm)), 64)
			if val <= 0 {
				return 0
			}

			max := 255.0
			if maxB, err := os.ReadFile(maxFile); err == nil {
				v, _ := strconv.ParseFloat(strings.TrimSpace(string(maxB)), 64)
				if v > 0 {
					max = v
				}
			}

			return (val / max) * 100.0
		}

		if rpm, err := os.ReadFile(fanRpmFile); err == nil {
			val, _ := strconv.ParseFloat(strings.TrimSpace(string(rpm)), 64)
			if val > 0 {
				return 1
			}
		}
	}

	return 0
}
