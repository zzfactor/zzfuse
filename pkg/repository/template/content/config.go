package content

import (
	"time"
)

// EngineConfig defines template processing configuration for the template engine
// (see template.md for full field descriptions)
type EngineConfig struct {
	ProcessorConfigID int64         `json:"processor_processor_id"`
	EnablePreInsert   bool          `json:"enable_pre_insert"`
	EnableWorkerMode  bool          `json:"enable_worker_mode"`
	StoreResults      bool          `json:"store_results"`
	PerformanceMode   string        `json:"performance_mode"`
	CacheTTL          time.Duration `json:"cache_ttl"`
	CacheSize         int           `json:"cache_size"`
}

// DefaultEngineConfig returns a default configuration for the template engine
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		EnablePreInsert:  false,
		EnableWorkerMode: false,
		StoreResults:     true,
		PerformanceMode:  "balanced",
		CacheTTL:         5 * time.Minute,
		CacheSize:        100,
	}
}
