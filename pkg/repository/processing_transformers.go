package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
	"github.com/medxops/trazr-otel/pkg/util"
)

// Constants for time formatting and thresholds
const (
	// Time conversion constants
	MicrosecondThreshold      = 1000000 // 1ms in nanoseconds
	NanosecondsPerMicrosecond = 1000
	NanosecondsPerMillisecond = 1000000

	// Template configuration constants
	DefaultTemplateKey     = "spans"
	MaxTemplateContentSize = 1024 * 1024 // 1MB max template size

	// Error message constants
	ErrNoTemplateContent    = "no template content provided"
	ErrTemplateValidation   = "template validation failed"
	ErrTemplateRegistration = "failed to register templates"
	ErrInvalidMetricType    = "invalid metric type"
	ErrInvalidDuration      = "invalid duration value"
)

// formatDuration formats duration in milliseconds by default, or microseconds if below 1ms
func formatDuration(nanoseconds int64) string {
	if nanoseconds < 1000000 {
		// Below 1ms, show in microseconds
		return fmt.Sprintf("%dµs", nanoseconds/1000)
	}
	// 1ms or above, show in milliseconds
	return fmt.Sprintf("%dms", nanoseconds/1000000)
}

// groupExemplarsByType groups exemplars by their metric type
func groupExemplarsByType(exemplars []ExemplarData) map[string][]ExemplarData {
	exemplarsByType := make(map[string][]ExemplarData)
	for _, exemplar := range exemplars {
		// Convert full signal type names to short names for template compatibility
		shortType := getShortMetricType(exemplar.MetricType)
		exemplarsByType[shortType] = append(exemplarsByType[shortType], exemplar)
	}
	return exemplarsByType
}

// getShortMetricType converts full signal type names to short names for template compatibility
func getShortMetricType(fullType string) string {
	switch fullType {
	case SignalTypeHistogramMetrics:
		return "histogram"
	case SignalTypeSummaryMetrics:
		return "summary"
	case SignalTypeGaugeMetrics:
		return "gauge"
	case SignalTypeSumMetrics:
		return "sum"
	case SignalTypeExponentialHistogramMetrics:
		return "exponential_histogram"
	default:
		return fullType // fallback to original if unknown
	}
}

// setExemplarTemplateData sets exemplar-related template data based on grouped exemplars
func setExemplarTemplateData(templateData map[string]interface{}, exemplarsByType map[string][]ExemplarData, correlated *CorrelatedSignals, builder *TraceContentBuilder) {
	// Set exemplar slices for template ranging (these are the actual slices for {{range .exemplars_gauge}} etc.)
	if len(exemplarsByType["histogram"]) > 0 {
		templateData["exemplars_histogram"] = exemplarsByType["histogram"]
	}
	if len(exemplarsByType["summary"]) > 0 {
		templateData["exemplars_summary"] = exemplarsByType["summary"]
	}
	if len(exemplarsByType["gauge"]) > 0 {
		templateData["exemplars_gauge"] = exemplarsByType["gauge"]
	}
	if len(exemplarsByType["sum"]) > 0 {
		templateData["exemplars_sum"] = exemplarsByType["sum"]
	}
	if len(exemplarsByType["exponential_histogram"]) > 0 {
		templateData["exemplars_exponential_histogram"] = exemplarsByType["exponential_histogram"]
	}

	// Set counts by metric type
	templateData["exemplar_count_histogram"] = len(exemplarsByType["histogram"])
	templateData["exemplar_count_summary"] = len(exemplarsByType["summary"])
	templateData["exemplar_count_gauge"] = len(exemplarsByType["gauge"])
	templateData["exemplar_count_sum"] = len(exemplarsByType["sum"])
	templateData["exemplar_count_exponential_histogram"] = len(exemplarsByType["exponential_histogram"])

	// Keep the combined exemplars for backward compatibility
	templateData["exemplars"] = correlated.Exemplars
	templateData["exemplar_count"] = len(correlated.Exemplars)

	// Create summaries by metric type (string summaries for display)
	if len(exemplarsByType["histogram"]) > 0 {
		templateData["exemplars_summary_histogram"] = builder.SummarizeExemplars(exemplarsByType["histogram"])
	}
	if len(exemplarsByType["summary"]) > 0 {
		templateData["exemplars_summary_summary"] = builder.SummarizeExemplars(exemplarsByType["summary"])
	}
	if len(exemplarsByType["gauge"]) > 0 {
		templateData["exemplars_summary_gauge"] = builder.SummarizeExemplars(exemplarsByType["gauge"])
	}
	if len(exemplarsByType["sum"]) > 0 {
		templateData["exemplars_summary_sum"] = builder.SummarizeExemplars(exemplarsByType["sum"])
	}
	if len(exemplarsByType["exponential_histogram"]) > 0 {
		templateData["exemplars_summary_exponential_histogram"] = builder.SummarizeExemplars(exemplarsByType["exponential_histogram"])
	}

	// Keep the combined summary for backward compatibility - only if there are exemplars
	if len(correlated.Exemplars) > 0 {
		templateData["exemplars_summary_text"] = builder.SummarizeExemplars(correlated.Exemplars)
	}
}

// clearExemplarTemplateData clears all exemplar-related template data
func clearExemplarTemplateData(templateData map[string]interface{}) {
	// Clear exemplar slices
	templateData["exemplars"] = nil
	templateData["exemplars_histogram"] = nil
	templateData["exemplars_summary"] = nil
	templateData["exemplars_gauge"] = nil
	templateData["exemplars_sum"] = nil
	templateData["exemplars_exponential_histogram"] = nil

	// Clear exemplar counts
	templateData["exemplar_count"] = 0
	templateData["exemplar_count_histogram"] = 0
	templateData["exemplar_count_summary"] = 0
	templateData["exemplar_count_gauge"] = 0
	templateData["exemplar_count_sum"] = 0
	templateData["exemplar_count_exponential_histogram"] = 0

	// Clear exemplar summaries
	templateData["exemplars_summary_histogram"] = nil
	templateData["exemplars_summary_summary"] = nil
	templateData["exemplars_summary_gauge"] = nil
	templateData["exemplars_summary_sum"] = nil
	templateData["exemplars_summary_exponential_histogram"] = nil
	templateData["exemplars_summary_text"] = nil
}

// Validation functions for input parameters
func validateSpanData(span SpanData) error {
	if span.SpanID == "" {
		return fmt.Errorf("span ID cannot be empty")
	}
	if span.Name == "" {
		return fmt.Errorf("span name cannot be empty")
	}
	if span.Duration < 0 {
		return fmt.Errorf("invalid duration: %d", span.Duration)
	}
	return nil
}

func validateCompleteTraceContent(content *CompleteTraceContent) error {
	if content == nil {
		return fmt.Errorf("complete trace content cannot be nil")
	}
	if content.TraceID == "" {
		return fmt.Errorf("trace ID cannot be empty")
	}
	if len(content.Spans) == 0 {
		return fmt.Errorf("trace must contain at least one span")
	}
	return nil
}

func validateCorrelatedSignals(correlated *CorrelatedSignals) error {
	if correlated == nil {
		return fmt.Errorf("correlated signals cannot be nil")
	}
	if correlated.SpanID == "" {
		return fmt.Errorf("correlated signals span ID cannot be empty")
	}
	return nil
}

// =========================
// Processing Transformers (Simplified)
// =========================

// ProcessingModeTransformer transforms complete trace content into different formats
type ProcessingModeTransformer interface {
	Transform(ctx context.Context, content *CompleteTraceContent) ([]ProcessedContent, error)
	GetMode() string
}

// ContextAwareTransformer extends ProcessingModeTransformer with context-aware methods
type ContextAwareTransformer interface {
	ProcessingModeTransformer
	ValidateWithContext(ctx context.Context, content *CompleteTraceContent) error
}

// ProcessedContent represents the result of transforming trace content
type ProcessedContent struct {
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
	Mode     string                 `json:"mode"`
}

// BaseTransformer provides common functionality for transformers
type BaseTransformer struct {
	config *TransformerConfig
}

// validateInputs validates all input parameters for template processing
func (b *BaseTransformer) validateInputs(content *CompleteTraceContent, span SpanData, correlated *CorrelatedSignals) error {
	if err := validateCompleteTraceContent(content); err != nil {
		return fmt.Errorf("invalid trace content: %w", err)
	}
	if err := validateSpanData(span); err != nil {
		return fmt.Errorf("invalid span data: %w", err)
	}
	if err := validateCorrelatedSignals(correlated); err != nil {
		return fmt.Errorf("invalid correlated signals: %w", err)
	}
	return nil
}

// getConfig returns the transformer configuration, creating default if not set
func (b *BaseTransformer) getConfig() *TransformerConfig {
	if b.config == nil {
		b.config = DefaultTransformerConfig()
	}
	return b.config
}

// FullSpanTransformer creates individual vectors for each span
type FullSpanTransformer struct {
	templateEngine   *content.ContentTemplateEngine
	logger           util.Logger
	templateAnalysis *TemplateAnalysis    // Cache template analysis
	builder          *TraceContentBuilder // Pre-configured builder
	metadataFields   []string             // ✅ NEW: Template metadata fields configuration
}

func NewFullSpanTransformer() *FullSpanTransformer {
	return &FullSpanTransformer{}
}

// ✅ NEW: NewFullSpanTransformerWithPrecompiledEngine creates transformer with pre-compiled engine
func NewFullSpanTransformerWithPrecompiledEngine(precompiledEngine *content.ContentTemplateEngine) *FullSpanTransformer {
	return &FullSpanTransformer{
		templateEngine: precompiledEngine,
		logger:         nil,
	}
}

// ✅ NEW: NewFullSpanTransformerWithPrecompiledEngineAndLogger creates transformer with pre-compiled engine and logger
func NewFullSpanTransformerWithPrecompiledEngineAndLogger(precompiledEngine *content.ContentTemplateEngine, logger util.Logger) *FullSpanTransformer {
	return &FullSpanTransformer{
		templateEngine: precompiledEngine,
		logger:         logger,
	}
}

// SetDatabaseClient configures the transformer with a database client for optimized querying
func (t *FullSpanTransformer) SetDatabaseClient(db OtelDatabaseClient) {
	t.builder = &TraceContentBuilder{db: db}

	// Configure exemplar types based on template analysis
	if t.templateAnalysis != nil && t.templateAnalysis.NeedsExemplars {
		var exemplarTypes []string
		for _, signalType := range t.templateAnalysis.ExemplarTypes {
			// ✅ SIMPLIFIED: Use signal types directly
			exemplarTypes = append(exemplarTypes, signalType)
		}
		t.builder.SetExemplarTypes(exemplarTypes)
	} else {
		// If no template analysis available, configure with default exemplar types
		// This ensures exemplars are fetched even without template analysis
		defaultExemplarTypes := []string{
			SignalTypeHistogramMetrics,
			SignalTypeSummaryMetrics,
			SignalTypeGaugeMetrics,
			SignalTypeSumMetrics,
			SignalTypeExponentialHistogramMetrics,
		}
		t.builder.SetExemplarTypes(defaultExemplarTypes)
	}
}

// ✅ NEW: SetMetadataFields sets the metadata fields configuration for template-based metadata extraction
func (t *FullSpanTransformer) SetMetadataFields(fields []string) {
	t.metadataFields = fields
}

// NewFullSpanTransformerWithLogger creates a new full span transformer with logger
func NewFullSpanTransformerWithLogger(logger util.Logger) *FullSpanTransformer {
	return &FullSpanTransformer{
		templateEngine: content.NewContentTemplateEngine(),
		logger:         logger,
	}
}

// NewFullSpanTransformerWithTemplateAndLogger creates a new full span transformer with custom template and logger
func NewFullSpanTransformerWithTemplateAndLogger(templateContent map[string]string, logger util.Logger) (*FullSpanTransformer, error) {
	// ✅ UPDATED: Use TemplateService approach instead of deleted CreateTemplateEngine
	// Create a template service to compile the template
	templateService, _ := NewTemplateServiceWithPreload(context.Background(), logger, nil, nil, nil)

	// Create a mock template for compilation
	template := &database.EmbeddingConfigTemplate{
		Content: templateContent,
	}

	// Compile the template using the centralized service
	templateInfo, err := templateService.GetOrCompileTemplate(0, template) // Use 0 as template ID for this case
	if err != nil {
		logger.Error("Failed to compile template", "error", err)
		return nil, err
	}

	return &FullSpanTransformer{
		templateEngine: templateInfo.ContentEngine,
		logger:         logger,
	}, nil
}

func (t *FullSpanTransformer) Transform(ctx context.Context, content *CompleteTraceContent) ([]ProcessedContent, error) {
	var results []ProcessedContent

	// Use pre-configured builder if available, otherwise create a new one
	builder := t.builder
	if builder == nil {
		builder = &TraceContentBuilder{}
	}

	for _, span := range content.Spans {
		// Check for context cancellation before processing each span
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during span processing: %w", ctx.Err())
		default:
		}

		// Get correlated signals based on template analysis (optimized database querying)
		var correlated *CorrelatedSignals
		if t.templateAnalysis != nil && t.builder != nil {
			// Use optimized querying - only fetch data that template needs
			correlated = builder.GetCorrelatedSignalsOptimized(ctx, content, span, t.templateAnalysis)
		} else {
			// Fallback to original method if no template analysis available
			correlated = builder.GetCorrelatedSignals(ctx, content, span)
		}

		// Prepare template data
		templateData := map[string]interface{}{
			"name":                span.Name,
			"span_id":             span.SpanID,
			"trace_id":            content.TraceID,
			"service_name":        content.ServiceName,
			"duration_ms":         span.Duration / 1000000,
			"status_code":         span.StatusCode,
			"log_count":           len(correlated.Logs),
			"exemplar_count":      len(correlated.Exemplars),
			"profile_count":       len(correlated.Profiles),
			"event_count":         len(span.Events),
			"events":              span.Events,
			"logs":                correlated.Logs,
			"exemplars":           correlated.Exemplars,
			"profiles":            correlated.Profiles,
			"attributes":          span.Attributes,
			"resource_attributes": span.ResourceAttributes,
			"metadata":            span.Metadata,
		}

		// Add exemplar data for template flexibility
		// Only set exemplars if exemplar types are configured
		exemplarTypes := builder.GetExemplarMetricTypes()

		if len(exemplarTypes) > 0 {
			exemplarsByType := groupExemplarsByType(correlated.Exemplars)
			setExemplarTemplateData(templateData, exemplarsByType, correlated, builder)
		} else {
			// Don't set exemplars if no types configured - template will skip exemplar logic
			clearExemplarTemplateData(templateData)
		}

		// Add metric type constants for template use
		templateData["SignalTypeHistogramMetrics"] = SignalTypeHistogramMetrics
		templateData["SignalTypeSummaryMetrics"] = SignalTypeSummaryMetrics
		templateData["SignalTypeGaugeMetrics"] = SignalTypeGaugeMetrics
		templateData["SignalTypeSumMetrics"] = SignalTypeSumMetrics

		// Add template-driven exemplar fetching capability
		templateData["builder"] = builder
		templateData["span_id"] = span.SpanID
		templateData["context"] = ctx

		// Set parent span ID
		if span.ParentSpanID != nil {
			templateData["parent_span_id"] = *span.ParentSpanID
		}

		// Set span kind
		if span.SpanKind != nil {
			templateData["span_kind"] = *span.SpanKind
		}

		// Set status message if available
		if span.StatusMessage != nil {
			templateData["status_message"] = *span.StatusMessage
		}

		// Add correlated signals summaries
		if len(correlated.Logs) > 0 {
			templateData["logs_summary"] = builder.SummarizeLogs(correlated.Logs)
		}

		if len(correlated.Profiles) > 0 {
			templateData["profiles_summary"] = builder.SummarizeProfiles(correlated.Profiles)
		}

		// Check for context cancellation before template rendering
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during template rendering: %w", ctx.Err())
		default:
		}

		// Render template
		var spanContent string
		if t.templateEngine != nil {

			rendered, err := t.templateEngine.Render("spans", templateData)
			if err != nil {
				// Fallback to hardcoded format if template rendering fails
				spanContent = t.renderFallbackFormat(content, span, correlated, builder)
			} else {
				spanContent = rendered
			}
		} else {
			// Fallback to hardcoded format if no template engine
			spanContent = t.renderFallbackFormat(content, span, correlated, builder)
		}

		// ✅ FIXED: Use template-based metadata extraction instead of hardcoded metadata
		// Convert span data to the format expected by template-based metadata extraction
		signalData := map[string]interface{}{
			"trace_id":            content.TraceID,
			"span_id":             span.SpanID,
			"service_name":        content.ServiceName,
			"duration":            span.Duration,
			"duration_ms":         span.Duration / NanosecondsPerMillisecond,
			"status_code":         span.StatusCode,
			"log_count":           len(correlated.Logs),
			"exemplar_count":      len(correlated.Exemplars),
			"profile_count":       len(correlated.Profiles),
			"event_count":         len(span.Events),
			"attributes":          span.Attributes,
			"resource_attributes": span.ResourceAttributes,
			"metadata":            span.Metadata,
			"name":                span.Name,
			"id":                  span.SpanID, // Use span_id as id for consistency
		}

		// Use template-based metadata extraction if available
		var metadata map[string]interface{}
		if t.metadataFields != nil && len(t.metadataFields) > 0 {
			// Extract metadata using template-based approach with configured fields
			// This follows the same pattern as extractMetadataFromSignal in embedding_worker_batch.go
			metadata = selectMetadataFields(signalData, t.metadataFields)

			// Create internal metadata structure
			internalMetadata := map[string]interface{}{
				"signal_type": "spans",
				"id":          span.SpanID,
			}

			// Add internal metadata under "internal" key
			metadata["internal"] = internalMetadata

		} else {
			// Fallback to basic metadata if no metadata fields configured
			metadata = map[string]interface{}{
				"internal": map[string]interface{}{
					"signal_type": "spans",
					"id":          span.SpanID,
					"trace_id":    content.TraceID,
					"span_id":     span.SpanID,
				},
			}
		}

		results = append(results, ProcessedContent{
			Content:  spanContent,
			Metadata: metadata,
			Mode:     t.GetMode(),
		})
	}

	return results, nil
}

// renderFallbackFormat provides the original hardcoded format as fallback
func (t *FullSpanTransformer) renderFallbackFormat(content *CompleteTraceContent, span SpanData, correlated *CorrelatedSignals, builder *TraceContentBuilder) string {
	var spanContent strings.Builder
	spanContent.WriteString(fmt.Sprintf("Span: %s (%s)\n", span.Name, span.SpanID))
	spanContent.WriteString(fmt.Sprintf("Trace: %s\n", content.TraceID))
	spanContent.WriteString(fmt.Sprintf("Service: %s\n", content.ServiceName))

	if span.ParentSpanID != nil {
		spanContent.WriteString(fmt.Sprintf("Parent: %s\n", *span.ParentSpanID))
	} else {
		spanContent.WriteString("Parent: ROOT\n")
	}

	spanContent.WriteString(fmt.Sprintf("Kind: %s\n", func() string {
		if span.SpanKind != nil {
			return *span.SpanKind
		}
		return "UNSPECIFIED"
	}()))

	spanContent.WriteString(fmt.Sprintf("Duration: %s\n", formatDuration(span.Duration)))
	spanContent.WriteString(fmt.Sprintf("Status: %d\n", span.StatusCode))

	if span.StatusMessage != nil {
		spanContent.WriteString(fmt.Sprintf("Message: %s\n", *span.StatusMessage))
	}

	// Add events summary
	if len(span.Events) > 0 {
		spanContent.WriteString(fmt.Sprintf("Events: %s\n", formatEventCountsByName(span.Events)))
	}

	// Add correlated signals
	if len(correlated.Logs) > 0 {
		spanContent.WriteString(fmt.Sprintf("Logs: %s\n", builder.SummarizeLogs(correlated.Logs)))
	}

	if len(correlated.Exemplars) > 0 {
		spanContent.WriteString(fmt.Sprintf("Exemplars: %s\n", builder.SummarizeExemplars(correlated.Exemplars)))
	}

	if len(correlated.Profiles) > 0 {
		spanContent.WriteString(fmt.Sprintf("Profiles: %s\n", builder.SummarizeProfiles(correlated.Profiles)))
	}

	return spanContent.String()
}

// formatEventCountsByName formats events by counting occurrences of each event name
func formatEventCountsByName(events []map[string]interface{}) string {
	if len(events) == 0 {
		return "0 events"
	}

	// Count events by name
	eventCounts := make(map[string]int)
	for _, event := range events {
		if name, ok := event["name"].(string); ok {
			eventCounts[name]++
		}
	}

	// Build summary
	var parts []string
	for name, count := range eventCounts {
		if count == 1 {
			parts = append(parts, name)
		} else {
			parts = append(parts, fmt.Sprintf("%d %s", count, name))
		}
	}

	return fmt.Sprintf("%d events (%s)", len(events), strings.Join(parts, ", "))
}

// GetMode returns the processing mode
func (t *FullSpanTransformer) GetMode() string {
	return "full_span"
}

type TransformerFactory struct {
	templateContent map[string]string
}

// NewTransformerFactory creates a new transformer factory
func NewTransformerFactory() *TransformerFactory {
	return &TransformerFactory{}
}

// NewTransformerFactoryWithTemplates creates a new transformer factory with database templates
func NewTransformerFactoryWithTemplates(templateContent map[string]string) *TransformerFactory {
	return &TransformerFactory{
		templateContent: templateContent,
	}
}

// CreateTransformer creates a transformer for the specified mode
func (f *TransformerFactory) CreateTransformer(mode string) (ProcessingModeTransformer, error) {
	// Template system is the primary processing mode
	// All processing now goes through the template system
	switch mode {
	case "template":
		if f.templateContent != nil {
			// ✅ UPDATED: Use TemplateService approach instead of deleted function
			templateService, _ := NewTemplateServiceWithPreload(context.Background(), nil, nil, nil, nil) // Use nil logger for this case
			template := &database.EmbeddingConfigTemplate{
				Content: f.templateContent,
			}
			templateInfo, err := templateService.GetOrCompileTemplate(0, template)
			if err != nil {
				return nil, fmt.Errorf("failed to compile template: %w", err)
			}
			return NewFullSpanTransformerWithPrecompiledEngine(templateInfo.ContentEngine), nil
		}
		return NewFullSpanTransformer(), nil
	case "template_plus":
		// template_plus mode does everything template mode does, plus trace-level embeddings
		// The trace embedding logic is handled separately in the batch processing
		if f.templateContent != nil {
			// ✅ UPDATED: Use TemplateService approach instead of deleted function
			templateService, _ := NewTemplateServiceWithPreload(context.Background(), nil, nil, nil, nil) // Use nil logger for this case
			template := &database.EmbeddingConfigTemplate{
				Content: f.templateContent,
			}
			templateInfo, err := templateService.GetOrCompileTemplate(0, template)
			if err != nil {
				return nil, fmt.Errorf("failed to compile template: %w", err)
			}
			return NewFullSpanTransformerWithPrecompiledEngine(templateInfo.ContentEngine), nil
		}
		return NewFullSpanTransformer(), nil
	default:
		// For any unknown mode, use template as default
		if f.templateContent != nil {
			// ✅ UPDATED: Use TemplateService approach instead of deleted function
			templateService, _ := NewTemplateServiceWithPreload(context.Background(), nil, nil, nil, nil) // Use nil logger for this case
			template := &database.EmbeddingConfigTemplate{
				Content: f.templateContent,
			}
			templateInfo, err := templateService.GetOrCompileTemplate(0, template)
			if err != nil {
				return nil, fmt.Errorf("failed to compile template: %w", err)
			}
			return NewFullSpanTransformerWithPrecompiledEngine(templateInfo.ContentEngine), nil
		}
		return NewFullSpanTransformer(), nil
	}
}

// TransformerConfig holds configuration for transformers
type TransformerConfig struct {
	// Template configuration
	DefaultTemplateKey     string
	MaxTemplateContentSize int
	EnableTemplateCaching  bool

	// Performance configuration
	PreAllocateMapCapacity int
	EstimatedSliceCapacity int

	// Error handling configuration
	EnableStrictValidation bool
	LogValidationErrors    bool

	// Time formatting configuration
	DurationUnit      string // "ms", "µs", "ns"
	DefaultTimeFormat string
}

// DefaultTransformerConfig returns the default configuration
func DefaultTransformerConfig() *TransformerConfig {
	return &TransformerConfig{
		DefaultTemplateKey:     "spans",
		MaxTemplateContentSize: MaxTemplateContentSize,
		EnableTemplateCaching:  true,
		PreAllocateMapCapacity: 10,
		EstimatedSliceCapacity: 5,
		EnableStrictValidation: false,
		LogValidationErrors:    true,
		DurationUnit:           "ms",
		DefaultTimeFormat:      "2006-01-02T15:04:05.000Z07:00",
	}
}
