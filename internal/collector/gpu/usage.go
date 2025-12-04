package gpu

import (
	"os"
	"strconv"
	"strings"
)

func readCoreUsage(card string) float64 {
	path := "/sys/class/drm/" + card + "/device/gpu_busy_percent"
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)

	return v
}
