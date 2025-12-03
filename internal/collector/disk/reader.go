package disk

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func readRawSizeGiB(disk string) float64 {
	sizeFile := filepath.Join("/sys/block", disk, "size")
	b, err := os.ReadFile(sizeFile)
	if err != nil {
		return 0
	}

	sectors, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0
	}

	bytes := float64(sectors) * 512
	const gib = 1024 * 1024 * 1024
	return bytes / gib
}

func readFSUsage(mountpoint string, devName string) FilesystemUsage {
	var fs syscall.Statfs_t

	if err := syscall.Statfs(mountpoint, &fs); err != nil {
		return FilesystemUsage{
			Device:     devName,
			Mountpoint: mountpoint,
		}
	}

	total := float64(fs.Blocks) * float64(fs.Bsize)
	free := float64(fs.Bfree) * float64(fs.Bsize)
	used := total - free

	const gib = 1024 * 1024 * 1024

	return FilesystemUsage{
		Device:     devName,
		Mountpoint: mountpoint,
		TotalGB:    total / gib,
		UsedGB:     used / gib,
		FreeGB:     free / gib,
		Percent:    (used / total) * 100,
	}
}
