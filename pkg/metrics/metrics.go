package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// Package metrics provides project-wide Prometheus metrics definitions and registration.

// Global collectors for API access
var (
	memoryCollector  *MemoryMetricsCollector
	storageCollector *StorageMetricsCollector
	collectorsMutex  sync.RWMutex
)

func Register() {
	registerDBNotifierMetrics()
	registerDBMetrics()
	registerHTTPMetrics()
	registerEmbeddingMetrics()
	registerTemplateMetrics()
	registerMemoryMetrics()
	registerStorageMetrics()
	registerEventBusMetrics()

	// Initialize global collectors
	collectorsMutex.Lock()
	defer collectorsMutex.Unlock()

	memoryCollector = NewMemoryMetricsCollector()
	storageCollector = NewStorageMetricsCollector([]string{"/"}) // Default to root path
}

// GetAllMetrics returns all metrics data in a unified format suitable for API responses
func GetAllMetrics() map[string]interface{} {
	collectorsMutex.RLock()
	defer collectorsMutex.RUnlock()

	return map[string]interface{}{
		"memory":    getMemoryMetrics(),
		"storage":   getStorageMetrics(),
		"database":  getDatabaseMetrics(),
		"http":      getHTTPMetrics(),
		"notifier":  getNotifierMetrics(),
		"embedding": getEmbeddingMetrics(),
		"template":  getTemplateMetrics(),
		"event_bus": getEventBusMetrics(),
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
	}
}

// GetMemoryMetrics returns memory and runtime statistics
func GetMemoryMetrics() map[string]interface{} {
	collectorsMutex.RLock()
	defer collectorsMutex.RUnlock()

	if memoryCollector == nil {
		return map[string]interface{}{"error": "memory collector not initialized"}
	}

	return memoryCollector.GetCombinedStats()
}

// GetStorageMetrics returns storage and I/O statistics
func GetStorageMetrics() map[string]interface{} {
	collectorsMutex.RLock()
	defer collectorsMutex.RUnlock()

	if storageCollector == nil {
		return map[string]interface{}{"error": "storage collector not initialized"}
	}

	return storageCollector.GetStorageStats()
}

// GetDatabaseMetrics returns database operation statistics
func GetDatabaseMetrics() map[string]interface{} {
	// Collect current values from Prometheus metrics
	metrics := make(map[string]interface{})

	// Get DB query metrics
	queryMetrics := make(map[string]interface{})

	// Note: Prometheus metrics are typically accessed via HTTP endpoint
	// For programmatic access, we'd need to implement metric value extraction
	// This is a simplified version - in practice, you might want to use
	// prometheus.Registry to get metric values programmatically

	queryMetrics["total_queries"] = "use_prometheus_endpoint"
	queryMetrics["latency_stats"] = "use_prometheus_endpoint"

	metrics["queries"] = queryMetrics
	metrics["connections"] = map[string]interface{}{
		"active": "use_prometheus_endpoint",
		"idle":   "use_prometheus_endpoint",
	}

	return metrics
}

// GetHTTPMetrics returns HTTP request statistics
func GetHTTPMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	// HTTP metrics are typically accessed via Prometheus endpoint
	// This provides a structure for the API response
	metrics["requests"] = map[string]interface{}{
		"total":     "use_prometheus_endpoint",
		"by_method": "use_prometheus_endpoint",
		"by_status": "use_prometheus_endpoint",
	}

	metrics["latency"] = map[string]interface{}{
		"average": "use_prometheus_endpoint",
		"p95":     "use_prometheus_endpoint",
		"p99":     "use_prometheus_endpoint",
	}

	return metrics
}

// GetNotifierMetrics returns notifier operation statistics
func GetNotifierMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	metrics["notifications"] = map[string]interface{}{
		"delivered": "use_prometheus_endpoint",
		"dropped":   "use_prometheus_endpoint",
		"by_topic":  "use_prometheus_endpoint",
	}

	return metrics
}

// GetEmbeddingMetrics returns embedding operation statistics
func GetEmbeddingMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	metrics["jobs"] = map[string]interface{}{
		"total":          "use_prometheus_endpoint",
		"active":         "use_prometheus_endpoint",
		"by_status":      "use_prometheus_endpoint",
		"by_signal_type": "use_prometheus_endpoint",
		"duration_stats": "use_prometheus_endpoint",
	}

	metrics["signals"] = map[string]interface{}{
		"processed":      "use_prometheus_endpoint",
		"by_status":      "use_prometheus_endpoint",
		"by_signal_type": "use_prometheus_endpoint",
	}

	metrics["tokens"] = map[string]interface{}{
		"processed":      "use_prometheus_endpoint",
		"truncated":      "use_prometheus_endpoint",
		"by_signal_type": "use_prometheus_endpoint",
	}

	metrics["vectorstore"] = map[string]interface{}{
		"writes":        "use_prometheus_endpoint",
		"errors":        "use_prometheus_endpoint",
		"latency_stats": "use_prometheus_endpoint",
		"by_provider":   "use_prometheus_endpoint",
	}

	metrics["templates"] = map[string]interface{}{
		"renders":       "use_prometheus_endpoint",
		"errors":        "use_prometheus_endpoint",
		"latency_stats": "use_prometheus_endpoint",
	}

	metrics["quality"] = map[string]interface{}{
		"scores":         "use_prometheus_endpoint",
		"by_signal_type": "use_prometheus_endpoint",
	}

	metrics["memory"] = map[string]interface{}{
		"usage":          "use_prometheus_endpoint",
		"by_signal_type": "use_prometheus_endpoint",
	}

	metrics["workers"] = map[string]interface{}{
		"active":       "use_prometheus_endpoint",
		"uptime":       "use_prometheus_endpoint",
		"by_worker_id": "use_prometheus_endpoint",
	}

	return metrics
}

// GetTemplateMetrics returns template processing metrics
func GetTemplateMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	metrics["processing"] = map[string]interface{}{
		"total":          "use_prometheus_endpoint",
		"active":         "use_prometheus_endpoint",
		"by_context":     "use_prometheus_endpoint",
		"by_signal_type": "use_prometheus_endpoint",
		"duration_stats": "use_prometheus_endpoint",
	}

	metrics["errors"] = map[string]interface{}{
		"total":          "use_prometheus_endpoint",
		"by_error_type":  "use_prometheus_endpoint",
		"by_context":     "use_prometheus_endpoint",
		"by_signal_type": "use_prometheus_endpoint",
	}

	metrics["performance"] = map[string]interface{}{
		"throughput":     "use_prometheus_endpoint",
		"batch_size":     "use_prometheus_endpoint",
		"memory_usage":   "use_prometheus_endpoint",
		"cache_hit_rate": "use_prometheus_endpoint",
	}

	metrics["content"] = map[string]interface{}{
		"metadata_extracted": "use_prometheus_endpoint",
		"content_generated":  "use_prometheus_endpoint",
		"by_signal_type":     "use_prometheus_endpoint",
	}

	metrics["feature_flags"] = map[string]interface{}{
		"enabled":  "use_prometheus_endpoint",
		"disabled": "use_prometheus_endpoint",
	}

	return metrics
}

// GetPrometheusMetrics returns metrics in Prometheus format
func GetPrometheusMetrics() (string, error) {
	// Get the default Prometheus registry
	registry := prometheus.DefaultRegisterer.(*prometheus.Registry)

	// Create a buffer to write the metrics
	var buf bytes.Buffer

	// Write metrics in Prometheus text format
	encoder := expfmt.NewEncoder(&buf, expfmt.FmtText)

	// Gather all metrics
	metricFamilies, err := registry.Gather()
	if err != nil {
		return "", fmt.Errorf("failed to gather metrics: %w", err)
	}

	// Encode each metric family
	for _, mf := range metricFamilies {
		if err := encoder.Encode(mf); err != nil {
			return "", fmt.Errorf("failed to encode metric family: %w", err)
		}
	}

	return buf.String(), nil
}

// GetMetricsJSON returns all metrics as JSON
func GetMetricsJSON() ([]byte, error) {
	metrics := GetAllMetrics()
	return json.MarshalIndent(metrics, "", "  ")
}

// GetMetricsSummary returns a summary of key metrics
func GetMetricsSummary() map[string]interface{} {
	collectorsMutex.RLock()
	defer collectorsMutex.RUnlock()

	summary := make(map[string]interface{})

	// Memory summary
	if memoryCollector != nil {
		memoryStats := memoryCollector.GetMemoryStats()
		summary["memory"] = map[string]interface{}{
			"alloc_bytes":  memoryStats["alloc"],
			"heap_objects": memoryStats["heap_objects"],
			"goroutines":   memoryCollector.GetRuntimeStats()["goroutines"],
		}
	}

	// Storage summary
	if storageCollector != nil {
		storageStats := storageCollector.GetDiskUsageStats()
		if len(storageStats) > 0 {
			// Get first path stats as summary
			for path, stats := range storageStats {
				if pathStats, ok := stats.(map[string]interface{}); ok {
					summary["storage"] = map[string]interface{}{
						"path":         path,
						"used_percent": pathStats["used_percent"],
						"free_bytes":   pathStats["free"],
					}
					break
				}
			}
		}
	}

	summary["timestamp"] = time.Now().Unix()

	return summary
}

// Helper functions for internal use
func getMemoryMetrics() map[string]interface{} {
	return GetMemoryMetrics()
}

func getStorageMetrics() map[string]interface{} {
	return GetStorageMetrics()
}

func getDatabaseMetrics() map[string]interface{} {
	return GetDatabaseMetrics()
}

func getHTTPMetrics() map[string]interface{} {
	return GetHTTPMetrics()
}

func getNotifierMetrics() map[string]interface{} {
	return GetNotifierMetrics()
}

func getEmbeddingMetrics() map[string]interface{} {
	return GetEmbeddingMetrics()
}

func getTemplateMetrics() map[string]interface{} {
	return GetTemplateMetrics()
}

func getEventBusMetrics() map[string]interface{} {
	return GetEventBusMetrics()
}
