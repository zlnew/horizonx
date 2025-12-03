package disk

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readDiskTemperature(name string) float64 {
	root := filepath.Join("/sys/block", name, "device")
	return readTempFromHwmonRoot(root)
}

func readTempFromHwmonRoot(root string) float64 {
	hwmons, err := os.ReadDir(root)
	if err != nil {
		return 0
	}

	for _, hw := range hwmons {
		file := filepath.Join(root, hw.Name(), "temp1_input")
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil {
			continue
		}
		return val / 1000.0
	}

	return 0
}
