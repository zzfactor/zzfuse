package repository

import (
	"context"
	"fmt"
	"maps"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
	"github.com/medxops/trazr-otel/pkg/util"
	"go.uber.org/zap"
)

// ==== Types and Interfaces ====
// ChunkingConfig holds configuration for chunking per signal type.
type ChunkingConfig struct {
	MaxMessageLength   int
	SplitOnNewline     bool
	MaxSpansPerChunk   int
	MaxMetricsPerBatch int
}

// embeddingEventHandler implements event handling for embedding triggers used by the event bus path
type embeddingEventHandler struct {
	schema string
	done   <-chan struct{}
	notify func()
}

func (h *embeddingEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	// Fast-cancel if worker is stopping
	select {
	case <-h.done:
		return nil
	default:
	}
	// Schema filter (only process events for our schema)
	if h.schema != "" && ev.Schema != h.schema {
		return nil
	}
	// Signal the worker loop
	if h.notify != nil {
		h.notify()
	}
	return nil
}

// VectorStoreConfig holds configuration for a specific vector store that this worker handles
type VectorStoreConfig struct {
	VectorStoreKey   string                          // "mapping1_pgvector1"
	ConfigID         int64                           // Unique config ID per vector store
	VectorStore      VectorStoreClient               // Vector store client
	EmbeddingConfig  database.EmbeddingConfig        // Embedding configuration
	MappingConfig    database.ProcessorMappingConfig // Mapping configuration
	SignalTypes      []string                        // Signal types this vector store wants
	TemplateContent  map[string]string               // Template content for this vector store
	TemplateMetadata map[string][]string             // Template metadata for this vector store
	// References to pre-compiled template engines for performance
	TemplateID     int64                          // Template ID for engine lookup
	ContentEngine  *content.ContentTemplateEngine // Reference to pre-compiled content engine
	MetadataEngine *content.ContentTemplateEngine // Reference to pre-compiled metadata engine
}

// EmbeddingWorker handles batch embedding jobs for a specific signal type.
//
// Enhanced with state management, statistics collection, and base worker functionality.
// Batch Processing Controls:
//   - FlushInterval: Interval to wait after each batch (0 disables)
//   - MaxRetries: Maximum retry attempts for failed batch operations
//   - BatchErrorPolicy: Error policy for batch failures ("continue", "stop", "retry")
//   - MaxConcurrentBatches: Maximum concurrent batch operations (0 disables)
//   - RetryBackoffBase/Max: Exponential backoff base/max for retries
//
// Use functional options (WithX) to configure these fields.
type EmbeddingWorker struct {
	// Base worker functionality
	*BaseWorker

	// Config
	ChunkingConfig   map[string]string
	FilterConfig     map[string]interface{}
	FetchOptions     *FetchOptions
	MaxContextLength int                    // NEW: enforce token limit from embedding config
	Metadata         map[string]interface{} // Metadata from ProcessorMappingConfig
	metadataConfig   metadataConfig         // Parsed metadata configuration

	// Dependencies
	DB          OtelDatabaseClient // Use unified interface
	VectorStore VectorStoreClient
	// Optional event bus for trigger mode
	eventBus event.EventBusInterface

	// NEW: Map of vector store configs this worker handles
	VectorStoreConfigs map[string]*VectorStoreConfig

	// Token Management
	tokenizer   Tokenizer
	tokenBudget *TokenBudget

	// Metrics
	metricsCollector  *EmbeddingMetricsCollector
	currentJobStats   *EmbeddingJobStats
	currentJobDetails *EmbeddingJobDetails
	batchMu           sync.Mutex // Serializes a production batch's per-job state.
}

// WorkerStatus tracks running state and last error for observability.
type WorkerStatus struct {
	Running     bool
	LastError   error
	LastRunTime int64 // Unix timestamp
}

// EmbeddingWorkerOption is a functional option for configuring the worker.
type EmbeddingWorkerOption func(*EmbeddingWorker)

// WithBatchSize sets the batch size for the worker. Use to tune throughput vs. memory usage.
func WithBatchSize(size int) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.BaseWorker.BatchSize = size }
}

func WithWorkerFetchOptions(options *FetchOptions) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) {
		if options != nil {
			copy := *options
			w.FetchOptions = &copy
		}
	}
}

// WithChunkingConfig sets the chunking config for the worker. Config string format: key1=val1;key2=val2. See chunkSignals for parsing logic.
func WithChunkingConfig(cfg map[string]string) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.ChunkingConfig = cfg }
}

// WithFilterConfig sets the filter config for the worker.
func WithFilterConfig(fc map[string]interface{}) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.FilterConfig = fc }
}

// WithConfigID sets the config ID for the worker.
func WithConfigID(configID int64) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.BaseWorker.ConfigID = configID }
}

// WithMaxContextLength sets the max context length (token limit) for the worker.
func WithMaxContextLength(limit int) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) {
		// Use the default tokenizer
		tokenizer := GetDefaultTokenizer()

		// Use database max context length
		maxTokens := limit
		if maxTokens <= 0 {
			maxTokens = 8192 // Default context length
		}

		w.tokenizer = tokenizer
		w.MaxContextLength = maxTokens
		w.tokenBudget = NewTokenBudget(tokenizer, maxTokens, 0)

		w.Logger.Info("Configured tokenizer with max context length",
			"max_tokens", maxTokens,
			"tokenizer", tokenizer.GetName())
	}
}

// WithSignalType sets the signal type for the worker.
func WithSignalType(signalType string) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.BaseWorker.SignalType = signalType }
}

// WithWorkerID sets the worker ID for the worker.
func WithWorkerID(id string) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.BaseWorker.WorkerID = id }
}

// WithMetadata sets the metadata configuration for the worker.
func WithMetadata(metadata map[string]interface{}) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.Metadata = maps.Clone(metadata) }
}

// WithContentProcessingMode sets the content processing mode for the worker.
func WithContentProcessingMode(mode string) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) {
		if w.Metadata == nil {
			w.Metadata = make(map[string]interface{})
		}
		w.Metadata["content_processing_mode"] = mode
		// Note: Logger might not be available during initialization, so we can't log here
	}
}

// WithFlushInterval sets the flush interval for batch processing.
// If interval is 0, no delay is added after batches.
func WithFlushInterval(interval time.Duration) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.FlushInterval = interval }
}

// WithMaxRetries sets the maximum number of retry attempts for failed operations.
// If maxRetries is 0, no retries are performed.
func WithMaxRetries(maxRetries int) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.MaxRetries = maxRetries }
}

// WithBatchErrorPolicy sets the error policy for batch processing.
// Valid values: "continue" (log and continue), "stop" (abort worker), "retry" (retry up to MaxRetries).
func WithBatchErrorPolicy(policy string) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.BatchErrorPolicy = policy }
}

// WithMaxConcurrentBatches sets the maximum number of concurrent batch operations.
// If maxConcurrent is 0, concurrency is unlimited.
func WithMaxConcurrentBatches(maxConcurrent int) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.MaxConcurrentBatches = maxConcurrent }
}

// WithRetryBackoff sets the retry backoff configuration for exponential backoff.
// base: initial delay; max: maximum delay.
func WithRetryBackoff(base, max time.Duration) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) {
		w.RetryBackoffBase = base
		w.RetryBackoffMax = max
	}
}

// WithMetricsCollector sets the metrics collector for the worker.
func WithMetricsCollector(collector *EmbeddingMetricsCollector) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) {
		w.metricsCollector = collector
	}
}

// WithVectorStoreConfigs sets the vector store configurations for the worker.
func WithVectorStoreConfigs(configs map[string]*VectorStoreConfig) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.VectorStoreConfigs = configs }
}

// WithEventBus sets the event bus to use for trigger mode (preferred over DB Notifier when available)
func WithEventBus(bus event.EventBusInterface) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.eventBus = bus }
}

// WithEmbeddingSchema sets the schema for the worker.
func WithEmbeddingSchema(schema string) EmbeddingWorkerOption {
	return func(w *EmbeddingWorker) { w.BaseWorker.Schema = schema }
}

// ==== Metadata Parsing (Phase 1) ====

// metadataConfig holds parsed metadata configuration with defaults
type metadataConfig struct {
	Mode          string
	BatchInterval time.Duration
	StartTime     int64
	StopTime      int64
	GroupSignals  bool
	DebounceDelay time.Duration // NEW: configurable debounce delay for triggers
}

// parseMetadata extracts and validates metadata configuration
func (w *EmbeddingWorker) parseMetadata() {
	w.metadataConfig = metadataConfig{
		Mode:          "hybrid",
		BatchInterval: 5 * time.Second,
		GroupSignals:  false,
		DebounceDelay: 2 * time.Second, // Default 2 second debounce
	}

	if mode, ok := w.Metadata["embedding_mode"].(string); ok {
		switch mode {
		case "batch", "trigger", "hybrid":
			w.metadataConfig.Mode = mode
		default:
			w.Logger.Debug("Invalid embedding_mode, using default", "mode", mode)
		}
	}

	if interval, ok := w.Metadata["embedding_batch_interval"].(float64); ok && interval > 0 {
		w.metadataConfig.BatchInterval = time.Duration(interval * float64(time.Second))
	}

	if startTime, ok := w.Metadata["embedding_start_time"].(float64); ok && startTime > 0 {
		w.metadataConfig.StartTime = int64(startTime)
	}

	if stopTime, ok := w.Metadata["embedding_stop_time"].(float64); ok && stopTime > 0 {
		w.metadataConfig.StopTime = int64(stopTime)
	}

	if groupSignals, ok := w.Metadata["embedding_batch_group_signals"].(bool); ok {
		w.metadataConfig.GroupSignals = groupSignals
	}

	// NEW: Parse debounce delay for trigger mode
	if debounceDelay, ok := w.Metadata["embedding_trigger_debounce_delay"].(float64); ok && debounceDelay > 0 {
		w.metadataConfig.DebounceDelay = time.Duration(debounceDelay * float64(time.Second))
	}

	// Validate time range consistency
	if w.metadataConfig.StartTime > 0 && w.metadataConfig.StopTime > 0 && w.metadataConfig.StartTime >= w.metadataConfig.StopTime {
		w.Logger.Debug("Invalid time range", "start_time", w.metadataConfig.StartTime, "stop_time", w.metadataConfig.StopTime)
		w.metadataConfig.StartTime, w.metadataConfig.StopTime = 0, 0 // Reset invalid range
	}
}

// Getter methods for metadata configuration
func (w *EmbeddingWorker) ProcessingMode() string {
	return w.metadataConfig.Mode
}

func (w *EmbeddingWorker) BatchInterval() time.Duration {
	return w.metadataConfig.BatchInterval
}

func (w *EmbeddingWorker) StartTime() int64 {
	return w.metadataConfig.StartTime
}

func (w *EmbeddingWorker) StopTime() int64 {
	return w.metadataConfig.StopTime
}

func (w *EmbeddingWorker) GroupSignals() bool {
	return w.metadataConfig.GroupSignals
}

func (w *EmbeddingWorker) DebounceDelay() time.Duration {
	return w.metadataConfig.DebounceDelay
}

// GetTargetVectorStores returns the vector store keys this worker handles
func (w *EmbeddingWorker) GetTargetVectorStores() []string {
	keys := make([]string, 0, len(w.VectorStoreConfigs))
	for key := range w.VectorStoreConfigs {
		keys = append(keys, key)
	}
	return keys
}

// ContentProcessingMode returns the content processing mode for this worker.
func (w *EmbeddingWorker) ContentProcessingMode() string {
	w.Logger.Debug("ContentProcessingMode called", "metadata", w.Metadata)

	if w.Metadata != nil {
		if mode, ok := w.Metadata["content_processing_mode"].(string); ok {
			w.Logger.Debug("Found content processing mode in metadata", "mode", mode)
			return mode
		}
		w.Logger.Debug("No content_processing_mode found in metadata", "available_keys", getMapKeys(w.Metadata))
	} else {
		w.Logger.Debug("Metadata is nil")
	}

	w.Logger.Debug("Using default content processing mode", "default_mode", "template")
	return "template" // Default fallback
}

// getMapKeys returns the keys of a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func getStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ==== Worker Lifecycle ====
// NewEmbeddingWorker constructs a new EmbeddingWorker with required dependencies and options.
func NewEmbeddingWorker(
	db OtelDatabaseClient,
	logger util.Logger,
	vectorStore VectorStoreClient,
	opts ...EmbeddingWorkerOption,
) *EmbeddingWorker {
	// Create base worker with common options
	baseOpts := []BaseWorkerOption{
		WithBaseBatchSize(100),
		WithBaseMaxRetries(3),
		WithBaseBatchErrorPolicy("continue"),
		WithBaseMaxConcurrentBatches(1),
		WithBaseRetryBackoff(1*time.Second, 30*time.Second),
		WithBaseFlushInterval(200 * time.Millisecond),
	}

	baseWorker := NewBaseWorker(db, logger, baseOpts...)

	w := &EmbeddingWorker{
		BaseWorker:     baseWorker,
		DB:             db, // Set the DB field to avoid nil database connection
		VectorStore:    vectorStore,
		ChunkingConfig: map[string]string{},
		FilterConfig:   map[string]interface{}{},
		Metadata:       map[string]interface{}{},
	}

	// Apply embedding worker options
	for _, opt := range opts {
		opt(w)
	}

	// Parse metadata on initialization
	w.parseMetadata()

	// Initialize tokenizer and token budget only if not already set by options
	if w.tokenizer == nil {
		w.tokenizer = GetDefaultTokenizer()
		w.tokenBudget = NewTokenBudget(w.tokenizer, 8192, 0) // Default context length
	}

	// Initialize metrics collector if not already set
	if w.metricsCollector == nil {
		w.metricsCollector = NewEmbeddingMetricsCollector(
			w.ConfigID,
			w.SignalType,
			w.ProcessingMode(),
			"default", // embedding provider - will be updated when available
			"default", // vector store provider - will be updated when available
			w.WorkerID,
		)
	}

	w.initTemplateCache()
	return w
}

// ==== Batch Processing ====
// Start begins the worker loop based on the processing mode.
func (w *EmbeddingWorker) Start() {
	w.mutex.Lock()
	if w.status.Running || w.IsStopped() {
		w.mutex.Unlock()
		return
	}
	w.status.Running = true
	w.mutex.Unlock()
	defer func() {
		w.mutex.Lock()
		w.status.Running = false
		w.mutex.Unlock()
	}()

	switch w.ProcessingMode() {
	case "batch":
		w.startBatchMode()
	case "trigger":
		w.startTriggerMode()
	case "hybrid":
		w.startHybridMode()
	default:
		w.Logger.Error("Unknown processing mode, defaulting to hybrid", "mode", w.ProcessingMode())
		w.startHybridMode()
	}
}

// startBatchMode runs the traditional batch processing with periodic intervals
func (w *EmbeddingWorker) startBatchMode() {
	// Ensure we have a valid batch interval
	batchInterval := w.metadataConfig.BatchInterval
	if batchInterval <= 0 {
		batchInterval = 5 * time.Second // Default fallback
		w.Logger.Debug("Invalid batch interval, using default", "interval", batchInterval)
	}

	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopped:
			w.Logger.Debug("EmbeddingWorker stopped")
			return
		case <-w.paused:
			w.togglePauseState()
		case <-ticker.C:
			if w.IsPaused() {
				continue // Skip this tick if paused
			}
			w.Logger.Info("Starting batch processing",
				zap.String("signal_type", w.SignalType),
				zap.String("worker_id", w.WorkerID))
			w.executeBatchWithProtection("batch")
		}
	}
}

// togglePauseState safely toggles the pause state with mutex protection
func (w *EmbeddingWorker) togglePauseState() {
	w.mutex.Lock()
	w.isPaused = !w.isPaused
	isPaused := w.isPaused
	w.mutex.Unlock()

	if isPaused {
		w.Logger.Info("EmbeddingWorker paused")
	} else {
		w.Logger.Info("EmbeddingWorker resumed")
	}
}

// startTriggerMode runs trigger-based processing using database notifications with debouncing
func (w *EmbeddingWorker) startTriggerMode() {
	w.Logger.Info("Starting trigger mode", "signal_type", w.SignalType)

	// Check eventBus status
	w.Logger.Debug("Checking eventBus status for trigger mode",
		zap.String("signal_type", w.SignalType),
		zap.Bool("has_event_bus", w.eventBus != nil))

	// Try preferred EventBus path first
	if w.eventBus != nil {
		w.Logger.Info("Using EventBus for trigger mode", "signal_type", w.SignalType)
		// Use the worker's configured schema to filter events
		schema := w.Schema
		if schema == "" {
			schema = DefaultSchema // fallback for backwards compatibility
		}

		// Select event types for this signal
		var eventTypes []EventType
		switch w.SignalType {
		case SignalTypeLogs:
			eventTypes = []EventType{EventTypeLogInserted}
		case SignalTypeSpans:
			eventTypes = []EventType{EventTypeTraceInserted}
		case SignalTypeGaugeMetrics, SignalTypeSumMetrics, SignalTypeHistogramMetrics, SignalTypeSummaryMetrics, SignalTypeExponentialHistogramMetrics:
			// Prefer unified metric event if producers emit it
			eventTypes = []EventType{EventTypeMetricInserted}
		case "profiles":
			eventTypes = []EventType{EventTypeProfileInserted}
		default:
			// Fallback to unified metric event to catch any producers
			eventTypes = []EventType{EventTypeMetricInserted}
		}

		// Channel and done signal for handler
		msgCh := make(chan struct{}, streamBufferSize)
		done := make(chan struct{})

		// Define handler that checks schema and signals work
		handler := &embeddingEventHandler{
			schema: schema,
			done:   done,
			notify: func() {
				select {
				case msgCh <- struct{}{}:
				default:
				}
			},
		}

		// Register handlers using Subscribe to get closeable subscriptions
		subscriptions := make([]event.Subscription, 0, len(eventTypes))
		for _, et := range eventTypes {
			sub := w.eventBus.Subscribe(et.ToEventType(), handler)
			subscriptions = append(subscriptions, sub)
		}
		w.Logger.Debug("Subscribed to event bus for trigger mode", "schema", schema, "event_types", eventTypes, "subscription_count", len(subscriptions))

		// Debounce loop with race-safe timer handling
		// Notifications reduce latency. Database polling remains authoritative
		// when delivery is missed, NATS reconnects, or an expired lease returns.
		pollInterval := w.metadataConfig.BatchInterval
		if pollInterval <= 0 {
			pollInterval = 5 * time.Second
		}
		pollTicker := time.NewTicker(pollInterval)
		defer pollTicker.Stop()
		debounceDelay := w.DebounceDelay()
		var triggerCount int
		var timerActive bool
		debounceTimer := time.NewTimer(debounceDelay)
		debounceTimer.Stop() // Start with timer stopped

		// Cleanup function to unregister handlers
		cleanup := func() {
			debounceTimer.Stop()
			close(done)
			// Unregister all subscriptions to prevent goroutine leaks
			for _, sub := range subscriptions {
				if err := sub.Close(); err != nil {
					w.Logger.Debug("Failed to close subscription", "error", err)
				}
			}
			w.Logger.Debug("Cleaned up event bus subscriptions", "count", len(subscriptions))
		}

		// resetTimer safely resets the debounce timer
		resetTimer := func() {
			if timerActive {
				// Timer is already running, just increment count
				return
			}
			// Drain any pending timer event before reset
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
			debounceTimer.Reset(debounceDelay)
			timerActive = true
		}

		for {
			select {
			case <-w.stopped:
				cleanup()
				w.Logger.Debug("EmbeddingWorker stopped")
				return
			case <-w.paused:
				w.togglePauseState()
			case <-msgCh:
				if w.IsPaused() {
					continue
				}
				triggerCount++
				resetTimer()
			case <-pollTicker.C:
				if !w.IsPaused() {
					w.executeBatchWithProtection("trigger-poll")
				}
			case <-debounceTimer.C:
				timerActive = false
				if w.IsPaused() {
					continue
				}
				if triggerCount > 0 {
					currentCount := triggerCount
					triggerCount = 0 // Reset count BEFORE processing to capture new events

					w.Logger.Info("Processing debounced triggers (event bus)",
						zap.String("signal_type", w.SignalType),
						zap.String("worker_id", w.WorkerID),
						zap.Int("trigger_count", currentCount))
					w.executeBatchWithProtection("trigger")

					// Check if new triggers arrived during batch processing
					// If so, restart the timer to process them
					if triggerCount > 0 {
						resetTimer()
					}
				}
			}
		}
	}

	// EventBus not available: fall back to batch mode
	w.Logger.Info("EventBus not configured; falling back to batch mode for trigger processing",
		zap.String("signal_type", w.SignalType))
	w.startBatchMode()
	return
}

// startHybridMode runs trigger-based processing with an initial batch on startup
func (w *EmbeddingWorker) startHybridMode() {

	// Run initial batch to process any existing unembedded signals
	w.executeBatchWithProtection("hybrid_initial")

	// Then switch to trigger-only mode
	w.Logger.Debug("Initial batch complete, switching to trigger mode", "signal_type", w.SignalType)
	w.startTriggerMode()
}

// Stop signals the worker to stop gracefully.
func (w *EmbeddingWorker) Stop() {
	w.BaseWorker.Stop()
}

// Pause pauses the worker.
func (w *EmbeddingWorker) Pause() {
	w.BaseWorker.Pause()
}

// Resume resumes the worker.
func (w *EmbeddingWorker) Resume() {
	w.BaseWorker.Resume()
}

// ReloadConfig reloads the worker configuration (placeholder).
func (w *EmbeddingWorker) ReloadConfig(newCfg *EmbeddingWorker) {
	// TODO: Implement dynamic config reload
}

// ReportError reports an error to the worker status.
func (w *EmbeddingWorker) ReportError(err error) {
	w.BaseWorker.ReportError(err)
}

// executeBatchWithProtection executes a batch with job overlap protection
// This centralizes the batch execution logic used by both batch and trigger modes
func (w *EmbeddingWorker) executeBatchWithProtection(mode string) {
	// Check if a batch is already running (same protection for all modes)
	if !w.tryLock() {
		w.Logger.Debug("Previous batch still in progress, skipping processing", "mode", mode, "signal_type", w.SignalType, "worker_id", w.WorkerID)
		return
	}

	// Create context with timeout for the entire batch operation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Run the batch and ensure we unlock when done
	if err := w.RunBatch(ctx); err != nil {
		w.ReportError(err)
		w.Logger.Error("Batch execution failed", "mode", mode, "error", err, "signal_type", w.SignalType, "worker_id", w.WorkerID)
	}
	w.unlock() // Explicitly unlock after batch completes
}

// ==== Template Logic ====
// initTemplateCache populates the template cache with sprig and custom helpers for embedding templates.
func (w *EmbeddingWorker) initTemplateCache() {
	// ✅ REMOVED: No longer needed - we use pre-compiled template engines from VectorStoreConfigs
	// The template engines are now managed by TemplateService and stored in VectorStoreConfig.ContentEngine

	w.Logger.Debug("initTemplateCache: Using pre-compiled template engines from VectorStoreConfigs")
}

// selectMetadataFields extracts only the specified fields from the signal data. Supports dot notation for nested fields.
func selectMetadataFields(data map[string]interface{}, fields []string) map[string]interface{} {
	result := make(map[string]interface{})
	for _, field := range fields {
		if val, ok := getNestedField(data, field); ok {
			result[field] = val
		}
	}
	return result
}

// getNestedField retrieves a value from a map using dot notation (e.g., attributes.service_name)
func getNestedField(data map[string]interface{}, field string) (interface{}, bool) {
	// First, try direct field access
	if val, ok := data[field]; ok {
		return val, true
	}

	// If field contains dots, try to navigate nested objects
	if strings.Contains(field, ".") {
		parts := strings.Split(field, ".")
		var current interface{} = data

		// Try to navigate as nested objects first
		for i, part := range parts {
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil, false
			}
			current, ok = m[part]
			if !ok {
				// If we can't find this part, try to find a key that contains the remaining parts
				remainingParts := parts[i:]
				remainingKey := strings.Join(remainingParts, ".")

				// Look for a key that matches the remaining parts
				for key, value := range m {
					if key == remainingKey {
						return value, true
					}
				}

				// If not found, try mapping underscores to dots for resource attributes
				if parts[0] == "resource_attributes" && len(remainingParts) > 0 {
					// Convert underscores back to dots for the key lookup
					dotKey := strings.ReplaceAll(remainingKey, "_", ".")
					if val, exists := m[dotKey]; exists {
						return val, true
					}
				}

				return nil, false
			}
		}
		return current, true
	}

	return nil, false
}

// getSignalTypesFromEmbeddingFilter extracts signal types from embedding filter configuration
func getSignalTypesFromEmbeddingFilter(filterConfig map[string]interface{}) []string {
	if filterConfig == nil {
		return AllSupportedSignalTypes() // Default to all if no filter
	}

	// Parse the filter config to extract signal types
	filter, err := ParseFilterConfig(filterConfig)
	if err != nil {
		// If parsing fails, default to all signal types
		return AllSupportedSignalTypes()
	}

	return getSignalTypesFromFilterConfig(filter)
}

// getSignalTypesFromDatabaseFilter extracts signal types from database filter configuration
func getSignalTypesFromDatabaseFilter(dbFilterConfig map[string]interface{}) []string {
	if dbFilterConfig == nil {
		return AllSupportedSignalTypes() // Default to all if no filter
	}

	// Parse the database filter config to extract signal types
	filter, err := ParseFilterConfig(dbFilterConfig)
	if err != nil {
		// If parsing fails, default to all signal types
		return AllSupportedSignalTypes()
	}

	return getSignalTypesFromFilterConfig(filter)
}

// getSignalTypesFromFilterConfig extracts signal types from a parsed FilterConfig
func getSignalTypesFromFilterConfig(filter *FilterConfig) []string {
	if filter == nil || filter.SignalType == "" || filter.SignalType == "*" {
		// No signal type filter or wildcard - return all supported types
		return AllSupportedSignalTypes()
	}

	if strings.Contains(filter.SignalType, ",") {
		// Multiple signal types specified
		types := strings.Split(filter.SignalType, ",")
		for i, t := range types {
			types[i] = strings.TrimSpace(t)
		}
		return types
	}

	// Single signal type
	return []string{filter.SignalType}
}

// getIntersectionOfSignalTypes returns the intersection of two signal type slices
func getIntersectionOfSignalTypes(types1, types2 []string) []string {
	// Create a map for O(1) lookup
	typeMap := make(map[string]bool)
	for _, t := range types1 {
		typeMap[t] = true
	}

	var intersection []string
	for _, t := range types2 {
		if typeMap[t] {
			intersection = append(intersection, t)
		}
	}

	return intersection
}

// validateSignalTypes validates that all signal types are supported
func validateSignalTypes(signalTypes []string) []string {
	validTypes := AllSupportedSignalTypes()
	validMap := make(map[string]bool)
	for _, t := range validTypes {
		validMap[t] = true
	}

	var validated []string
	for _, t := range signalTypes {
		if validMap[t] {
			validated = append(validated, t)
		}
	}

	return validated
}

// CreateEmbeddingWorkersWithDatabaseFilter creates and returns a slice of EmbeddingWorker pointers for signal types
// that are targeted by both the embedding filter and database filter.
func CreateEmbeddingWorkersWithDatabaseFilter(
	ctx context.Context,
	db OtelDatabaseClient,
	logger util.Logger,
	vectorStore VectorStoreClient,
	embConfig database.EmbeddingConfig,
	vsConfig database.VectorStoreConfig,
	mapping database.ProcessorMappingConfig,
	databaseFilter map[string]interface{}, // 🔑 Database signal filter
) ([]*EmbeddingWorker, error) {
	var workers []*EmbeddingWorker
	batchSize := vsConfig.BatchSize
	if batchSize == 0 {
		batchSize = 100
	}
	intervalSeconds := vsConfig.MaxIdleConns
	if intervalSeconds == 0 {
		intervalSeconds = 5
	}

	// Get the underlying *sql.DB from the database client
	typedDB, ok := db.(*OtelSQLDatabaseClient)
	if !ok {
		return nil, fmt.Errorf("db is not a supported database client type; got %T", db)
	}
	underlyingDB := typedDB.SQLDatabaseClient.DB

	templateRepo, err := database.NewTemplateConfigRepo(ctx, underlyingDB, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create template repository: %w", err)
	}

	// Debug: Log template loading attempt
	logger.Info("Loading template from database",
		"template_id", mapping.TemplateID,
		"schema", DefaultConfigSchema)

	tpl, err := templateRepo.GetByID(ctx, mapping.TemplateID)
	if err != nil {
		logger.Error("Failed to load template from database",
			"template_id", mapping.TemplateID,
			"error", err)
		return nil, err
	}

	// Debug: Log template loading success
	logger.Info("Template loaded successfully from database",
		"template_id", tpl.ID,
		"template_name", tpl.Name,
		"template_content_keys", getStringMapKeys(tpl.Content),
		"has_spans_template", func() bool {
			_, exists := tpl.Content["spans"]
			return exists
		}())

	// 🔑 SMART WORKER CREATION: Determine which signal types to create workers for
	embeddingSignalTypes := getSignalTypesFromEmbeddingFilter(mapping.EmbeddingFilter)
	databaseSignalTypes := getSignalTypesFromDatabaseFilter(databaseFilter)

	// Get intersection of embedding and database signal types
	targetSignalTypes := getIntersectionOfSignalTypes(embeddingSignalTypes, databaseSignalTypes)

	// Validate signal types
	targetSignalTypes = validateSignalTypes(targetSignalTypes)

	// If no valid signal types found, default to all supported types
	if len(targetSignalTypes) == 0 {
		logger.Info("No valid signal types found in filters, defaulting to all supported types")
		targetSignalTypes = AllSupportedSignalTypes()
	}

	logger.Info("Creating embedding workers",
		"embedding_signal_types", embeddingSignalTypes,
		"database_signal_types", databaseSignalTypes,
		"target_signal_types", targetSignalTypes,
		"total_workers", len(targetSignalTypes))

	// Create workers only for the target signal types
	for _, signalType := range targetSignalTypes {
		workerID := fmt.Sprintf("%s-%d", signalType, embConfig.ID)
		logger.Info("Creating embedding worker",
			"signal_type", signalType,
			"processing_mode", mapping.ProcessingMode,
			"worker_id", workerID,
			"mapping_metadata", mapping.Metadata)

		worker := NewEmbeddingWorker(
			db,
			logger,
			vectorStore,
			WithBatchSize(batchSize),
			WithConfigID(embConfig.ID),
			// ✅ REMOVED: Legacy template options - templates are now managed through VectorStoreConfigs
			// WithTemplateContent(tpl.Content),
			// WithTemplateMetadata(tpl.TemplateMetadata),
			WithSignalType(signalType),
			WithWorkerID(workerID),
			WithFilterConfig(mapping.EmbeddingFilter),
			WithMetadata(mapping.Metadata),
			WithContentProcessingMode(mapping.ProcessingMode), // Pass the processing mode from mapping
			// Database-driven tokenizer configuration
			WithMaxContextLength(embConfig.MaxContextLength),
			// Batch Processing Controls from vector store config
			WithFlushInterval(vsConfig.FlushInterval),
			WithMaxRetries(vsConfig.MaxRetries),
			WithBatchErrorPolicy(vsConfig.BatchErrorPolicy),
			WithMaxConcurrentBatches(vsConfig.MaxConcurrentBatches),
			WithRetryBackoff(1*time.Second, 30*time.Second), // Default backoff settings
		)
		workers = append(workers, worker)
	}
	return workers, nil
}

// CreateEmbeddingWorkers creates and returns a slice of EmbeddingWorker pointers for signal types
// that are targeted by both the embedding filter and database filter.
func CreateEmbeddingWorkers(
	ctx context.Context,
	db OtelDatabaseClient,
	logger util.Logger,
	vectorStore VectorStoreClient,
	embConfig database.EmbeddingConfig,
	vsConfig database.VectorStoreConfig,
	mapping database.ProcessorMappingConfig,
) ([]*EmbeddingWorker, error) {
	// Call the enhanced version with nil database filter (backward compatibility)
	return CreateEmbeddingWorkersWithDatabaseFilter(ctx, db, logger, vectorStore, embConfig, vsConfig, mapping, nil)
}

// GetMetrics returns the current embedding metrics
func (w *EmbeddingWorker) GetMetrics() *EmbeddingMetrics {
	if w.metricsCollector != nil {
		return w.metricsCollector.GetMetrics()
	}
	return nil
}

// LogMetrics logs the current metrics for debugging/monitoring
func (w *EmbeddingWorker) LogMetrics(ctx context.Context) {
	if w.metricsCollector != nil {
		w.metricsCollector.LogMetrics(ctx)
	}
}

// ResetMetrics resets all metrics
func (w *EmbeddingWorker) ResetMetrics() {
	if w.metricsCollector != nil {
		w.metricsCollector.ResetMetrics()
	}
}

// getCurrentJobStats returns the current job stats
func (w *EmbeddingWorker) getCurrentJobStats() *EmbeddingJobStats {
	return w.currentJobStats
}

// getCurrentMemoryUsage returns the current memory usage in bytes
func (w *EmbeddingWorker) getCurrentMemoryUsage() int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Alloc) // Returns allocated memory in bytes
}

// GetMemoryStats returns comprehensive memory statistics
func (w *EmbeddingWorker) GetMemoryStats() map[string]int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]int64{
		"alloc":          int64(m.Alloc),        // Currently allocated memory
		"total_alloc":    int64(m.TotalAlloc),   // Total memory allocated
		"sys":            int64(m.Sys),          // Total system memory
		"heap_alloc":     int64(m.HeapAlloc),    // Heap allocated memory
		"heap_sys":       int64(m.HeapSys),      // Heap system memory
		"heap_idle":      int64(m.HeapIdle),     // Heap idle memory
		"heap_inuse":     int64(m.HeapInuse),    // Heap in-use memory
		"heap_released":  int64(m.HeapReleased), // Heap released memory
		"heap_objects":   int64(m.HeapObjects),  // Number of heap objects
		"stack_inuse":    int64(m.StackInuse),   // Stack in-use memory
		"stack_sys":      int64(m.StackSys),     // Stack system memory
		"mspan_inuse":    int64(m.MSpanInuse),   // MSpan in-use memory
		"mspan_sys":      int64(m.MSpanSys),     // MSpan system memory
		"mcache_inuse":   int64(m.MCacheInuse),  // MCache in-use memory
		"mcache_sys":     int64(m.MCacheSys),    // MCache system memory
		"buck_hash_sys":  int64(m.BuckHashSys),  // Bucket hash table memory
		"gc_sys":         int64(m.GCSys),        // GC metadata memory
		"other_sys":      int64(m.OtherSys),     // Other system memory
		"next_gc":        int64(m.NextGC),       // Next GC target
		"last_gc":        int64(m.LastGC),       // Last GC time
		"pause_total_ns": int64(m.PauseTotalNs), // Total GC pause time
		"num_gc":         int64(m.NumGC),        // Number of GC cycles
		"num_forced_gc":  int64(m.NumForcedGC),  // Number of forced GC cycles
	}
}

// GetMemoryUsageSummary returns a human-readable memory usage summary
func (w *EmbeddingWorker) GetMemoryUsageSummary() map[string]string {
	stats := w.GetMemoryStats()

	// Convert bytes to human-readable format
	formatBytes := func(bytes int64) string {
		const unit = 1024
		if bytes < unit {
			return fmt.Sprintf("%d B", bytes)
		}
		div, exp := int64(unit), 0
		for n := bytes / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
	}

	return map[string]string{
		"alloc":        formatBytes(stats["alloc"]),
		"total_alloc":  formatBytes(stats["total_alloc"]),
		"sys":          formatBytes(stats["sys"]),
		"heap_alloc":   formatBytes(stats["heap_alloc"]),
		"heap_sys":     formatBytes(stats["heap_sys"]),
		"heap_idle":    formatBytes(stats["heap_idle"]),
		"heap_inuse":   formatBytes(stats["heap_inuse"]),
		"heap_objects": fmt.Sprintf("%d", stats["heap_objects"]),
		"num_gc":       fmt.Sprintf("%d", stats["num_gc"]),
		"last_gc":      fmt.Sprintf("%d", stats["last_gc"]),
	}
}
