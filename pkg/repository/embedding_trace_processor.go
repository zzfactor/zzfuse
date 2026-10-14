package repository

import (
	"context"
	"fmt"
	"time"
)

// =========================
// Trace Processor (Simplified Integration)
// =========================

// TraceProcessor provides a simple interface for processing complete spans
type TraceProcessor struct {
	db      OtelDatabaseClient
	builder *TraceContentBuilder
	factory *TransformerFactory
}

// NewTraceProcessor creates a new trace processor
func NewTraceProcessor(db OtelDatabaseClient) *TraceProcessor {
	return &TraceProcessor{
		db:      db,
		builder: NewTraceContentBuilder(db),
		factory: NewTransformerFactory(),
	}
}

// ProcessTrace processes a complete trace and returns the processed content
func (p *TraceProcessor) ProcessTrace(ctx context.Context, traceID string, mode string) ([]ProcessedContent, error) {
	// 1. Build complete trace content
	content, err := p.builder.BuildCompleteContent(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to build trace content: %w", err)
	}

	// 2. Create transformer for the specified mode
	transformer, err := p.factory.CreateTransformer(mode)
	if err != nil {
		return nil, fmt.Errorf("failed to create transformer: %w", err)
	}

	// 3. Transform the content
	processed, err := transformer.Transform(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("failed to transform content: %w", err)
	}

	return processed, nil
}

// ProcessSpanAndStore processes a single span and stores the content in the database
// This is used for testing to understand what data we're collecting
func (p *TraceProcessor) ProcessSpanAndStore(ctx context.Context, traceID, spanID, mode string) error {
	// 1. Get the span data
	spans, err := p.db.FetchSpansByTraceID(ctx, traceID)
	if err != nil {
		return fmt.Errorf("failed to fetch spans for trace %s: %w", traceID, err)
	}

	// Find the specific span
	var targetSpan *SpanData
	for i := range spans {
		if spans[i].SpanID == spanID {
			targetSpan = &spans[i]
			break
		}
	}

	if targetSpan == nil {
		return fmt.Errorf("span %s not found in trace %s", spanID, traceID)
	}

	// 2. Build complete trace content (we need all signals for correlation)
	content, err := p.builder.BuildCompleteContent(ctx, traceID)
	if err != nil {
		return fmt.Errorf("failed to build trace content: %w", err)
	}

	// 3. Create transformer for the specified mode
	transformer, err := p.factory.CreateTransformer(mode)
	if err != nil {
		return fmt.Errorf("failed to create transformer: %w", err)
	}

	// 4. Transform the content
	processed, err := transformer.Transform(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to transform content: %w", err)
	}

	// 5. Find the processed content for this specific span
	var spanContent *ProcessedContent
	for _, pc := range processed {
		if metadata, ok := pc.Metadata["span_id"].(string); ok && metadata == spanID {
			spanContent = &pc
			break
		}
	}

	if spanContent == nil {
		// If no specific span content found, use the first one (for trace-level modes)
		if len(processed) > 0 {
			spanContent = &processed[0]
		} else {
			return fmt.Errorf("no processed content generated for span %s", spanID)
		}
	}

	// Note: Processed content storage has been removed for batch optimization
	// The content is processed but not stored in the database
	return nil
}

// ProcessTraceWithEmbedding processes a trace and sends the result to the embedding worker
func (p *TraceProcessor) ProcessTraceWithEmbedding(ctx context.Context, traceID string, mode string, worker *EmbeddingWorker) error {
	// 1. Process the trace
	processed, err := p.ProcessTrace(ctx, traceID, mode)
	if err != nil {
		return fmt.Errorf("failed to process trace: %w", err)
	}

	// 2. Convert processed content to signals format for the embedding worker
	for _, content := range processed {
		// Create a signal map that the embedding worker expects
		signal := map[string]interface{}{
			"id":              fmt.Sprintf("trace_%s_%s", traceID, mode),
			"content":         content.Content,
			"processing_mode": mode,
			"trace_id":        traceID,
		}

		// Add trace-specific metadata
		if traceID, ok := content.Metadata["trace_id"].(string); ok {
			signal["trace_id"] = traceID
		}
		if spanID, ok := content.Metadata["span_id"].(string); ok {
			signal["span_id"] = spanID
		}
		if serviceName, ok := content.Metadata["service_name"].(string); ok {
			signal["service_name"] = serviceName
		}
		if durationMs, ok := content.Metadata["duration_ms"].(int64); ok {
			signal["duration_ms"] = durationMs
		}

		// Create a batch with this single signal
		batch := []map[string]interface{}{signal}

		// Use the embedding worker's embedBatch method
		err := worker.embedBatch(ctx, batch, "spans", false)
		if err != nil {
			return fmt.Errorf("failed to embed trace content: %w", err)
		}
	}

	return nil
}

// GetAvailableModes returns the list of available processing modes
func (p *TraceProcessor) GetAvailableModes() []string {
	return []string{
		"full_trace",
		"full_span",
		"full_trace_signals",
		"full_span_signals",
	}
}

// GetTraceSummary gets a summary of a trace without full processing
func (p *TraceProcessor) GetTraceSummary(ctx context.Context, traceID string) (*CompleteTraceContent, error) {
	return p.builder.BuildCompleteContent(ctx, traceID)
}

// buildTraceContent builds a complete trace content structure from a span signal
func (w *EmbeddingWorker) buildTraceContent(ctx context.Context, sig map[string]interface{}) *CompleteTraceContent {
	traceID := getStringValue(sig, "trace_id")
	spanID := getStringValue(sig, "span_id")

	// Create span data from signal (used by both TraceContentBuilder and fallback)
	span := w.createSpanFromSignal(sig, spanID, traceID)

	// Use TraceContentBuilder to build complete content from span
	// This handles log fetching by span ID and other signal correlation
	if traceDB := w.getTraceDatabaseClient(); traceDB != nil {
		builder := NewTraceContentBuilder(traceDB)

		// Configure exemplar types based on template configuration
		// This allows templates to specify which metric types to query
		if exemplarTypes := w.getExemplarTypesFromTemplate(); len(exemplarTypes) > 0 {
			builder.SetExemplarMetricTypes(exemplarTypes)
		}

		if content, err := builder.BuildCompleteContentFromSpan(ctx, span); err == nil {
			// Also check if the current signal itself is a log and add it
			if logFromSignal := w.extractLogFromSignal(sig); logFromSignal != nil {
				content.Logs = append(content.Logs, *logFromSignal)
				w.Logger.Info("Added log from current signal to TraceContentBuilder result",
					"log_id", logFromSignal.ID,
					"total_logs", len(content.Logs))
			}
			content.Metadata["processing_mode"] = w.ContentProcessingMode()
			content.Metadata["worker_id"] = w.WorkerID
			return content
		}
	}

	// Fallback to basic implementation without database queries
	// This is used when no database client is available
	var logs []LogData
	if logFromSignal := w.extractLogFromSignal(sig); logFromSignal != nil {
		logs = append(logs, *logFromSignal)
		w.Logger.Info("Added log from current signal in fallback mode",
			"log_id", logFromSignal.ID,
			"total_logs", len(logs))
	}

	return &CompleteTraceContent{
		TraceID:     traceID,
		ServiceName: getStringValue(sig, "service_name"),
		Spans:       []SpanData{span},
		Logs:        logs,
		Metrics:     []MetricData{},
		Profiles:    []ProfileData{},
		Metadata: map[string]interface{}{
			"processing_mode": w.ContentProcessingMode(),
			"worker_id":       w.WorkerID,
		},
	}
}

// createSpanFromSignal creates a SpanData struct from a signal map
func (w *EmbeddingWorker) createSpanFromSignal(sig map[string]interface{}, spanID, traceID string) SpanData {
	return SpanData{
		SpanID:             spanID,
		TraceID:            traceID,
		Name:               getStringValue(sig, "name"),
		ServiceName:        getStringPtr(sig, "service_name"),
		Attributes:         w.extractAttributes(sig, "attributes"),
		ResourceAttributes: w.extractAttributes(sig, "resource_attributes"),
		ScopeAttributes:    w.extractAttributes(sig, "scope_attributes"),
		StatusCode:         getIntValue(sig, "status_code"),
		Duration:           getInt64Value(sig, "duration"),
		Events:             w.extractEvents(sig),
	}
}

// extractAttributes safely extracts attributes from a signal
func (w *EmbeddingWorker) extractAttributes(sig map[string]interface{}, key string) map[string]interface{} {
	if attrs, ok := sig[key]; ok {
		if attrMap, ok := attrs.(map[string]interface{}); ok {
			return attrMap
		}
	}
	return make(map[string]interface{})
}

// extractEvents safely extracts events from a signal
func (w *EmbeddingWorker) extractEvents(sig map[string]interface{}) []map[string]interface{} {
	if events, ok := sig["events"]; ok {
		if eventsSlice, ok := events.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(eventsSlice))
			for _, event := range eventsSlice {
				if eventMap, ok := event.(map[string]interface{}); ok {
					result = append(result, eventMap)
				}
			}
			return result
		}
		// Handle case where events is already []map[string]interface{}
		if eventsSlice, ok := events.([]map[string]interface{}); ok {
			return eventsSlice
		}
	}
	return make([]map[string]interface{}, 0)
}

// extractLogFromSignal safely extracts log data if the signal itself is a log
func (w *EmbeddingWorker) extractLogFromSignal(sig map[string]interface{}) *LogData {
	// Check if this signal is a log signal
	if signalType, ok := sig["signal_type"]; ok {
		if signalTypeStr, ok := signalType.(string); ok && signalTypeStr == "logs" {
			// This is a log signal, extract it as LogData
			logData := &LogData{
				ID:        getStringValue(sig, "id"),
				TraceID:   getStringPtr(sig, "trace_id"),
				SpanID:    getStringPtr(sig, "span_id"),
				Body:      getStringValue(sig, "body"),
				Timestamp: time.Now(), // Default timestamp
			}

			// Extract severity if available
			if severityText, ok := sig["severity_text"]; ok {
				if severityStr, ok := severityText.(string); ok {
					logData.SeverityText = &severityStr
				}
			}

			// Extract attributes if available
			if attrs, ok := sig["attributes"]; ok {
				if attrMap, ok := attrs.(map[string]interface{}); ok {
					logData.Attributes = attrMap
				}
			}

			return logData
		}
	}
	return nil
}

// getTraceDatabaseClient safely gets an OtelDatabaseClient if available
func (w *EmbeddingWorker) getTraceDatabaseClient() OtelDatabaseClient {
	if w.BaseWorker != nil && w.BaseWorker.DB != nil {
		if traceDB, ok := w.BaseWorker.DB.(OtelDatabaseClient); ok {
			return traceDB
		}
	}
	return nil
}
