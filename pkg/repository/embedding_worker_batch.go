package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
)

// Safety margin for token estimation (80% of the configured limit)
const tokenSafetyMargin = 0.8

// ==== Batch Processing ====
// RunBatch runs a single batch embedding job with real integration pipeline.
// It fetches unembedded signals, chunks or groups them according to config, embeds them, and marks them as embedded.
func (w *EmbeddingWorker) RunBatch(ctx context.Context) error {
	// A worker owns one current attempt. Serialize direct callers as well as the
	// lifecycle loop so overlapping triggers cannot overwrite its job statistics.
	w.batchMu.Lock()
	defer w.batchMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	w.Logger.Info("Starting batch processing",
		zap.String("signal_type", w.SignalType),
		zap.String("worker_id", w.WorkerID),
		zap.Int64("processor_id", w.ConfigID),
		zap.Int("batch_size", w.BatchSize))

	// Initialize metrics collection
	success := false
	if w.metricsCollector != nil {
		jobStats := w.metricsCollector.RecordJobStart(w.WorkerID, 0, 0)
		w.currentJobStats = jobStats
		w.currentJobDetails = &EmbeddingJobDetails{}
		defer func() {
			if !success && w.currentJobDetails.ErrorCount == 0 {
				w.currentJobDetails.ErrorCount = 1
			}
			w.metricsCollector.RecordJobCompletion(jobStats, success, w.currentJobDetails)
			w.currentJobStats = nil
			w.currentJobDetails = nil
		}()
	}

	// Process the batch
	if err := w.processBatchInternal(ctx); err != nil {
		w.Logger.Error("Batch processing failed", "error", err, "signal_type", w.SignalType, "worker_id", w.WorkerID)
		w.ReportError(err)
		return fmt.Errorf("batch processing failed: %w", err)
	}

	w.Logger.Debug("Batch processing completed successfully", "signal_type", w.SignalType, "worker_id", w.WorkerID)
	success = true
	return nil
}

// processBatchInternal contains the core batch processing logic
func (w *EmbeddingWorker) processBatchInternal(ctx context.Context) error {
	// 1. Fetch and Validate
	signals, err := w.fetchSignals(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch signals: %w", err)
	}

	if len(signals) == 0 {
		w.Logger.Info("No signals to process", "signal_type", w.SignalType, "processor_id", w.ConfigID)
		return nil
	}

	// 2. Process (Grouped or Chunked)
	embeddedIDs, err := w.processSignals(ctx, signals)
	if err != nil {
		return fmt.Errorf("failed to process signals: %w", err)
	}

	// 3. Update status. Successful chunks recorded their work as they completed,
	// so a later error cannot erase a previously stored chunk from job metrics.
	w.mutex.Lock()
	w.status.LastRunTime = time.Now().Unix()
	w.mutex.Unlock()

	w.Logger.Debug("Successfully embedded signals", "count", len(embeddedIDs), "signal_type", w.SignalType, "processor_id", w.ConfigID)
	return nil
}

// updateJobMetrics updates job statistics and metrics
func (w *EmbeddingWorker) updateJobMetrics(signals []map[string]interface{}, embeddedIDs []interface{}) {
	if w.metricsCollector == nil {
		return
	}

	// Count tokens for all signals
	totalTokens := 0
	for _, sig := range signals {
		// Get the target vector store key from the signal with safe type assertion
		vectorStoreKey, ok := sig["target_vector_store"].(string)
		if !ok || vectorStoreKey == "" {
			w.Logger.Error("Missing or invalid target_vector_store in signal during token counting",
				"signal_keys", getMapKeys(sig),
				"target_vector_store_value", sig["target_vector_store"])
			continue
		}
		if text := w.extractTextFromSignal(sig, w.SignalType, vectorStoreKey); text != "" {
			totalTokens += estimateTokenCount(text)
		}
	}

	// Update job stats
	if jobStats := w.getCurrentJobStats(); jobStats != nil {
		jobStats.SignalCount += len(embeddedIDs)
		jobStats.TokenCount += totalTokens
	}

	// Record memory usage
	w.metricsCollector.RecordEmbeddingMemoryUsage(w.getCurrentMemoryUsage())

	// Create job details
	if w.currentJobDetails != nil {
		w.currentJobDetails.MemoryUsage = w.getCurrentMemoryUsage()
	}
}

// buildFieldConfigs converts an OptimizedFieldList to SignalFieldConfig slice
func (w *EmbeddingWorker) buildFieldConfigs(signalType string, optimizedList *OptimizedFieldList) []SignalFieldConfig {
	extractor := NewTemplateFieldExtractor()

	// Define all field sources
	fieldSources := [][]string{
		{"id", "timestamp_ns", "service_name"}, // Essential fields
		optimizedList.StaticFields,             // Static fields
		optimizedList.FullJSONBFields,          // Full JSONB fields
	}

	// Add JSONB base fields
	for baseField := range optimizedList.JSONBFields {
		fieldSources = append(fieldSources, []string{baseField})
	}

	// Extract all field configs
	var fieldConfigs []SignalFieldConfig
	for _, fields := range fieldSources {
		for _, field := range fields {
			if config := extractor.getFieldConfig(signalType, field); config != nil {
				fieldConfigs = append(fieldConfigs, *config)
			}
		}
	}

	return fieldConfigs
}

// tryLock attempts to acquire the job lock. Returns false if already locked.
// TODO: Implement proper locking mechanism or remove if not needed
func (w *EmbeddingWorker) tryLock() bool {
	return true
}

// unlock releases the job lock.
// TODO: Implement proper locking mechanism or remove if not needed
func (w *EmbeddingWorker) unlock() {
	// No-op: locking handled by BaseWorker
}

// fetchSignals retrieves unembedded signals from the database and validates them.
func (w *EmbeddingWorker) fetchSignals(ctx context.Context) ([]map[string]interface{}, error) {
	if w.DB == nil {
		w.Logger.Error("Database connection is nil - cannot fetch signals", "signal_type", w.SignalType, "processor_id", w.ConfigID)
		return nil, fmt.Errorf("database connection is nil for signal type %s, config ID %d", w.SignalType, w.ConfigID)
	}

	// NEW: Query for each vector store's config ID
	var allSignals []map[string]interface{}

	// If VectorStoreConfigs is not set, fall back to original behavior
	if w.VectorStoreConfigs == nil || len(w.VectorStoreConfigs) == 0 {
		requiredFields := w.getRequiredFieldsFromTemplates()
		//TODO: this worker has been created with a schema, but we need to pass it to the fetchUnembeddedSignalsAdvanced method
		signals, _, err := w.DB.FetchUnembeddedSignalsAdvanced(ctx, w.SignalType, w.BatchSize, w.ConfigID, w.Schema, DefaultFetchOptions(), requiredFields)
		if err != nil {
			w.Logger.Error("Database fetch error", "error", err, "signal_type", w.SignalType, "processor_id", w.ConfigID)
			w.ReportError(err)
			return nil, fmt.Errorf("database fetch error: %w", err)
		}
		allSignals = signals
	} else {
		// NEW: Query for each vector store's config ID
		for vsKey, vsConfig := range w.VectorStoreConfigs {
			requiredFields := w.getRequiredFieldsFromTemplates()

			w.Logger.Info("Fetching unembedded signals with schema",
				"signal_type", w.SignalType,
				"schema", w.Schema,
				"config_id", vsConfig.ConfigID,
				"vector_store", vsKey)

			// Query database for signals not embedded by this specific config ID
			signals, _, err := w.DB.FetchUnembeddedSignalsAdvanced(
				ctx,
				w.SignalType,
				w.BatchSize,
				vsConfig.ConfigID, // ✅ Query for specific config ID
				w.Schema,
				DefaultFetchOptions(),
				requiredFields,
			)
			if err != nil {
				w.Logger.Error("Database fetch error for vector store",
					"vector_store", vsKey,
					"processor_id", vsConfig.ConfigID,
					"error", err)
				return nil, fmt.Errorf("database fetch error for vector store %s: %w", vsKey, err)
			}

			// Mark signals as belonging to this vector store
			for _, signal := range signals {
				signal["target_vector_store"] = vsKey
				signal["target_processor_id"] = vsConfig.ConfigID
			}

			allSignals = append(allSignals, signals...)
		}
	}

	w.validateSignalIDs(allSignals)

	w.Logger.Info("Fetched and validated signals", "count", len(allSignals), "signal_type", w.SignalType, "processor_id", w.ConfigID)
	return allSignals, nil
}

// getRequiredFieldsFromTemplates extracts required fields from template content and metadata
func (w *EmbeddingWorker) getRequiredFieldsFromTemplates() []SignalFieldConfig {
	// ✅ UPDATED: Use VectorStoreConfigs instead of legacy template fields
	if w.VectorStoreConfigs == nil || len(w.VectorStoreConfigs) == 0 {
		return nil
	}

	// Get the first vector store config (they should all have the same templates)
	var vsConfig *VectorStoreConfig
	for _, config := range w.VectorStoreConfigs {
		vsConfig = config
		break
	}

	if vsConfig == nil || vsConfig.TemplateContent == nil || len(vsConfig.TemplateContent) == 0 {
		return nil
	}

	optimizer := NewTemplateFieldOptimizer()

	// ✅ SIMPLIFIED: Use signal type directly for template lookup
	// Create single-template maps for optimization
	contentTemplates := map[string]string{}
	metadataTemplates := map[string][]string{}

	// Use signal type directly
	if template, exists := vsConfig.TemplateContent[w.SignalType]; exists {
		contentTemplates[w.SignalType] = template
	}

	if metadata, exists := vsConfig.TemplateMetadata[w.SignalType]; exists {
		metadataTemplates[w.SignalType] = metadata
	}

	// Optimize templates to get combined field list
	if len(contentTemplates) > 0 || len(metadataTemplates) > 0 {
		if optimizedLists, err := optimizer.OptimizeTemplates(contentTemplates, metadataTemplates); err == nil {
			if optimizedList := optimizedLists[w.SignalType]; optimizedList != nil {
				fieldConfigs := w.buildFieldConfigs(w.SignalType, optimizedList)
				return fieldConfigs
			}
		} else {
			w.Logger.Error("Failed to optimize templates, using full field selection",
				"signal_type", w.SignalType, "error", err)
		}
	}

	return nil
}

// validateSignalIDs checks for duplicate IDs in the signal batch
func (w *EmbeddingWorker) validateSignalIDs(signals []map[string]interface{}) {
	idCount := make(map[interface{}]int)
	for _, sig := range signals {
		if id, ok := sig["id"]; ok {
			idCount[id]++
		}
	}

	for id, count := range idCount {
		if count > 1 {
			w.Logger.Error("Duplicate signal ID found in batch", "id", id, "count", count, "signal_type", w.SignalType)
		}
	}
}

// processSignals dispatches to the correct processing function based on the worker's configuration.
func (w *EmbeddingWorker) processSignals(ctx context.Context, signals []map[string]interface{}) ([]interface{}, error) {
	if w.isGroupedProcessing() {
		return w.processGroupedSignals(ctx, signals)
	}
	return w.processSingleSignal(ctx, signals)
}

// isGroupedProcessing checks if grouped processing is enabled in metadata
func (w *EmbeddingWorker) isGroupedProcessing() bool {
	if groupSignals, ok := w.Metadata["embedding_batch_group_signals"].(bool); ok {
		return groupSignals
	}
	return false
}

// markSignalsAsEmbedded updates the database to mark the given signal IDs as embedded.
// DEPRECATED: Marking is now handled within embedding methods

// embedAndCollectIDs embeds a batch and returns the IDs of the signals in the batch.
func (w *EmbeddingWorker) embedAndCollectIDs(ctx context.Context, batch []map[string]interface{}) ([]interface{}, error) {
	if err := w.embedBatch(ctx, batch, w.SignalType, w.isGroupedProcessing()); err != nil {
		return nil, err
	}

	var ids []interface{}
	for _, s := range batch {
		ids = append(ids, s["id"])
	}
	return ids, nil
}

// processGroupedSignals handles token-based batching for grouped embeddings.
func (w *EmbeddingWorker) processGroupedSignals(ctx context.Context, signals []map[string]interface{}) ([]interface{}, error) {
	effectiveLimit := int(float64(w.MaxContextLength) * tokenSafetyMargin)
	w.Logger.Info("Grouping signals by token limit", "effective_token_limit", effectiveLimit)

	var allEmbeddedIDs []interface{}
	var currentBatch []map[string]interface{}
	var currentTokenCount int

	for _, sig := range signals {
		// ✅ FIXED: Use simple text extraction for token counting instead of old template rendering
		// The actual template rendering will happen in embedBatch with pre-compiled engines
		text := w.getSimpleTextForTokenCounting(sig)
		tokens := w.countTokens(text)

		if len(currentBatch) > 0 && currentTokenCount+tokens > effectiveLimit {
			embeddedIDs, err := w.embedAndCollectIDs(ctx, currentBatch)
			if err != nil {
				return nil, err
			}
			allEmbeddedIDs = append(allEmbeddedIDs, embeddedIDs...)
			w.updateJobMetrics(currentBatch, embeddedIDs)
			currentBatch = nil
			currentTokenCount = 0
		}

		currentBatch = append(currentBatch, sig)
		currentTokenCount += tokens
	}

	if len(currentBatch) > 0 {
		embeddedIDs, err := w.embedAndCollectIDs(ctx, currentBatch)
		if err != nil {
			return nil, err
		}
		allEmbeddedIDs = append(allEmbeddedIDs, embeddedIDs...)
		w.updateJobMetrics(currentBatch, embeddedIDs)
	}

	return allEmbeddedIDs, nil
}

// getSimpleTextForTokenCounting extracts simple text for token counting without template rendering
func (w *EmbeddingWorker) getSimpleTextForTokenCounting(sig map[string]interface{}) string {
	// Extract basic fields for token counting
	var parts []string

	if name, ok := sig["name"].(string); ok && name != "" {
		parts = append(parts, name)
	}

	if serviceName, ok := sig["service_name"].(string); ok && serviceName != "" {
		parts = append(parts, serviceName)
	}

	if message, ok := sig["message"].(string); ok && message != "" {
		parts = append(parts, message)
	}

	if body, ok := sig["body"].(string); ok && body != "" {
		parts = append(parts, body)
	}

	if value, ok := sig["value"]; ok {
		parts = append(parts, fmt.Sprintf("%v", value))
	}

	// If no meaningful text found, use signal ID
	if len(parts) == 0 {
		if id, ok := sig["id"]; ok {
			parts = append(parts, fmt.Sprintf("Signal %v", id))
		} else {
			parts = append(parts, "Unknown signal")
		}
	}

	return strings.Join(parts, " ")
}

// countTokens safely counts tokens with fallback to estimation
func (w *EmbeddingWorker) countTokens(text string) int {
	if tokens, err := w.countTokensWithTokenizer(text); err == nil {
		return tokens
	}
	return estimateTokenCount(text)
}

// processSingleSignal handles chunking and embedding for non-grouped signals.
func (w *EmbeddingWorker) processSingleSignal(ctx context.Context, signals []map[string]interface{}) ([]interface{}, error) {
	chunks, err := w.chunkSignals(signals)
	if err != nil {
		w.Logger.Error("Chunking error", err)
		w.ReportError(err)
		return nil, err
	}

	var allEmbeddedIDs []interface{}
	for i, chunk := range chunks {
		if err := w.embedBatch(ctx, chunk, w.SignalType, false); err != nil {
			w.Logger.Error("Failed to embed batch", "error", err, "chunk_index", i)
			return nil, err
		}

		// Collect IDs from this chunk
		chunkIDs := w.extractSignalIDs(chunk)
		allEmbeddedIDs = append(allEmbeddedIDs, chunkIDs...)
		w.updateJobMetrics(chunk, chunkIDs)
		w.Logger.Debug("Collected IDs from chunk", "chunk_index", i, "chunk_ids_count", len(chunkIDs), "total_ids_count", len(allEmbeddedIDs))
	}

	w.Logger.Debug("Processed all signals", "total_signals", len(signals), "total_embedded_ids", len(allEmbeddedIDs))
	return allEmbeddedIDs, nil
}

// extractSignalIDs extracts IDs from a slice of signals
func (w *EmbeddingWorker) extractSignalIDs(signals []map[string]interface{}) []interface{} {
	var ids []interface{}
	for _, sig := range signals {
		if id := sig["id"]; id != nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// --- DRY: Chunker Strategy Pattern ---
// ChunkerFunc defines the function signature for chunking strategies by signal type.

// --- DRY: Batch Embedding Helper ---
// embedBatch embeds a batch of signals (grouped or ungrouped), renders templates, creates documents, and stores them in the vector store.
// If grouped is true, the batch is treated as a single document with grouped_ids metadata.
// If grouped is false, each signal becomes a separate document.
func (w *EmbeddingWorker) embedBatch(ctx context.Context, batch []map[string]interface{}, signalType string, grouped bool) error {
	if len(batch) == 0 {
		return nil
	}

	// Extract IDs once
	var signalIDs []interface{}
	var batchIDs []string
	for _, sig := range batch {
		if id, ok := sig["id"]; ok {
			signalIDs = append(signalIDs, id)
			batchIDs = append(batchIDs, fmt.Sprintf("%v", id))
		}
	}

	// Handle nil VectorStore
	if w.VectorStore == nil {
		w.Logger.Info("VectorStore is nil, marking signals as embedded with mock IDs", "signal_type", signalType, "batch_size", len(batch))
		// Create mock embedded IDs for testing
		mockIDs := make([]string, len(signalIDs))
		for i := range signalIDs {
			mockIDs[i] = fmt.Sprintf("mock_%d", i)
		}
		return w.DB.MarkSignalsEmbeddedAdvanced(ctx, signalType, signalIDs, w.ConfigID, mockIDs, w.Schema)
	}

	// Create documents
	var docs []schema.Document
	if grouped {
		// Single document for all signals
		var texts []string
		for _, sig := range batch {
			// Get the target vector store key from the signal with safe type assertion
			vectorStoreKey, ok := sig["target_vector_store"].(string)
			if !ok || vectorStoreKey == "" {
				w.Logger.Error("Missing or invalid target_vector_store in signal during grouped processing",
					"signal_keys", getMapKeys(sig),
					"target_vector_store_value", sig["target_vector_store"])
				continue
			}
			texts = append(texts, w.extractTextFromSignal(sig, signalType, vectorStoreKey))
		}

		// Use metadata from first signal as representative (most efficient)
		var combinedMetadata map[string]interface{}
		if len(batch) > 0 {
			// Safe access to target_vector_store for first signal
			firstVectorStoreKey, ok := batch[0]["target_vector_store"].(string)
			if !ok || firstVectorStoreKey == "" {
				w.Logger.Error("Missing or invalid target_vector_store in first signal during grouped processing",
					"signal_keys", getMapKeys(batch[0]),
					"target_vector_store_value", batch[0]["target_vector_store"])
				firstVectorStoreKey = "unknown"
			}
			firstMetadata := w.extractMetadataFromSignal(batch[0], signalType, firstVectorStoreKey)

			// Extract template metadata (excluding internal)
			combinedMetadata = make(map[string]interface{})
			for key, value := range firstMetadata {
				if key != "internal" {
					combinedMetadata[key] = value
				}
			}

			// Create internal metadata structure
			internalMetadata := map[string]interface{}{
				MetadataFieldGroupedIDs:  batchIDs,
				MetadataFieldSignalType:  signalType,
				MetadataFieldSignalCount: len(batch),
			}

			// Add internal fields from first signal
			if firstInternal, ok := firstMetadata["internal"].(map[string]interface{}); ok {
				for key, value := range firstInternal {
					if key != MetadataFieldID && key != MetadataFieldSignalType {
						internalMetadata[key] = value
					}
				}
			}

			combinedMetadata["internal"] = internalMetadata
		} else {
			combinedMetadata = map[string]interface{}{
				"internal": map[string]interface{}{
					MetadataFieldGroupedIDs:  batchIDs,
					MetadataFieldSignalType:  signalType,
					MetadataFieldSignalCount: 0,
				},
			}
		}

		docs = []schema.Document{{
			PageContent: strings.Join(texts, "\n"),
			Metadata:    combinedMetadata,
		}}
	} else {
		// Individual documents
		for _, sig := range batch {
			// Safe access to target_vector_store
			targetVectorStore, ok := sig["target_vector_store"].(string)
			if !ok || targetVectorStore == "" {
				w.Logger.Error("Missing or invalid target_vector_store in signal",
					"signal_keys", getMapKeys(sig),
					"target_vector_store_value", sig["target_vector_store"])
				continue // Skip this signal
			}

			// Extract metadata from signal based on template configuration
			metadata := w.extractMetadataFromSignal(sig, signalType, targetVectorStore)
			// Get the target vector store key from the signal
			vectorStoreKey := targetVectorStore
			docs = append(docs, schema.Document{
				PageContent: w.extractTextFromSignal(sig, signalType, vectorStoreKey),
				Metadata:    metadata,
			})
		}
	}

	// NEW: Process each vector store separately
	if w.VectorStoreConfigs != nil && len(w.VectorStoreConfigs) > 0 {
		// Group signals by target vector store
		signalsByVectorStore := make(map[string][]map[string]interface{})

		for _, signal := range batch {
			// Safe access to target_vector_store
			vsKey, ok := signal["target_vector_store"].(string)
			if !ok || vsKey == "" {
				w.Logger.Error("Missing or invalid target_vector_store in signal during grouping",
					"signal_keys", getMapKeys(signal),
					"target_vector_store_value", signal["target_vector_store"])
				continue // Skip this signal
			}
			signalsByVectorStore[vsKey] = append(signalsByVectorStore[vsKey], signal)
		}

		// Process each vector store separately
		for vsKey, signals := range signalsByVectorStore {
			vsConfig := w.VectorStoreConfigs[vsKey]
			if vsConfig == nil {
				// Get available config keys
				availableConfigs := make([]string, 0, len(w.VectorStoreConfigs))
				for k := range w.VectorStoreConfigs {
					availableConfigs = append(availableConfigs, k)
				}
				w.Logger.Error("Vector store config is nil",
					"vector_store_key", vsKey,
					"available_configs", availableConfigs)
				return fmt.Errorf("vector store config not found for %s", vsKey)
			}

			// Create documents for this vector store
			var docs []schema.Document
			if grouped {
				// Single document for all signals
				var texts []string
				for _, sig := range signals {
					// Get the target vector store key from the signal with safe type assertion
					vectorStoreKey, ok := sig["target_vector_store"].(string)
					if !ok || vectorStoreKey == "" {
						w.Logger.Error("Missing or invalid target_vector_store in signal during vector store processing",
							"signal_keys", getMapKeys(sig),
							"target_vector_store_value", sig["target_vector_store"])
						continue
					}
					texts = append(texts, w.extractTextFromSignal(sig, signalType, vectorStoreKey))
				}

				// Use metadata from first signal as representative, but make it unique per vector store
				var combinedMetadata map[string]interface{}
				if len(signals) > 0 {
					// Safe access to target_vector_store for first signal
					firstVectorStoreKey, ok := signals[0]["target_vector_store"].(string)
					if !ok || firstVectorStoreKey == "" {
						w.Logger.Error("Missing or invalid target_vector_store in first signal during vector store processing",
							"signal_keys", getMapKeys(signals[0]),
							"target_vector_store_value", signals[0]["target_vector_store"])
						firstVectorStoreKey = "unknown"
					}
					firstMetadata := w.extractMetadataFromSignal(signals[0], signalType, firstVectorStoreKey)

					// Extract template metadata (excluding internal)
					combinedMetadata = make(map[string]interface{})
					for key, value := range firstMetadata {
						if key != "internal" {
							combinedMetadata[key] = value
						}
					}

					// Create internal metadata structure
					internalMetadata := map[string]interface{}{
						MetadataFieldGroupedIDs:  batchIDs,
						MetadataFieldSignalType:  signalType,
						MetadataFieldSignalCount: len(signals),
						"vector_store_id":        vsConfig.ConfigID, // Make metadata unique per vector store
					}

					// Add internal fields from first signal
					if firstInternal, ok := firstMetadata["internal"].(map[string]interface{}); ok {
						for key, value := range firstInternal {
							if key != MetadataFieldID && key != MetadataFieldSignalType {
								internalMetadata[key] = value
							}
						}
					}

					combinedMetadata["internal"] = internalMetadata
				} else {
					combinedMetadata = map[string]interface{}{
						"internal": map[string]interface{}{
							MetadataFieldGroupedIDs:  batchIDs,
							MetadataFieldSignalType:  signalType,
							MetadataFieldSignalCount: 0,
							"vector_store_id":        vsConfig.ConfigID, // Make metadata unique per vector store
						},
					}
				}

				docs = []schema.Document{{
					PageContent: strings.Join(texts, "\n"),
					Metadata:    combinedMetadata,
				}}
			} else {
				// Individual documents with unique metadata per vector store
				for i, sig := range signals {
					metadata := w.extractMetadataFromSignal(sig, signalType, vsKey)

					// Add vector store specific internal fields
					if internal, ok := metadata["internal"].(map[string]interface{}); ok {
						internal["vector_store_id"] = vsConfig.ConfigID // Make metadata unique per vector store
						internal["document_index"] = i                  // Make each document unique within the vector store
					}

					docs = append(docs, schema.Document{
						PageContent: w.extractTextFromSignal(sig, signalType, vsKey),
						Metadata:    metadata,
					})
				}
			}

			// Add to this specific vector store
			embeddedIDs, err := vsConfig.VectorStore.AddDocuments(ctx, docs)
			if err != nil {
				w.Logger.Error("Failed to add documents to vector store",
					"vector_store", vsKey,
					"processor_id", vsConfig.ConfigID,
					"error", err)
				return fmt.Errorf("failed to add documents to vector store %s: %w", vsKey, err)
			}

			// Mark as embedded with this specific config ID
			signalIDs := w.extractSignalIDs(signals)
			err = w.DB.MarkSignalsEmbeddedAdvanced(ctx, signalType, signalIDs, vsConfig.ConfigID, embeddedIDs, w.Schema)
			if err != nil {
				w.Logger.Error("Failed to mark signals as embedded",
					"vector_store", vsKey,
					"processor_id", vsConfig.ConfigID,
					"error", err)
				return fmt.Errorf("failed to mark signals as embedded for vector store %s: %w", vsKey, err)
			}

			w.Logger.Debug("Successfully processed vector store",
				"vector_store", vsKey,
				"processor_id", vsConfig.ConfigID,
				"signal_count", len(signals),
				"embedded_count", len(embeddedIDs))
		}
	} else {
		// Fallback to original behavior
		// Add to vector store
		embeddedIDs, err := w.VectorStore.AddDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("failed to add documents to vector store: %w", err)
		}

		// Mark as embedded
		err = w.DB.MarkSignalsEmbeddedAdvanced(ctx, signalType, signalIDs, w.ConfigID, embeddedIDs, w.Schema)
		if err != nil {
			return fmt.Errorf("failed to mark signals as embedded: %w", err)
		}
	}

	// Process trace-level embeddings for qualifying root spans (only for template_plus mode)
	if w.ContentProcessingMode() == "template_plus" {
		if err := w.processTraceEmbeddings(ctx, batch); err != nil {
			w.Logger.Error("Failed to process trace embeddings", "error", err)
			// Don't fail the batch if trace embedding fails
		}
	}

	return nil
}

// extractTextFromSignal extracts the text content from a signal based on its type
func (w *EmbeddingWorker) extractTextFromSignal(sig map[string]interface{}, signalType string, vectorStoreKey string) string {
	// ✅ FIXED: Use complex processing for spans (fetch correlated data) but with pre-compiled template engine
	if signalType == "spans" {
		return w.processSpanWithCorrelatedData(sig, signalType, vectorStoreKey)
	}

	// Use normal template rendering for other signal types
	if content, err := w.renderTemplateContent(sig, signalType, vectorStoreKey); err == nil && content != "" {
		return content
	}

	return w.getFallbackText(sig)
}

// processSpanWithCorrelatedData processes a span with correlated data using pre-compiled template engine
func (w *EmbeddingWorker) processSpanWithCorrelatedData(sig map[string]interface{}, signalType string, vectorStoreKey string) string {
	vsConfig, exists := w.VectorStoreConfigs[vectorStoreKey]
	if !exists {
		return fmt.Sprintf("Vector store config not found for key %s", vectorStoreKey)
	}

	// Create transformer with pre-compiled template engine
	transformer := NewFullSpanTransformerWithPrecompiledEngine(vsConfig.ContentEngine)

	// Set the database client for the transformer to fetch correlated signals
	transformer.SetDatabaseClient(w.DB)

	// ✅ NEW: Set metadata fields configuration for template-based metadata extraction
	if metadataFields, found := vsConfig.TemplateMetadata["spans"]; found && len(metadataFields) > 0 {
		transformer.SetMetadataFields(metadataFields)
	}

	// Get exemplar types from template configuration
	exemplarTypes := w.getExemplarTypesFromTemplate()
	if len(exemplarTypes) > 0 {
		// Configure builder with specific exemplar types
		builder := &TraceContentBuilder{}
		builder.SetExemplarTypes(exemplarTypes)
	}

	// Build complete trace content with safe type assertions
	// Try span_id first, then fall back to id field
	spanID, ok := sig["span_id"].(string)
	if !ok {
		// Fall back to id field if span_id is not available
		if id, ok := sig["id"].(string); ok {
			spanID = id
		} else {
			spanID = "unknown"
		}
	}

	traceID, ok := sig["trace_id"].(string)
	if !ok {
		traceID = "unknown"
	}

	serviceName, ok := sig["service_name"].(string)
	if !ok {
		serviceName = "unknown"
	}

	// Get span name with safe type assertion
	spanName, ok := sig["name"].(string)
	if !ok {
		spanName = "unknown"
	}

	// Get span duration with safe type assertion
	spanDuration, ok := sig["duration"].(int64)
	if !ok {
		spanDuration = 0
	}

	// Get span status code with safe type assertion
	spanStatusCode, ok := sig["status_code"].(int)
	if !ok {
		spanStatusCode = 0
	}

	// Get parent span ID with safe type assertion
	var parentSpanID *string
	if parentID, ok := sig["parent_span_id"].(string); ok && parentID != "" {
		parentSpanID = &parentID
	}

	// Get span kind with safe type assertion
	var spanKind *string
	if kind, ok := sig["span_kind"].(string); ok && kind != "" {
		spanKind = &kind
	}

	// Get status message with safe type assertion
	var statusMessage *string
	if message, ok := sig["status_message"].(string); ok && message != "" {
		statusMessage = &message
	}

	// Get events with safe type assertion
	var events []map[string]interface{}
	if eventsData, ok := sig["events"].([]map[string]interface{}); ok {
		events = eventsData
	} else if eventsData, ok := sig["events"].([]interface{}); ok {
		// Convert []interface{} to []map[string]interface{}
		events = make([]map[string]interface{}, 0, len(eventsData))
		for _, event := range eventsData {
			if eventMap, ok := event.(map[string]interface{}); ok {
				events = append(events, eventMap)
			}
		}
	}

	span := SpanData{
		SpanID:        spanID,
		Name:          spanName,
		Duration:      spanDuration,
		StatusCode:    spanStatusCode,
		ParentSpanID:  parentSpanID,
		SpanKind:      spanKind,
		StatusMessage: statusMessage,
		Events:        events,
	}

	content := &CompleteTraceContent{
		TraceID:     traceID,
		ServiceName: serviceName,
		Spans:       []SpanData{span},
	}

	// Transform using pre-compiled template engine
	results, err := transformer.Transform(context.Background(), content)
	if err != nil {
		return fmt.Sprintf("Failed to transform span with correlated data: %v", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No transformation results for span %s", spanID)
	}

	return results[0].Content
}

// renderTemplateContent renders template content for a signal using the pre-compiled template engine
func (w *EmbeddingWorker) renderTemplateContent(sig map[string]interface{}, signalType string, vectorStoreKey string) (string, error) {
	vsConfig, exists := w.VectorStoreConfigs[vectorStoreKey]
	if !exists {
		return "", fmt.Errorf("vector store config not found for key %s", vectorStoreKey)
	}

	if vsConfig.ContentEngine == nil {
		return "", fmt.Errorf("pre-compiled content engine not found for vector store %s", vectorStoreKey)
	}

	// ✅ SIMPLIFIED: Use signal type directly for template lookup
	w.Logger.Debug("Template lookup",
		"signal_type", signalType,
		"vector_store", vectorStoreKey,
		"template_id", vsConfig.TemplateID,
		"available_content_templates", getStringMapKeys(vsConfig.TemplateContent))

	// Prepare template data
	templateData := w.prepareTemplateData(sig)

	// ✅ SIMPLE: Use signal type directly for template rendering
	content, err := vsConfig.ContentEngine.Render(signalType, templateData)
	if err != nil {
		return "", fmt.Errorf("template not found for signal type %s in vector store %s: %w", signalType, vectorStoreKey, err)
	}

	return content, nil
}

// prepareTemplateData prepares data for template rendering
func (w *EmbeddingWorker) prepareTemplateData(sig map[string]interface{}) map[string]interface{} {
	templateData := make(map[string]interface{})
	for key, value := range sig {
		templateData[key] = value
	}

	if _, exists := templateData["service_name"]; !exists {
		templateData["service_name"] = "unknown"
	}

	return templateData
}

// getFallbackText provides fallback text for signals without templates
func (w *EmbeddingWorker) getFallbackText(sig map[string]interface{}) string {
	if id, ok := sig["id"]; ok {
		return fmt.Sprintf("Signal %v", id)
	}
	return "Unknown signal"
}

// extractMetadataFromSignal extracts metadata fields from a signal using template metadata configuration
func (w *EmbeddingWorker) extractMetadataFromSignal(sig map[string]interface{}, signalType string, vectorStoreKey string) map[string]interface{} {
	vsConfig, exists := w.VectorStoreConfigs[vectorStoreKey]
	if !exists {
		return w.createBasicMetadata(sig, signalType)
	}

	// ✅ SIMPLIFIED: Use signal type directly for metadata lookup
	var metadataFields []string

	// Try to get metadata fields using signal type directly
	if fields, found := vsConfig.TemplateMetadata[signalType]; found && len(fields) > 0 {
		metadataFields = fields
	} else {
		// Fallback to basic metadata
		return w.createBasicMetadata(sig, signalType)
	}

	// Extract the specified fields (template-based metadata only)
	metadata := selectMetadataFields(sig, metadataFields)

	// Create internal metadata structure
	internalMetadata := map[string]interface{}{
		"signal_type": signalType,
	}
	if id, ok := sig["id"]; ok {
		internalMetadata["id"] = id
	}

	// Add internal metadata under "internal" key
	metadata["internal"] = internalMetadata

	return metadata
}

// Metadata field constants
const (
	MetadataFieldSignalType  = "signal_type"
	MetadataFieldID          = "id"
	MetadataFieldGroupedIDs  = "grouped_ids"
	MetadataFieldSignalCount = "signal_count"
)

// createBasicMetadata creates basic metadata for a signal
func (w *EmbeddingWorker) createBasicMetadata(sig map[string]interface{}, signalType string) map[string]interface{} {
	return map[string]interface{}{
		"internal": map[string]interface{}{
			MetadataFieldSignalType: signalType,
			MetadataFieldID:         sig["id"],
		},
	}
}

// chunkSignals splits signals into chunks using the unified processor registry
func (w *EmbeddingWorker) chunkSignals(signals []map[string]interface{}) ([][]map[string]interface{}, error) {
	if len(signals) == 0 {
		return nil, nil
	}

	// Convert to interface{} slice for the processor registry
	interfaceSignals := make([]interface{}, len(signals))
	for i, sig := range signals {
		interfaceSignals[i] = sig
	}

	// Simple chunking fallback - convert interface{} back to map[string]interface{}
	var convertedSignals []map[string]interface{}
	for _, sig := range interfaceSignals {
		if signal, ok := sig.(map[string]interface{}); ok {
			convertedSignals = append(convertedSignals, signal)
		}
	}
	result := [][]map[string]interface{}{convertedSignals}

	return result, nil
}

// estimateTokenCount provides a simple fallback token estimation
// This is used when the worker doesn't have a tokenizer configured
func estimateTokenCount(text string) int {
	return len(strings.Fields(text))
}

// countTokensWithTokenizer uses the worker's tokenizer for accurate token counting
func (w *EmbeddingWorker) countTokensWithTokenizer(text string) (int, error) {
	if w.tokenizer != nil {
		return w.tokenizer.CountTokens(text)
	}
	// Fallback to simple estimation
	return estimateTokenCount(text), nil
}

// getExemplarTypesFromTemplate extracts exemplar signal types from template configuration
func (w *EmbeddingWorker) getExemplarTypesFromTemplate() []string {
	var signalTypes []string

	// Check if template metadata contains exemplar configuration
	if w.Metadata != nil {
		if exemplarConfig, ok := w.Metadata["exemplar_types"]; ok {
			switch v := exemplarConfig.(type) {
			case []string:
				for _, t := range v {
					// ✅ SIMPLIFIED: Use signal types directly
					signalType := strings.TrimSpace(t)
					if signalType != "" {
						signalTypes = append(signalTypes, signalType)
					}
				}
			case string:
				// Handle comma-separated string
				types := strings.Split(v, ",")
				for _, t := range types {
					// ✅ SIMPLIFIED: Use signal types directly
					signalType := strings.TrimSpace(t)
					if signalType != "" {
						signalTypes = append(signalTypes, signalType)
					}
				}
			}
		}
	}

	return signalTypes
}

// processTraceEmbeddings processes trace-level embeddings for qualifying root spans
func (w *EmbeddingWorker) processTraceEmbeddings(ctx context.Context, batch []map[string]interface{}) error {
	// Only process for spans signal type
	if w.SignalType != "spans" {
		return nil
	}

	// Create trace embedding processor
	processor := NewTraceEmbeddingProcessor(w.DB, w.VectorStore, w.Logger, w.ConfigID, w.SignalType)

	// Process trace embeddings
	return processor.ProcessTraceEmbeddings(ctx, batch)
}
