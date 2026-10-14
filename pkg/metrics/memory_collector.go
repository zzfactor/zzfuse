package metrics

import (
	"runtime"
	"time"
)

// MemoryMetricsCollector provides memory and runtime metrics collection
type MemoryMetricsCollector struct {
	lastGCCycles   uint32
	lastForcedGC   uint32
	lastPauseTotal uint64
}

// NewMemoryMetricsCollector creates a new memory metrics collector
func NewMemoryMetricsCollector() *MemoryMetricsCollector {
	return &MemoryMetricsCollector{}
}

// CollectMemoryMetrics collects all memory and runtime metrics
func (s *MemoryMetricsCollector) CollectMemoryMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Memory metrics
	MemoryAllocBytes.Set(float64(m.Alloc))
	MemoryTotalAllocBytes.Set(float64(m.TotalAlloc))
	MemorySysBytes.Set(float64(m.Sys))
	MemoryHeapAllocBytes.Set(float64(m.HeapAlloc))
	MemoryHeapSysBytes.Set(float64(m.HeapSys))
	MemoryHeapIdleBytes.Set(float64(m.HeapIdle))
	MemoryHeapInuseBytes.Set(float64(m.HeapInuse))
	MemoryHeapReleasedBytes.Set(float64(m.HeapReleased))
	MemoryHeapObjects.Set(float64(m.HeapObjects))
	MemoryStackInuseBytes.Set(float64(m.StackInuse))
	MemoryStackSysBytes.Set(float64(m.StackSys))
	MemoryMSpanInuseBytes.Set(float64(m.MSpanInuse))
	MemoryMSpanSysBytes.Set(float64(m.MSpanSys))
	MemoryMCacheInuseBytes.Set(float64(m.MCacheInuse))
	MemoryMCacheSysBytes.Set(float64(m.MCacheSys))
	MemoryBuckHashSysBytes.Set(float64(m.BuckHashSys))
	MemoryGCSysBytes.Set(float64(m.GCSys))
	MemoryOtherSysBytes.Set(float64(m.OtherSys))

	// GC metrics
	GCNextTargetBytes.Set(float64(m.NextGC))
	GCLastTimeSeconds.Set(float64(m.LastGC) / 1e9)         // Convert nanoseconds to seconds
	GCPauseTotalSeconds.Set(float64(m.PauseTotalNs) / 1e9) // Convert nanoseconds to seconds

	// GC cycle counters (only increment when they change)
	if m.NumGC > s.lastGCCycles {
		GCCyclesTotal.Add(float64(m.NumGC - s.lastGCCycles))
		s.lastGCCycles = m.NumGC
	}

	if m.NumForcedGC > s.lastForcedGC {
		GCForcedCyclesTotal.Add(float64(m.NumForcedGC - s.lastForcedGC))
		s.lastForcedGC = m.NumForcedGC
	}

	// Runtime metrics
	Goroutines.Set(float64(runtime.NumGoroutine()))
	Threads.Set(float64(runtime.GOMAXPROCS(0))) // Current GOMAXPROCS
	CPUs.Set(float64(runtime.NumCPU()))
}

// CollectMemoryMetricsPeriodically starts periodic collection of memory metrics
func (s *MemoryMetricsCollector) CollectMemoryMetricsPeriodically(interval time.Duration, stopChan <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.CollectMemoryMetrics()
		case <-stopChan:
			return
		}
	}
}

// GetMemoryStats returns current memory statistics as a map
func (s *MemoryMetricsCollector) GetMemoryStats() map[string]int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]int64{
		"alloc":          int64(m.Alloc),
		"total_alloc":    int64(m.TotalAlloc),
		"sys":            int64(m.Sys),
		"heap_alloc":     int64(m.HeapAlloc),
		"heap_sys":       int64(m.HeapSys),
		"heap_idle":      int64(m.HeapIdle),
		"heap_inuse":     int64(m.HeapInuse),
		"heap_released":  int64(m.HeapReleased),
		"heap_objects":   int64(m.HeapObjects),
		"stack_inuse":    int64(m.StackInuse),
		"stack_sys":      int64(m.StackSys),
		"mspan_inuse":    int64(m.MSpanInuse),
		"mspan_sys":      int64(m.MSpanSys),
		"mcache_inuse":   int64(m.MCacheInuse),
		"mcache_sys":     int64(m.MCacheSys),
		"buck_hash_sys":  int64(m.BuckHashSys),
		"gc_sys":         int64(m.GCSys),
		"other_sys":      int64(m.OtherSys),
		"next_gc":        int64(m.NextGC),
		"last_gc":        int64(m.LastGC),
		"pause_total_ns": int64(m.PauseTotalNs),
		"num_gc":         int64(m.NumGC),
		"num_forced_gc":  int64(m.NumForcedGC),
	}
}

// GetRuntimeStats returns current runtime statistics
func (s *MemoryMetricsCollector) GetRuntimeStats() map[string]int64 {
	return map[string]int64{
		"goroutines": int64(runtime.NumGoroutine()),
		"threads":    int64(runtime.GOMAXPROCS(0)),
		"cpus":       int64(runtime.NumCPU()),
	}
}

// GetCombinedStats returns combined memory and runtime statistics
func (s *MemoryMetricsCollector) GetCombinedStats() map[string]interface{} {
	memoryStats := s.GetMemoryStats()
	runtimeStats := s.GetRuntimeStats()

	return map[string]interface{}{
		"memory":    memoryStats,
		"runtime":   runtimeStats,
		"timestamp": time.Now().Unix(),
	}
}
