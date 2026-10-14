package metrics

import (
	"log"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
)

// StorageMetricsCollector collects disk usage and I/O metrics
type StorageMetricsCollector struct {
	paths []string // Filesystem paths to monitor
}

// NewStorageMetricsCollector creates a new storage metrics collector
func NewStorageMetricsCollector(paths []string) *StorageMetricsCollector {
	if len(paths) == 0 {
		// Default to root filesystem
		paths = []string{"/"}
	}
	return &StorageMetricsCollector{paths: paths}
}

// CollectStorageMetrics collects disk usage metrics for all monitored paths
func (s *StorageMetricsCollector) CollectStorageMetrics() {
	for _, path := range s.paths {
		s.collectDiskUsage(path)
	}
}

// collectDiskUsage collects disk usage metrics for a specific path
func (s *StorageMetricsCollector) collectDiskUsage(path string) {
	usage, err := disk.Usage(path)
	if err != nil {
		log.Printf("Error getting disk usage for %s: %v", path, err)
		return
	}

	// Get partition info for additional labels
	partitions, err := disk.Partitions(false)
	if err != nil {
		log.Printf("Error getting partitions: %v", err)
		return
	}

	// Find the partition for this path
	var device, fstype string
	for _, partition := range partitions {
		if partition.Mountpoint == path {
			device = partition.Device
			fstype = partition.Fstype
			break
		}
	}

	// If we couldn't find the partition, use defaults
	if device == "" {
		device = "unknown"
	}
	if fstype == "" {
		fstype = "unknown"
	}

	// Set disk usage metrics
	StorageTotalBytes.WithLabelValues(path, device, fstype).Set(float64(usage.Total))
	StorageFreeBytes.WithLabelValues(path, device, fstype).Set(float64(usage.Free))
	StorageUsedBytes.WithLabelValues(path, device, fstype).Set(float64(usage.Used))
	StorageUsedPercent.WithLabelValues(path, device, fstype).Set(usage.UsedPercent)

	// Set inode metrics
	StorageInodesTotal.WithLabelValues(path, device, fstype).Set(float64(usage.InodesTotal))
	StorageInodesFree.WithLabelValues(path, device, fstype).Set(float64(usage.InodesFree))
	StorageInodesUsed.WithLabelValues(path, device, fstype).Set(float64(usage.InodesUsed))
	StorageInodesUsedPercent.WithLabelValues(path, device, fstype).Set(usage.InodesUsedPercent)
}

// CollectIoStats collects disk I/O statistics
func (s *StorageMetricsCollector) CollectIoStats() {
	ioStats, err := disk.IOCounters()
	if err != nil {
		log.Printf("Error getting disk I/O stats: %v", err)
		return
	}

	for device, stats := range ioStats {
		// Read metrics
		StorageReadBytesTotal.WithLabelValues(device).Add(float64(stats.ReadBytes))
		StorageReadTimeSeconds.WithLabelValues(device).Add(float64(stats.ReadTime) / 1000.0) // Convert to seconds
		StorageReadsTotal.WithLabelValues(device).Add(float64(stats.ReadCount))

		// Write metrics
		StorageWriteBytesTotal.WithLabelValues(device).Add(float64(stats.WriteBytes))
		StorageWriteTimeSeconds.WithLabelValues(device).Add(float64(stats.WriteTime) / 1000.0) // Convert to seconds
		StorageWritesTotal.WithLabelValues(device).Add(float64(stats.WriteCount))

		// I/O time metrics
		StorageIoTimeSeconds.WithLabelValues(device).Add(float64(stats.IoTime) / 1000.0)             // Convert to seconds
		StorageWeightedIoTimeSeconds.WithLabelValues(device).Add(float64(stats.WeightedIO) / 1000.0) // Convert to seconds
	}
}

// CollectAllStorageMetrics collects both disk usage and I/O metrics
func (s *StorageMetricsCollector) CollectAllStorageMetrics() {
	s.CollectStorageMetrics()
	s.CollectIoStats()
}

// CollectStorageMetricsPeriodically starts periodic collection of storage metrics
func (s *StorageMetricsCollector) CollectStorageMetricsPeriodically(interval time.Duration, stopChan <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.CollectAllStorageMetrics()
		case <-stopChan:
			return
		}
	}
}

// GetDiskUsageStats returns current disk usage statistics as a map
func (s *StorageMetricsCollector) GetDiskUsageStats() map[string]interface{} {
	stats := make(map[string]interface{})

	for _, path := range s.paths {
		usage, err := disk.Usage(path)
		if err != nil {
			log.Printf("Error getting disk usage for %s: %v", path, err)
			continue
		}

		stats[path] = map[string]interface{}{
			"total":               usage.Total,
			"free":                usage.Free,
			"used":                usage.Used,
			"used_percent":        usage.UsedPercent,
			"inodes_total":        usage.InodesTotal,
			"inodes_free":         usage.InodesFree,
			"inodes_used":         usage.InodesUsed,
			"inodes_used_percent": usage.InodesUsedPercent,
		}
	}

	return stats
}

// GetIoStats returns current I/O statistics as a map
func (s *StorageMetricsCollector) GetIoStats() map[string]interface{} {
	ioStats, err := disk.IOCounters()
	if err != nil {
		log.Printf("Error getting disk I/O stats: %v", err)
		return nil
	}

	result := make(map[string]interface{})
	for device, stats := range ioStats {
		result[device] = map[string]interface{}{
			"read_bytes":  stats.ReadBytes,
			"write_bytes": stats.WriteBytes,
			"read_count":  stats.ReadCount,
			"write_count": stats.WriteCount,
			"read_time":   stats.ReadTime,
			"write_time":  stats.WriteTime,
			"io_time":     stats.IoTime,
			"weighted_io": stats.WeightedIO,
		}
	}

	return result
}

// GetStorageStats returns combined storage statistics
func (s *StorageMetricsCollector) GetStorageStats() map[string]interface{} {
	return map[string]interface{}{
		"disk_usage": s.GetDiskUsageStats(),
		"io_stats":   s.GetIoStats(),
		"timestamp":  time.Now().Unix(),
	}
}
