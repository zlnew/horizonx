package gpu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readUsage() float64 {
	matches, _ := filepath.Glob("/sys/class/drm/card*/device/gpu_busy_percent")

	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err == nil {
			return v
		}
	}

	return 0
}
