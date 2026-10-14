package repository

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

// WorkerPoolOptimizer manages dynamic worker pool sizing based on system resources and load
type WorkerPoolOptimizer struct {
	// Configuration
	maxWorkersPerSchema int
	minWorkersPerSchema int
	cpuThreshold        float64
	memoryThreshold     float64
	loadCheckInterval   time.Duration

	// State
	currentLoad      float64
	memoryUsage      float64
	cpuUsage         float64
	lastOptimization time.Time
	mu               sync.RWMutex

	// Observability
	logger util.Logger
}

// WorkerPoolConfig holds configuration for worker pool optimization
type WorkerPoolConfig struct {
	MaxWorkersPerSchema int           `json:"max_workers_per_schema"`
	MinWorkersPerSchema int           `json:"min_workers_per_schema"`
	CPUThreshold        float64       `json:"cpu_threshold"`
	MemoryThreshold     float64       `json:"memory_threshold"`
	LoadCheckInterval   time.Duration `json:"load_check_interval"`
}

// DefaultWorkerPoolConfig returns sensible defaults for worker pool configuration
func DefaultWorkerPoolConfig() *WorkerPoolConfig {
	return &WorkerPoolConfig{
		MaxWorkersPerSchema: runtime.NumCPU() * 2, // 2x CPU cores
		MinWorkersPerSchema: 2,                    // Minimum 2 workers
		CPUThreshold:        0.8,                  // 80% CPU usage
		MemoryThreshold:     0.85,                 // 85% memory usage
		LoadCheckInterval:   30 * time.Second,     // Check every 30 seconds
	}
}

// NewWorkerPoolOptimizer creates a new worker pool optimizer
func NewWorkerPoolOptimizer(config *WorkerPoolConfig, logger util.Logger) *WorkerPoolOptimizer {
	if config == nil {
		config = DefaultWorkerPoolConfig()
	}

	return &WorkerPoolOptimizer{
		maxWorkersPerSchema: config.MaxWorkersPerSchema,
		minWorkersPerSchema: config.MinWorkersPerSchema,
		cpuThreshold:        config.CPUThreshold,
		memoryThreshold:     config.MemoryThreshold,
		loadCheckInterval:   config.LoadCheckInterval,
		logger:              logger,
	}
}

// CalculateOptimalWorkerCount calculates the optimal number of workers for a schema
func (wpo *WorkerPoolOptimizer) CalculateOptimalWorkerCount(
	signalTypes []string,
	vectorStoreCount int,
	currentLoad float64,
) int {
	wpo.mu.Lock()
	defer wpo.mu.Unlock()

	// Base calculation: one worker per signal type
	baseWorkers := len(signalTypes)
	if baseWorkers == 0 {
		baseWorkers = 1
	}

	// Adjust based on vector store count (more vector stores = more work)
	vectorStoreMultiplier := 1.0
	if vectorStoreCount > 1 {
		vectorStoreMultiplier = 1.0 + (float64(vectorStoreCount-1) * 0.2) // 20% increase per additional vector store
	}

	// Adjust based on system load
	loadMultiplier := 1.0
	if currentLoad > wpo.cpuThreshold {
		loadMultiplier = 0.8 // Reduce workers under high load
	} else if currentLoad < 0.3 {
		loadMultiplier = 1.2 // Increase workers under low load
	}

	// Calculate optimal count
	optimalCount := int(float64(baseWorkers) * vectorStoreMultiplier * loadMultiplier)

	// Apply bounds
	if optimalCount < wpo.minWorkersPerSchema {
		optimalCount = wpo.minWorkersPerSchema
	}
	if optimalCount > wpo.maxWorkersPerSchema {
		optimalCount = wpo.maxWorkersPerSchema
	}

	wpo.logger.Info("Calculated optimal worker count",
		"base_workers", baseWorkers,
		"vector_store_count", vectorStoreCount,
		"vector_store_multiplier", vectorStoreMultiplier,
		"current_load", currentLoad,
		"load_multiplier", loadMultiplier,
		"optimal_count", optimalCount)

	return optimalCount
}

// GetSystemMetrics returns current system resource usage
func (wpo *WorkerPoolOptimizer) GetSystemMetrics() (cpuUsage, memoryUsage float64) {
	wpo.mu.RLock()
	defer wpo.mu.RUnlock()

	// Get CPU usage (simplified - in production, use proper metrics)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Calculate memory usage as percentage
	memoryUsage = float64(m.Alloc) / float64(m.Sys) * 100

	// For CPU, we'll use a simplified approach
	// In production, use proper CPU metrics collection
	cpuUsage = wpo.currentLoad

	return cpuUsage, memoryUsage
}

// UpdateLoad updates the current system load
func (wpo *WorkerPoolOptimizer) UpdateLoad(load float64) {
	wpo.mu.Lock()
	defer wpo.mu.Unlock()
	wpo.currentLoad = load
}

// ShouldOptimize checks if it's time to run optimization
func (wpo *WorkerPoolOptimizer) ShouldOptimize() bool {
	wpo.mu.RLock()
	defer wpo.mu.RUnlock()

	return time.Since(wpo.lastOptimization) >= wpo.loadCheckInterval
}

// MarkOptimizationComplete marks that optimization has been completed
func (wpo *WorkerPoolOptimizer) MarkOptimizationComplete() {
	wpo.mu.Lock()
	defer wpo.mu.Unlock()
	wpo.lastOptimization = time.Now()
}

// GetOptimizationRecommendations returns recommendations for worker pool optimization
func (wpo *WorkerPoolOptimizer) GetOptimizationRecommendations(
	signalTypes []string,
	vectorStoreCount int,
) map[string]interface{} {
	cpuUsage, memoryUsage := wpo.GetSystemMetrics()

	recommendations := map[string]interface{}{
		"current_metrics": map[string]interface{}{
			"cpu_usage":    cpuUsage,
			"memory_usage": memoryUsage,
		},
		"current_config": map[string]interface{}{
			"signal_types":       signalTypes,
			"vector_store_count": vectorStoreCount,
			"max_workers":        wpo.maxWorkersPerSchema,
			"min_workers":        wpo.minWorkersPerSchema,
			"cpu_threshold":      wpo.cpuThreshold,
			"memory_threshold":   wpo.memoryThreshold,
		},
		"recommendations": []string{},
	}

	// Generate recommendations based on current state
	if cpuUsage > wpo.cpuThreshold {
		recommendations["recommendations"] = append(
			recommendations["recommendations"].([]string),
			"High CPU usage detected - consider reducing worker count or increasing CPU resources")
	}

	if memoryUsage > wpo.memoryThreshold {
		recommendations["recommendations"] = append(
			recommendations["recommendations"].([]string),
			"High memory usage detected - consider reducing worker count or increasing memory")
	}

	if len(signalTypes) > wpo.maxWorkersPerSchema {
		recommendations["recommendations"] = append(
			recommendations["recommendations"].([]string),
			"More signal types than max workers - consider increasing max_workers_per_schema")
	}

	if vectorStoreCount > 5 {
		recommendations["recommendations"] = append(
			recommendations["recommendations"].([]string),
			"Many vector stores detected - consider consolidating or optimizing routing")
	}

	return recommendations
}

// OptimizeWorkerPool optimizes the worker pool for a schema
func (wpo *WorkerPoolOptimizer) OptimizeWorkerPool(
	ctx context.Context,
	schemaName string,
	currentWorkers []*EmbeddingWorker,
	signalTypes []string,
	vectorStoreCount int,
) (int, error) {
	if !wpo.ShouldOptimize() {
		return len(currentWorkers), nil // No optimization needed
	}

	cpuUsage, memoryUsage := wpo.GetSystemMetrics()
	optimalCount := wpo.CalculateOptimalWorkerCount(signalTypes, vectorStoreCount, cpuUsage)

	wpo.logger.Info("Worker pool optimization",
		"schema", schemaName,
		"current_workers", len(currentWorkers),
		"optimal_workers", optimalCount,
		"cpu_usage", cpuUsage,
		"memory_usage", memoryUsage,
		"signal_types", signalTypes,
		"vector_store_count", vectorStoreCount)

	wpo.MarkOptimizationComplete()

	return optimalCount, nil
}

// StartOptimizationLoop starts the background optimization loop
func (wpo *WorkerPoolOptimizer) StartOptimizationLoop(ctx context.Context) {
	ticker := time.NewTicker(wpo.loadCheckInterval)
	defer ticker.Stop()

	wpo.logger.Info("Started worker pool optimization loop",
		"check_interval", wpo.loadCheckInterval)

	for {
		select {
		case <-ctx.Done():
			wpo.logger.Info("Stopping worker pool optimization loop")
			return
		case <-ticker.C:
			// Update system metrics
			cpuUsage, memoryUsage := wpo.GetSystemMetrics()
			wpo.UpdateLoad(cpuUsage)

			wpo.logger.Debug("Worker pool optimization check",
				"cpu_usage", cpuUsage,
				"memory_usage", memoryUsage)
		}
	}
}

// GetOptimizationStats returns optimization statistics
func (wpo *WorkerPoolOptimizer) GetOptimizationStats() map[string]interface{} {
	wpo.mu.RLock()
	defer wpo.mu.RUnlock()

	return map[string]interface{}{
		"current_load":           wpo.currentLoad,
		"memory_usage":           wpo.memoryUsage,
		"cpu_usage":              wpo.cpuUsage,
		"last_optimization":      wpo.lastOptimization,
		"max_workers_per_schema": wpo.maxWorkersPerSchema,
		"min_workers_per_schema": wpo.minWorkersPerSchema,
		"cpu_threshold":          wpo.cpuThreshold,
		"memory_threshold":       wpo.memoryThreshold,
		"load_check_interval":    wpo.loadCheckInterval,
	}
}
