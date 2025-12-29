// Package system
package system

import (
	"os"
	"strconv"
	"strings"

	"horizonx-server/internal/logger"
)

type SystemReader struct {
	log logger.Logger
}

func NewReader(log logger.Logger) *SystemReader {
	return &SystemReader{log: log}
}

func (r *SystemReader) ReadHwmonTempC(base string) int {
	hwmon := base + "/hwmon"

	entries, err := os.ReadDir(hwmon)
	if err != nil {
		r.log.Debug("failed to read hwmon directory", "error", err.Error())
		return 0
	}

	for _, e := range entries {
		tempPath := hwmon + "/" + e.Name() + "/temp1_input"
		data, err := os.ReadFile(tempPath)
		if err != nil {
			continue
		}

		val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return val / 1000
	}

	return 0
}
