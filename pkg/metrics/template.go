package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// TemplateProcessingTotal counts total template processing operations by context and status.
	TemplateProcessingTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "template_processing_total",
			Help: "Total number of template processing operations by context and status.",
		},
		[]string{"context", "status", "signal_type", "template_id"},
	)

	// TemplateProcessingDuration measures template processing duration by context and signal type.
	TemplateProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "template_processing_duration_seconds",
			Help:    "Template processing duration in seconds by context and signal type.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"context", "signal_type", "template_id"},
	)

	// TemplateProcessingErrorsTotal counts total template processing errors by error type.
	TemplateProcessingErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "template_processing_errors_total",
			Help: "Total number of template processing errors by error type and context.",
		},
		[]string{"error_type", "context", "signal_type"},
	)

	// TemplateProcessingBatchSize measures batch sizes for template processing.
	TemplateProcessingBatchSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "template_processing_batch_size",
			Help:    "Batch sizes for template processing by context.",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"context", "signal_type"},
	)

	// TemplateProcessingThroughput measures templates processed per second.
	TemplateProcessingThroughput = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "template_processing_throughput_templates_per_second",
			Help: "Templates processed per second by context and signal type.",
		},
		[]string{"context", "signal_type"},
	)

	// TemplateProcessingMemoryUsage measures memory usage during template processing.
	TemplateProcessingMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "template_processing_memory_usage_bytes",
			Help: "Memory usage in bytes during template processing by context.",
		},
		[]string{"context", "signal_type", "memory_type"},
	)

	// TemplateProcessingActiveOperations measures currently active template processing operations.
	TemplateProcessingActiveOperations = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "template_processing_active_operations",
			Help: "Currently active template processing operations by context.",
		},
		[]string{"context", "signal_type"},
	)

	// TemplateProcessingCacheHits measures template processing cache hit rate.
	TemplateProcessingCacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "template_processing_cache_hits_total",
			Help: "Total number of template processing cache hits by context.",
		},
		[]string{"context", "signal_type"},
	)

	// TemplateProcessingCacheMisses measures template processing cache miss rate.
	TemplateProcessingCacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "template_processing_cache_misses_total",
			Help: "Total number of template processing cache misses by context.",
		},
		[]string{"context", "signal_type"},
	)

	// TemplateProcessingMetadataExtracted measures metadata extraction success rate.
	TemplateProcessingMetadataExtracted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "template_processing_metadata_extracted_total",
			Help: "Total number of metadata fields extracted by template processing.",
		},
		[]string{"context", "signal_type", "metadata_field"},
	)

	// TemplateProcessingContentGenerated measures content generation success rate.
	TemplateProcessingContentGenerated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "template_processing_content_generated_total",
			Help: "Total number of content generations by template processing.",
		},
		[]string{"context", "signal_type", "content_type"},
	)

	// TemplateProcessingFeatureFlagEnabled tracks if template processing is enabled.
	TemplateProcessingFeatureFlagEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "template_processing_feature_flag_enabled",
			Help: "Whether template processing feature flag is enabled (1=enabled, 0=disabled).",
		},
		[]string{"context"},
	)
)

// TemplateProcessingContext represents the context where template processing occurs
const (
	TemplateContextPreInsert = "pre_insert"
	TemplateContextEmbedding = "embedding"
)

// TemplateProcessingStatus represents the status of template processing operations
const (
	TemplateStatusSuccess = "success"
	TemplateStatusError   = "error"
	TemplateStatusSkipped = "skipped"
)

// TemplateProcessingErrorType represents different types of template processing errors
const (
	TemplateErrorTypeConfigFetch     = "config_fetch"
	TemplateErrorTypeTemplateRender  = "template_render"
	TemplateErrorTypeMetadataExtract = "metadata_extract"
	TemplateErrorTypeContentGenerate = "content_generate"
	TemplateErrorTypeValidation      = "validation"
)

// TemplateProcessingMemoryType represents different types of memory usage
const (
	TemplateMemoryTypeAllocated = "allocated"
	TemplateMemoryTypePeak      = "peak"
	TemplateMemoryTypeGarbage   = "garbage"
)

// TemplateProcessingContentType represents different types of generated content
const (
	TemplateContentTypeMetadata = "metadata"
	TemplateContentTypeContent  = "content"
	TemplateContentTypeBoth     = "both"
)

func registerTemplateMetrics() {
	prometheus.MustRegister(
		TemplateProcessingTotal,
		TemplateProcessingDuration,
		TemplateProcessingErrorsTotal,
		TemplateProcessingBatchSize,
		TemplateProcessingThroughput,
		TemplateProcessingMemoryUsage,
		TemplateProcessingActiveOperations,
		TemplateProcessingCacheHits,
		TemplateProcessingCacheMisses,
		TemplateProcessingMetadataExtracted,
		TemplateProcessingContentGenerated,
		TemplateProcessingFeatureFlagEnabled,
	)
}
