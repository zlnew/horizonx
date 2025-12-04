package cpu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"zlnew/monitor-agent/pkg"
)

func readTemperature() float64 {
	matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/temp*_input")
	targets := []string{
		"coretemp",
		"k10temp",
		"zenpower",
		"zenpower3",
		"amd_smu",
		"ryzen_smu",
	}

	for _, f := range matches {
		dir := filepath.Dir(f)
		namePath := filepath.Join(dir, "name")

		nameBytes, err := os.ReadFile(namePath)
		if err != nil {
			continue
		}

		name := strings.TrimSpace(string(nameBytes))
		if !pkg.ContainsAny(name, targets) {
			continue
		}

		b, err := os.ReadFile(f)
		if err == nil {
			v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			return v / 1e3
		}
	}

	return 0
}
