package gpu

import (
	"os"
	"strings"
)

func readModel(card string) string {
	b, err := os.ReadFile("/sys/class/drm/" + card + "/device/product_name")
	if err == nil {
		return strings.TrimSpace(string(b))
	}

	b, err = os.ReadFile("/sys/class/drm/" + card + "/device/device")
	if err == nil {
		return strings.TrimSpace(string(b))
	}

	return "unknown"
}
