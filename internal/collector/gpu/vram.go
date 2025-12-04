package gpu

import (
	"os"
	"strconv"
	"strings"
)

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
