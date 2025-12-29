package system

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

func (r *SystemReader) Hostname() string {
	value, err := os.Hostname()
	if err != nil {
		r.log.Debug("failed to read hostname", "error", err.Error())
		return ""
	}

	return value
}

func (r *SystemReader) OsName() string {
	return runtime.GOOS
}

func (r *SystemReader) Arch() string {
	return runtime.GOARCH
}

func (r *SystemReader) KernelVersion() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	value, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		r.log.Debug("failed to read kernel version", "error", err.Error())
		return ""
	}

	return strings.TrimSpace(string(value))
}

func (r *SystemReader) Uptime() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		r.log.Debug("failed to read uptime", "error", err.Error())
		return 0
	}

	parts := strings.Fields(string(data))
	if len(parts) == 0 {
		return 0
	}

	uptime, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		r.log.Debug("failed to parse uptime", "error", err.Error())
		return 0
	}

	return uptime
}
