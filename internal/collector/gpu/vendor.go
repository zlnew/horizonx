package gpu

import (
	"os"
	"strings"
)

func readVendor(card string) string {
	b, err := os.ReadFile("/sys/class/drm/" + card + "/device/vendor")
	if err != nil {
		return "unknown"
	}

	val := strings.TrimSpace(string(b))
	switch val {
	case "0x1002":
		return "AMD"
	case "0x10de":
		return "NVIDIA"
	case "0x8086":
		return "INTEL"
	}

	return val
}
