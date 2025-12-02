package gpu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"zlnew/monitor-agent/pkg"
)

func readPower() (float64, error) {
	matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/power*_input")
	targets := []string{
		"nvidia",
		"nvidia_0",
		"nvac",
		"nouveau",
		"nouveau-pci-*",
		"amdgpu",
		"radeon",
		"i915",
		"xe",
		"asus-ec-sensors",
		"msi-ec",
		"gpu_fan",
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
			return v / 1e6, nil
		}
	}

	return 0, nil
}
