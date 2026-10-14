package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// MemoryAllocBytes measures currently allocated memory in bytes
	MemoryAllocBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_alloc_bytes",
			Help: "Currently allocated memory in bytes",
		},
	)

	// MemoryTotalAllocBytes measures total memory allocated since start
	MemoryTotalAllocBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_total_alloc_bytes",
			Help: "Total memory allocated since start in bytes",
		},
	)

	// MemorySysBytes measures total system memory in bytes
	MemorySysBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_sys_bytes",
			Help: "Total system memory in bytes",
		},
	)

	// MemoryHeapAllocBytes measures heap allocated memory in bytes
	MemoryHeapAllocBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_heap_alloc_bytes",
			Help: "Heap allocated memory in bytes",
		},
	)

	// MemoryHeapSysBytes measures heap system memory in bytes
	MemoryHeapSysBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_heap_sys_bytes",
			Help: "Heap system memory in bytes",
		},
	)

	// MemoryHeapIdleBytes measures heap idle memory in bytes
	MemoryHeapIdleBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_heap_idle_bytes",
			Help: "Heap idle memory in bytes",
		},
	)

	// MemoryHeapInuseBytes measures heap in-use memory in bytes
	MemoryHeapInuseBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_heap_inuse_bytes",
			Help: "Heap in-use memory in bytes",
		},
	)

	// MemoryHeapReleasedBytes measures heap released memory in bytes
	MemoryHeapReleasedBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_heap_released_bytes",
			Help: "Heap released memory in bytes",
		},
	)

	// MemoryHeapObjects measures number of heap objects
	MemoryHeapObjects = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_heap_objects",
			Help: "Number of heap objects",
		},
	)

	// MemoryStackInuseBytes measures stack in-use memory in bytes
	MemoryStackInuseBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_stack_inuse_bytes",
			Help: "Stack in-use memory in bytes",
		},
	)

	// MemoryStackSysBytes measures stack system memory in bytes
	MemoryStackSysBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_stack_sys_bytes",
			Help: "Stack system memory in bytes",
		},
	)

	// MemoryMSpanInuseBytes measures MSpan in-use memory in bytes
	MemoryMSpanInuseBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_mspan_inuse_bytes",
			Help: "MSpan in-use memory in bytes",
		},
	)

	// MemoryMSpanSysBytes measures MSpan system memory in bytes
	MemoryMSpanSysBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_mspan_sys_bytes",
			Help: "MSpan system memory in bytes",
		},
	)

	// MemoryMCacheInuseBytes measures MCache in-use memory in bytes
	MemoryMCacheInuseBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_mcache_inuse_bytes",
			Help: "MCache in-use memory in bytes",
		},
	)

	// MemoryMCacheSysBytes measures MCache system memory in bytes
	MemoryMCacheSysBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_mcache_sys_bytes",
			Help: "MCache system memory in bytes",
		},
	)

	// MemoryBuckHashSysBytes measures bucket hash table memory in bytes
	MemoryBuckHashSysBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_buck_hash_sys_bytes",
			Help: "Bucket hash table memory in bytes",
		},
	)

	// MemoryGCSysBytes measures GC metadata memory in bytes
	MemoryGCSysBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_gc_sys_bytes",
			Help: "GC metadata memory in bytes",
		},
	)

	// MemoryOtherSysBytes measures other system memory in bytes
	MemoryOtherSysBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_other_sys_bytes",
			Help: "Other system memory in bytes",
		},
	)

	// GCNextTargetBytes measures next GC target in bytes
	GCNextTargetBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gc_next_target_bytes",
			Help: "Next GC target in bytes",
		},
	)

	// GCLastTimeSeconds measures last GC time in seconds since epoch
	GCLastTimeSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gc_last_time_seconds",
			Help: "Last GC time in seconds since epoch",
		},
	)

	// GCPauseTotalSeconds measures total GC pause time in seconds
	GCPauseTotalSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gc_pause_total_seconds",
			Help: "Total GC pause time in seconds",
		},
	)

	// GCCyclesTotal measures total number of GC cycles
	GCCyclesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gc_cycles_total",
			Help: "Total number of GC cycles",
		},
	)

	// GCForcedCyclesTotal measures total number of forced GC cycles
	GCForcedCyclesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gc_forced_cycles_total",
			Help: "Total number of forced GC cycles",
		},
	)

	// Goroutines measures current number of goroutines
	Goroutines = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "goroutines",
			Help: "Current number of goroutines",
		},
	)

	// Threads measures current number of OS threads
	Threads = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "threads",
			Help: "Current number of OS threads",
		},
	)

	// CPUs measures number of logical CPUs
	CPUs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "cpus",
			Help: "Number of logical CPUs",
		},
	)
)

func registerMemoryMetrics() {
	prometheus.MustRegister(
		MemoryAllocBytes,
		MemoryTotalAllocBytes,
		MemorySysBytes,
		MemoryHeapAllocBytes,
		MemoryHeapSysBytes,
		MemoryHeapIdleBytes,
		MemoryHeapInuseBytes,
		MemoryHeapReleasedBytes,
		MemoryHeapObjects,
		MemoryStackInuseBytes,
		MemoryStackSysBytes,
		MemoryMSpanInuseBytes,
		MemoryMSpanSysBytes,
		MemoryMCacheInuseBytes,
		MemoryMCacheSysBytes,
		MemoryBuckHashSysBytes,
		MemoryGCSysBytes,
		MemoryOtherSysBytes,
		GCNextTargetBytes,
		GCLastTimeSeconds,
		GCPauseTotalSeconds,
		GCCyclesTotal,
		GCForcedCyclesTotal,
		Goroutines,
		Threads,
		CPUs,
	)
}
