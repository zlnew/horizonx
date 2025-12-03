// Package disk
package disk

import "context"

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) Collect(ctx context.Context) (any, error) {
	disks, parts, err := detectBlockDevices()
	if err != nil {
		return nil, err
	}

	var result []DiskMetric

	for _, disk := range disks {
		metric := DiskMetric{
			Name:        disk,
			RawSizeGB:   readRawSizeGiB(disk),
			Temperature: readDiskTemperature(disk),
		}

		for _, part := range parts {
			parent := getParentDisk(part)
			if parent != disk {
				continue
			}

			mounts := findMountpointsByDeviceName(part)
			for _, mp := range mounts {
				fs := readFSUsage(mp, part)
				metric.Filesystems = append(metric.Filesystems, fs)
			}
		}

		result = append(result, metric)
	}

	return result, nil
}
