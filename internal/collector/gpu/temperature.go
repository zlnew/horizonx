package gpu

import (
	"os"
	"strconv"
	"strings"
)

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
