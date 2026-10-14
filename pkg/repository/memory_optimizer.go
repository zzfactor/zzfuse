package repository

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

// MemoryOptimizer manages memory allocation optimization and object pooling
type MemoryOptimizer struct {
	// Object pools for frequently allocated objects
	signalPool    *sync.Pool
	embeddingPool *sync.Pool
	batchPool     *sync.Pool
	templatePool  *sync.Pool

	// Memory monitoring
	memoryThreshold float64
	gcThreshold     float64
	lastGC          time.Time
	gcInterval      time.Duration

	// Statistics
	allocationsSaved int64
	memoryReduced    int64
	lastOptimization time.Time
	mu               sync.RWMutex

	// Observability
	logger util.Logger
}

// MemoryOptimizerConfig holds configuration for memory optimization
type MemoryOptimizerConfig struct {
	MemoryThreshold float64       `json:"memory_threshold"`
	GCThreshold     float64       `json:"gc_threshold"`
	GCInterval      time.Duration `json:"gc_interval"`
	EnablePools     bool          `json:"enable_pools"`
}

// DefaultMemoryOptimizerConfig returns sensible defaults for memory optimization
func DefaultMemoryOptimizerConfig() *MemoryOptimizerConfig {
	return &MemoryOptimizerConfig{
		MemoryThreshold: 0.85,            // 85% memory usage
		GCThreshold:     0.90,            // 90% memory usage
		GCInterval:      5 * time.Minute, // GC every 5 minutes
		EnablePools:     true,            // Enable object pools
	}
}

// NewMemoryOptimizer creates a new memory optimizer
func NewMemoryOptimizer(config *MemoryOptimizerConfig, logger util.Logger) *MemoryOptimizer {
	if config == nil {
		config = DefaultMemoryOptimizerConfig()
	}

	mo := &MemoryOptimizer{
		memoryThreshold: config.MemoryThreshold,
		gcThreshold:     config.GCThreshold,
		gcInterval:      config.GCInterval,
		logger:          logger,
	}

	if config.EnablePools {
		mo.initializePools()
	}

	return mo
}

// initializePools initializes object pools for frequently allocated objects
func (mo *MemoryOptimizer) initializePools() {
	// Signal data pool
	mo.signalPool = &sync.Pool{
		New: func() interface{} {
			return make(map[string]interface{}, 16) // Pre-allocate with reasonable capacity
		},
	}

	// Embedding data pool
	mo.embeddingPool = &sync.Pool{
		New: func() interface{} {
			return make([]float32, 0, 768) // Pre-allocate for common embedding sizes
		},
	}

	// Batch data pool
	mo.batchPool = &sync.Pool{
		New: func() interface{} {
			return make([]map[string]interface{}, 0, 100) // Pre-allocate for batch size
		},
	}

	// Template data pool
	mo.templatePool = &sync.Pool{
		New: func() interface{} {
			return make(map[string]string, 8) // Pre-allocate for template fields
		},
	}

	mo.logger.Info("Initialized memory optimization pools")
}

// GetSignalMap returns a signal map from the pool or creates a new one
func (mo *MemoryOptimizer) GetSignalMap() map[string]interface{} {
	if mo.signalPool == nil {
		return make(map[string]interface{})
	}

	signal := mo.signalPool.Get().(map[string]interface{})
	// Clear the map for reuse
	for k := range signal {
		delete(signal, k)
	}
	return signal
}

// PutSignalMap returns a signal map to the pool
func (mo *MemoryOptimizer) PutSignalMap(signal map[string]interface{}) {
	if mo.signalPool != nil && len(signal) < 100 { // Only pool reasonably sized maps
		mo.signalPool.Put(signal)
	}
}

// GetEmbeddingSlice returns an embedding slice from the pool or creates a new one
func (mo *MemoryOptimizer) GetEmbeddingSlice() []float32 {
	if mo.embeddingPool == nil {
		return make([]float32, 0, 768)
	}

	embedding := mo.embeddingPool.Get().([]float32)
	return embedding[:0] // Reset slice length to 0, keep capacity
}

// PutEmbeddingSlice returns an embedding slice to the pool
func (mo *MemoryOptimizer) PutEmbeddingSlice(embedding []float32) {
	if mo.embeddingPool != nil && cap(embedding) <= 1024 { // Only pool reasonably sized slices
		mo.embeddingPool.Put(embedding)
	}
}

// GetBatchSlice returns a batch slice from the pool or creates a new one
func (mo *MemoryOptimizer) GetBatchSlice() []map[string]interface{} {
	if mo.batchPool == nil {
		return make([]map[string]interface{}, 0, 100)
	}

	batch := mo.batchPool.Get().([]map[string]interface{})
	return batch[:0] // Reset slice length to 0, keep capacity
}

// PutBatchSlice returns a batch slice to the pool
func (mo *MemoryOptimizer) PutBatchSlice(batch []map[string]interface{}) {
	if mo.batchPool != nil && cap(batch) <= 500 { // Only pool reasonably sized batches
		mo.batchPool.Put(batch)
	}
}

// GetTemplateMap returns a template map from the pool or creates a new one
func (mo *MemoryOptimizer) GetTemplateMap() map[string]string {
	if mo.templatePool == nil {
		return make(map[string]string)
	}

	template := mo.templatePool.Get().(map[string]string)
	// Clear the map for reuse
	for k := range template {
		delete(template, k)
	}
	return template
}

// PutTemplateMap returns a template map to the pool
func (mo *MemoryOptimizer) PutTemplateMap(template map[string]string) {
	if mo.templatePool != nil && len(template) < 50 { // Only pool reasonably sized maps
		mo.templatePool.Put(template)
	}
}

// CheckMemoryUsage checks current memory usage and triggers optimization if needed
func (mo *MemoryOptimizer) CheckMemoryUsage() (float64, bool) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Calculate memory usage as percentage of system memory
	memoryUsage := float64(m.Alloc) / float64(m.Sys)

	mo.mu.Lock()
	defer mo.mu.Unlock()

	// Check if GC is needed
	shouldGC := memoryUsage > mo.gcThreshold && time.Since(mo.lastGC) > mo.gcInterval

	if shouldGC {
		mo.logger.Info("Memory usage high, triggering garbage collection",
			"memory_usage", memoryUsage,
			"allocated", m.Alloc,
			"system", m.Sys,
			"threshold", mo.gcThreshold)

		// Trigger garbage collection
		runtime.GC()
		mo.lastGC = time.Now()

		// Read stats again after GC
		runtime.ReadMemStats(&m)
		memoryUsage = float64(m.Alloc) / float64(m.Sys)

		mo.logger.Info("Garbage collection completed",
			"memory_usage_after", memoryUsage,
			"allocated_after", m.Alloc)
	}

	return memoryUsage, shouldGC
}

// OptimizeMemory performs memory optimization
func (mo *MemoryOptimizer) OptimizeMemory() error {
	mo.mu.Lock()
	defer mo.mu.Unlock()

	memoryUsage, _ := mo.CheckMemoryUsage()

	if memoryUsage > mo.memoryThreshold {
		mo.logger.Error("Memory usage above threshold, performing optimization",
			"memory_usage", memoryUsage,
			"threshold", mo.memoryThreshold)

		// Force garbage collection
		runtime.GC()

		// Update statistics
		mo.lastOptimization = time.Now()
		mo.memoryReduced++

		mo.logger.Info("Memory optimization completed")
	}

	return nil
}

// GetMemoryStats returns memory optimization statistics
func (mo *MemoryOptimizer) GetMemoryStats() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	mo.mu.RLock()
	defer mo.mu.RUnlock()

	return map[string]interface{}{
		"current_usage":       float64(m.Alloc) / float64(m.Sys),
		"allocated_bytes":     m.Alloc,
		"system_bytes":        m.Sys,
		"heap_alloc_bytes":    m.HeapAlloc,
		"heap_sys_bytes":      m.HeapSys,
		"heap_idle_bytes":     m.HeapIdle,
		"heap_inuse_bytes":    m.HeapInuse,
		"heap_released_bytes": m.HeapReleased,
		"heap_objects":        m.HeapObjects,
		"allocations_saved":   mo.allocationsSaved,
		"memory_reduced":      mo.memoryReduced,
		"last_optimization":   mo.lastOptimization,
		"memory_threshold":    mo.memoryThreshold,
		"gc_threshold":        mo.gcThreshold,
		"gc_interval":         mo.gcInterval,
	}
}

// StartMemoryMonitoring starts the background memory monitoring loop
func (mo *MemoryOptimizer) StartMemoryMonitoring(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	mo.logger.Info("Started memory monitoring loop")

	for {
		select {
		case <-ctx.Done():
			mo.logger.Info("Stopping memory monitoring loop")
			return
		case <-ticker.C:
			memoryUsage, _ := mo.CheckMemoryUsage()

			if memoryUsage > mo.memoryThreshold {
				mo.logger.Error("Memory usage above threshold",
					"memory_usage", memoryUsage,
					"threshold", mo.memoryThreshold)
			}

			mo.logger.Debug("Memory monitoring check",
				"memory_usage", memoryUsage)
		}
	}
}

// GetOptimizationRecommendations returns memory optimization recommendations
func (mo *MemoryOptimizer) GetOptimizationRecommendations() map[string]interface{} {
	stats := mo.GetMemoryStats()
	memoryUsage := stats["current_usage"].(float64)

	recommendations := map[string]interface{}{
		"current_usage":   memoryUsage,
		"recommendations": []string{},
	}

	if memoryUsage > 0.9 {
		recommendations["recommendations"] = append(
			recommendations["recommendations"].([]string),
			"Critical memory usage - consider reducing worker count or increasing memory")
	} else if memoryUsage > 0.8 {
		recommendations["recommendations"] = append(
			recommendations["recommendations"].([]string),
			"High memory usage - consider optimizing batch sizes or enabling object pools")
	} else if memoryUsage > 0.7 {
		recommendations["recommendations"] = append(
			recommendations["recommendations"].([]string),
			"Moderate memory usage - monitor for trends")
	}

	return recommendations
}
