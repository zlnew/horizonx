package disk

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func findMountpointsByDeviceName(devName string) []string {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	defer f.Close()

	var result []string
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		mountpoint := fields[4]
		source := fields[9]
		if !strings.HasPrefix(source, "/dev/") {
			continue
		}

		realSource := strings.SplitN(source, "[", 2)[0]
		base := filepath.Base(realSource)
		if base == devName {
			result = append(result, mountpoint)
		}
	}

	return result
}
