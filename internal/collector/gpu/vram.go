package gpu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readVramTotal() float64 {
	matches, _ := filepath.Glob("/sys/class/drm/card*/device/mem_info_vram_total")

	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err == nil {
			return v / 1024 / 1024
		}
	}

	return 0
}

func readVramUsed() float64 {
	matches, _ := filepath.Glob("/sys/class/drm/card*/device/mem_info_vram_used")

	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err == nil {
			return v / 1024 / 1024
		}
	}

	return 0
}
