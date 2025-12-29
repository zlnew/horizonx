package system

import (
	"os"
	"strconv"
	"strings"
)

type NetStats struct {
	RxBytes uint64
	TxBytes uint64
}

type NetSpeed struct {
	RxBps float64
	TxBps float64
}

func (r *SystemReader) DefaultInterface() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		r.log.Debug("failed to read net route", "error", err.Error())
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		if fields[1] == "00000000" {
			return fields[0]
		}
	}

	return ""
}

func (r *SystemReader) NetBytes(iface string) NetStats {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		r.log.Debug("failed to read net dev", "error", err.Error())
		return NetStats{}
	}

	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, iface+":") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 10 {
			return NetStats{}
		}

		rxBytes, _ := strconv.ParseUint(fields[1], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[9], 10, 64)

		return NetStats{
			RxBytes: rxBytes,
			TxBytes: txBytes,
		}
	}

	return NetStats{}
}
