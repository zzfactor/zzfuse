package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// EmbeddingJobsTotal counts total embedding jobs by status and signal type.
	EmbeddingJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_jobs_total",
			Help: "Total number of embedding jobs by status and signal type.",
		},
		[]string{"status", "signal_type", "processing_mode"},
	)

	// EmbeddingJobsDuration measures embedding job duration by signal type.
	EmbeddingJobsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "embedding_jobs_duration_seconds",
			Help:    "Embedding job duration in seconds by signal type.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"signal_type", "processing_mode"},
	)

	// EmbeddingSignalsTotal counts total signals processed by status and signal type.
	EmbeddingSignalsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_signals_total",
			Help: "Total number of signals processed by status and signal type.",
		},
		[]string{"status", "signal_type"},
	)

	// EmbeddingTokensTotal counts total tokens processed by signal type.
	EmbeddingTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_tokens_total",
			Help: "Total number of tokens processed by signal type.",
		},
		[]string{"signal_type", "token_type"},
	)

	// EmbeddingBatchesTotal counts total batches processed by status and signal type.
	EmbeddingBatchesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_batches_total",
			Help: "Total number of batches processed by status and signal type.",
		},
		[]string{"status", "signal_type"},
	)

	// EmbeddingBatchSize measures batch sizes by signal type.
	EmbeddingBatchSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "embedding_batch_size",
			Help:    "Batch sizes by signal type.",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"signal_type"},
	)

	// EmbeddingThroughput measures signals processed per second by signal type.
	EmbeddingThroughput = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "embedding_throughput_signals_per_second",
			Help: "Signals processed per second by signal type.",
		},
		[]string{"signal_type"},
	)

	// EmbeddingErrorsTotal counts total errors by error type and signal type.
	EmbeddingErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_errors_total",
			Help: "Total number of errors by error type and signal type.",
		},
		[]string{"error_type", "signal_type"},
	)

	// EmbeddingRetriesTotal counts total retries by signal type.
	EmbeddingRetriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_retries_total",
			Help: "Total number of retries by signal type.",
		},
		[]string{"signal_type"},
	)

	// EmbeddingVectorStoreWritesTotal counts total vector store writes by status.
	EmbeddingVectorStoreWritesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_vectorstore_writes_total",
			Help: "Total number of vector store writes by status.",
		},
		[]string{"status", "vectorstore_provider", "vectorstore_name"},
	)

	// EmbeddingVectorStoreLatency measures vector store write latency.
	EmbeddingVectorStoreLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "embedding_vectorstore_latency_seconds",
			Help:    "Vector store write latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"vectorstore_provider", "vectorstore_name"},
	)

	// EmbeddingTemplateRendersTotal counts total template renders by status.
	EmbeddingTemplateRendersTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_template_renders_total",
			Help: "Total number of template renders by status.",
		},
		[]string{"status", "template_type"},
	)

	// EmbeddingTemplateLatency measures template render latency.
	EmbeddingTemplateLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "embedding_template_latency_seconds",
			Help:    "Template render latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"template_type"},
	)

	// EmbeddingQualityScore measures embedding quality scores.
	EmbeddingQualityScore = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "embedding_quality_score",
			Help:    "Embedding quality scores (0.0 to 1.0).",
			Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		},
		[]string{"signal_type"},
	)

	// EmbeddingMemoryUsage measures memory usage in bytes.
	EmbeddingMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "embedding_memory_usage_bytes",
			Help: "Memory usage in bytes by signal type.",
		},
		[]string{"signal_type", "memory_type"},
	)

	// EmbeddingActiveJobs measures currently active embedding jobs.
	EmbeddingActiveJobs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "embedding_active_jobs",
			Help: "Currently active embedding jobs by signal type.",
		},
		[]string{"signal_type", "worker_id"},
	)

	// EmbeddingWorkerUptime measures worker uptime in seconds.
	EmbeddingWorkerUptime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "embedding_worker_uptime_seconds",
			Help: "Worker uptime in seconds by worker ID.",
		},
		[]string{"worker_id", "signal_type"},
	)

	// EmbeddingCircuitBreakerState tracks circuit breaker state per vector store.
	EmbeddingCircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "embedding_circuit_breaker_state",
			Help: "Circuit breaker state per vector store (0=closed, 1=half-open, 2=open).",
		},
		[]string{"vectorstore_name", "vectorstore_provider"},
	)

	// EmbeddingCircuitBreakerFailures counts failures per vector store.
	EmbeddingCircuitBreakerFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_circuit_breaker_failures_total",
			Help: "Total failures per vector store that triggered circuit breaker.",
		},
		[]string{"vectorstore_name", "vectorstore_provider"},
	)

	// EmbeddingVectorStoreSuccessRate tracks success rate per vector store.
	EmbeddingVectorStoreSuccessRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "embedding_vectorstore_success_rate",
			Help: "Success rate per vector store (0.0 to 1.0).",
		},
		[]string{"vectorstore_name", "vectorstore_provider"},
	)

	// EmbeddingVectorStoreOperationsTotal counts operations per vector store.
	EmbeddingVectorStoreOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "embedding_vectorstore_operations_total",
			Help: "Total operations per vector store by status.",
		},
		[]string{"status", "vectorstore_name", "vectorstore_provider"},
	)
)

func registerEmbeddingMetrics() {
	prometheus.MustRegister(
		EmbeddingJobsTotal,
		EmbeddingJobsDuration,
		EmbeddingSignalsTotal,
		EmbeddingTokensTotal,
		EmbeddingBatchesTotal,
		EmbeddingBatchSize,
		EmbeddingThroughput,
		EmbeddingErrorsTotal,
		EmbeddingRetriesTotal,
		EmbeddingVectorStoreWritesTotal,
		EmbeddingVectorStoreLatency,
		EmbeddingTemplateRendersTotal,
		EmbeddingTemplateLatency,
		EmbeddingQualityScore,
		EmbeddingMemoryUsage,
		EmbeddingActiveJobs,
		EmbeddingWorkerUptime,
		EmbeddingCircuitBreakerState,
		EmbeddingCircuitBreakerFailures,
		EmbeddingVectorStoreSuccessRate,
		EmbeddingVectorStoreOperationsTotal,
	)
}
