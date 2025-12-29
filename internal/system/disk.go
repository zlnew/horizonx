package system

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

type DiskFilesystem struct {
	Device     string
	Mountpoint string
	TotalBytes uint64
	UsedBytes  uint64
	FreeBytes  uint64
}

type DiskInfo struct {
	Name        string
	RawBytes    uint64
	Temperature float64
	Filesystems []DiskFilesystem
}

type DiskUsage struct {
	TotalBytes     uint64
	UsedBytes      uint64
	AvailableBytes uint64
}

type DiskIOStats struct {
	ReadIOs      uint64
	ReadBytes    uint64
	WriteIOs     uint64
	WriteBytes   uint64
	IOTimeMillis uint64
}

func (r *SystemReader) Disks() []DiskInfo {
	disks := map[string]*DiskInfo{}

	mounts, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		r.log.Debug("failed to read mounts", "error", err.Error())
		return nil
	}

	for line := range strings.SplitSeq(string(mounts), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		device := fields[0]
		mount := fields[1]

		if !strings.HasPrefix(device, "/dev/") {
			continue
		}

		dev := strings.TrimPrefix(device, "/dev/")

		disk := dev
		if strings.HasPrefix(dev, "nvme") {
			if i := strings.LastIndex(dev, "p"); i != -1 {
				disk = dev[:i]
			}
		} else {
			disk = strings.TrimRightFunc(dev, func(r rune) bool {
				return r >= '0' && r <= '9'
			})
		}

		usage := r.DiskUsage(mount)
		if usage.TotalBytes == 0 {
			continue
		}

		if _, ok := disks[disk]; !ok {
			rawBytes := uint64(0)
			if data, err := os.ReadFile("/sys/class/block/" + disk + "/size"); err == nil {
				sectors, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
				rawBytes = sectors * 512
			}

			disks[disk] = &DiskInfo{
				Name:        disk,
				RawBytes:    rawBytes,
				Temperature: r.DiskTempC(disk),
				Filesystems: []DiskFilesystem{},
			}
		}

		disks[disk].Filesystems = append(disks[disk].Filesystems, DiskFilesystem{
			Device:     device,
			Mountpoint: mount,
			TotalBytes: usage.TotalBytes,
			UsedBytes:  usage.UsedBytes,
			FreeBytes:  usage.AvailableBytes,
		})
	}

	result := make([]DiskInfo, 0, len(disks))
	for _, d := range disks {
		result = append(result, *d)
	}

	return result
}

func (r *SystemReader) DiskUsage(path string) DiskUsage {
	var stat syscall.Statfs_t

	if err := syscall.Statfs(path, &stat); err != nil {
		r.log.Debug("failed to statfs", "path", path, "error", err.Error())
		return DiskUsage{}
	}

	total := stat.Blocks * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	used := total - available

	return DiskUsage{
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: available,
	}
}

func (r *SystemReader) DiskIO(device string) DiskIOStats {
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		r.log.Debug("failed to read diskstats", "error", err.Error())
		return DiskIOStats{}
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}

		name := fields[2]
		if name != device {
			continue
		}

		readIOs, _ := strconv.ParseUint(fields[3], 10, 64)
		readSectors, _ := strconv.ParseUint(fields[5], 10, 64)
		writeSectors, _ := strconv.ParseUint(fields[9], 10, 64)
		writeIOs, _ := strconv.ParseUint(fields[7], 10, 64)
		ioTimeMillis, _ := strconv.ParseUint(fields[12], 10, 64)

		return DiskIOStats{
			ReadIOs:      readIOs,
			ReadBytes:    readSectors * 512,
			WriteIOs:     writeIOs,
			WriteBytes:   writeSectors * 512,
			IOTimeMillis: ioTimeMillis,
		}
	}

	return DiskIOStats{}
}

func (r *SystemReader) DiskTempC(device string) float64 {
	base := "/sys/class/block/" + device + "/device"

	entries, err := os.ReadDir(base)
	if err != nil {
		r.log.Debug("failed to read block device", "error", err.Error())
		return 0
	}

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "hwmon") {
			continue
		}

		hwmonPath := base + "/" + e.Name()

		data, err := os.ReadFile(hwmonPath + "/temp1_input")
		if err == nil {
			v, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
			return v / 1000
		}

		sub, err := os.ReadDir(hwmonPath)
		if err != nil {
			continue
		}

		for _, s := range sub {
			if !strings.HasPrefix(s.Name(), "hwmon") {
				continue
			}

			data, err := os.ReadFile(hwmonPath + "/" + s.Name() + "/temp1_input")
			if err != nil {
				continue
			}

			v, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
			return v / 1000
		}
	}

	return 0
}
