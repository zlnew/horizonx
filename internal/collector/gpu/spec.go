package gpu

import (
	"os/exec"
	"strings"
)

func readSpec() (GPUSpec, error) {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return GPUSpec{}, err
	}

	lines := strings.SplitSeq(string(out), "\n")
	for line := range lines {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "vga") || strings.Contains(lineLower, "3d") || strings.Contains(lineLower, "display") {
			parts := strings.SplitN(line, ":", 3)
			if len(parts) < 3 {
				continue
			}

			info := strings.TrimSpace(parts[2])
			vendor := detectVendor(info)

			return GPUSpec{
				Vendor: vendor,
				Model:  info,
			}, nil
		}
	}

	return GPUSpec{}, nil
}

func detectVendor(info string) string {
	infoLower := strings.ToLower(info)
	switch {
	case strings.Contains(infoLower, "amd") || strings.Contains(infoLower, "ati"):
		return "AMD"
	case strings.Contains(infoLower, "nvidia"):
		return "NVIDIA"
	case strings.Contains(infoLower, "intel"):
		return "Intel"
	default:
		return "Unknown"
	}
}
