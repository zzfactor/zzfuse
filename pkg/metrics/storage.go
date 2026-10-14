package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// StorageTotalBytes measures total disk space in bytes
	StorageTotalBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "storage_total_bytes",
			Help: "Total disk space in bytes",
		},
		[]string{"path", "device", "fstype"},
	)

	// StorageFreeBytes measures free disk space in bytes
	StorageFreeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "storage_free_bytes",
			Help: "Free disk space in bytes",
		},
		[]string{"path", "device", "fstype"},
	)

	// StorageUsedBytes measures used disk space in bytes
	StorageUsedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "storage_used_bytes",
			Help: "Used disk space in bytes",
		},
		[]string{"path", "device", "fstype"},
	)

	// StorageUsedPercent measures disk usage percentage
	StorageUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "storage_used_percent",
			Help: "Disk usage percentage (0-100)",
		},
		[]string{"path", "device", "fstype"},
	)

	// StorageInodesTotal measures total number of inodes
	StorageInodesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "storage_inodes_total",
			Help: "Total number of inodes",
		},
		[]string{"path", "device", "fstype"},
	)

	// StorageInodesFree measures free number of inodes
	StorageInodesFree = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "storage_inodes_free",
			Help: "Free number of inodes",
		},
		[]string{"path", "device", "fstype"},
	)

	// StorageInodesUsed measures used number of inodes
	StorageInodesUsed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "storage_inodes_used",
			Help: "Used number of inodes",
		},
		[]string{"path", "device", "fstype"},
	)

	// StorageInodesUsedPercent measures inode usage percentage
	StorageInodesUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "storage_inodes_used_percent",
			Help: "Inode usage percentage (0-100)",
		},
		[]string{"path", "device", "fstype"},
	)

	// StorageReadBytesTotal measures total bytes read from disk
	StorageReadBytesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_read_bytes_total",
			Help: "Total bytes read from disk",
		},
		[]string{"device"},
	)

	// StorageWriteBytesTotal measures total bytes written to disk
	StorageWriteBytesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_write_bytes_total",
			Help: "Total bytes written to disk",
		},
		[]string{"device"},
	)

	// StorageReadTimeSeconds measures total time spent reading from disk
	StorageReadTimeSeconds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_read_time_seconds",
			Help: "Total time spent reading from disk in seconds",
		},
		[]string{"device"},
	)

	// StorageWriteTimeSeconds measures total time spent writing to disk
	StorageWriteTimeSeconds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_write_time_seconds",
			Help: "Total time spent writing to disk in seconds",
		},
		[]string{"device"},
	)

	// StorageReadsTotal measures total number of read operations
	StorageReadsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_reads_total",
			Help: "Total number of read operations",
		},
		[]string{"device"},
	)

	// StorageWritesTotal measures total number of write operations
	StorageWritesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_writes_total",
			Help: "Total number of write operations",
		},
		[]string{"device"},
	)

	// StorageIoTimeSeconds measures total time spent doing I/O
	StorageIoTimeSeconds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_io_time_seconds",
			Help: "Total time spent doing I/O in seconds",
		},
		[]string{"device"},
	)

	// StorageWeightedIoTimeSeconds measures weighted I/O time
	StorageWeightedIoTimeSeconds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_weighted_io_time_seconds",
			Help: "Weighted I/O time in seconds",
		},
		[]string{"device"},
	)
)

func registerStorageMetrics() {
	prometheus.MustRegister(
		StorageTotalBytes,
		StorageFreeBytes,
		StorageUsedBytes,
		StorageUsedPercent,
		StorageInodesTotal,
		StorageInodesFree,
		StorageInodesUsed,
		StorageInodesUsedPercent,
		StorageReadBytesTotal,
		StorageWriteBytesTotal,
		StorageReadTimeSeconds,
		StorageWriteTimeSeconds,
		StorageReadsTotal,
		StorageWritesTotal,
		StorageIoTimeSeconds,
		StorageWeightedIoTimeSeconds,
	)
}
