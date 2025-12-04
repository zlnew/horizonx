package gpu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
