package gpu

import (
	"os"
	"strconv"
	"strings"
)

func readPowerRaw(card string) float64 {
	hwmon := "/sys/class/drm/" + card + "/device/hwmon"
	hwmons, _ := os.ReadDir(hwmon)

	for _, hw := range hwmons {
		file := hwmon + "/" + hw.Name() + "/power1_input"
		b, err := os.ReadFile(file)
		if err == nil {
			v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			return v / 1e6
		}
	}

	return 0
}

func (c *Collector) readPower(card string) float64 {
	raw := readPowerRaw(card)
	ema := c.powerEMA[card]

	if raw > 0 {
		ema.Add(raw)
	}

	return ema.Value()
}
