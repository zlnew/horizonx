package gpu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (c *Collector) readFanSpeedPercent(card string) float64 {
	hwmonRoot := "/sys/class/drm/" + card + "/device/hwmon"
	hwmons, err := os.ReadDir(hwmonRoot)
	if err != nil {
		c.log.Debug("failed to read hwmon root for fan speed", "path", hwmonRoot, "error", err)
		return 0
	}

	for _, hw := range hwmons {
		base := filepath.Join(hwmonRoot, hw.Name())

		pwmFile := filepath.Join(base, "pwm1")
		maxFile := filepath.Join(base, "pwm1_max")
		fanRpmFile := filepath.Join(base, "fan1_input")

		if pwm, err := os.ReadFile(pwmFile); err == nil {
			val, err := strconv.ParseFloat(strings.TrimSpace(string(pwm)), 64)
			if err != nil {
				c.log.Warn("failed to parse pwm1", "file", pwmFile, "error", err)
				continue
			}
			if val <= 0 {
				return 0
			}

			max := 255.0
			if maxB, err := os.ReadFile(maxFile); err == nil {
				v, err := strconv.ParseFloat(strings.TrimSpace(string(maxB)), 64)
				if err == nil && v > 0 {
					max = v
				} else if err != nil {
					c.log.Debug("failed to parse pwm1_max", "file", maxFile, "error", err)
				}
			}

			return (val / max) * 100.0
		}

		if rpm, err := os.ReadFile(fanRpmFile); err == nil {
			val, err := strconv.ParseFloat(strings.TrimSpace(string(rpm)), 64)
			if err != nil {
				c.log.Warn("failed to parse fan1_input", "file", fanRpmFile, "error", err)
				continue
			}
			if val > 0 {
				return 1
			}
		}
	}

	return 0
}
