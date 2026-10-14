package repository

import (
	"fmt"
	"strings"
)

// =========================
// Unified Signal Processing (DRY Implementation)
// =========================

// SignalProcessor provides a unified interface for processing signals
type SignalProcessor interface {
	// ExtractText extracts the text content from a signal
	ExtractText(signal interface{}) (string, error)

	// Chunk chunks signals according to configuration
	Chunk(signals []interface{}, config ChunkingConfig) ([][]interface{}, error)

	// Validate validates a signal
	Validate(signal interface{}) error

	// GetSignalType returns the signal type this processor handles
	GetSignalType() SignalType
}

// BaseProcessor provides common functionality for all processors
type BaseProcessor struct {
	signalType SignalType
}

// GetSignalType returns the signal type
func (b *BaseProcessor) GetSignalType() SignalType {
	return b.signalType
}

// LogProcessor handles log signal processing
type LogProcessor struct {
	BaseProcessor
}

// NewLogProcessor creates a new log processor
func NewLogProcessor() *LogProcessor {
	return &LogProcessor{
		BaseProcessor: BaseProcessor{signalType: SignalTypeLogs},
	}
}

// ExtractText extracts text from a log signal
func (p *LogProcessor) ExtractText(signal interface{}) (string, error) {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid signal type for log processor")
	}

	// Try to extract from body field
	if body, ok := sig["body"]; ok {
		switch v := body.(type) {
		case string:
			return v, nil
		case []byte:
			return string(v), nil
		case map[string]interface{}:
			if msg, ok := v["message"].(string); ok {
				return msg, nil
			}
			if val, ok := v["value"].(string); ok {
				return val, nil
			}
		}
	}

	// Fallback: try to extract from attributes
	if attrs, ok := sig["attributes"].(map[string]interface{}); ok {
		if msg, ok := attrs["message"].(string); ok {
			return msg, nil
		}
	}

	// Final fallback
	if id, ok := sig["id"]; ok {
		return fmt.Sprintf("Log signal %v", id), nil
	}

	return "Unknown log signal", nil
}

// Chunk chunks log signals
func (p *LogProcessor) Chunk(signals []interface{}, config ChunkingConfig) ([][]interface{}, error) {
	var chunks [][]interface{}
	var currentChunk []interface{}

	for _, signal := range signals {
		text, err := p.ExtractText(signal)
		if err != nil {
			continue
		}

		// Check if adding this signal would exceed limits
		if len(currentChunk) > 0 && (len(currentChunk) >= config.MaxSpansPerChunk ||
			len(text) > config.MaxMessageLength) {
			chunks = append(chunks, currentChunk)
			currentChunk = []interface{}{}
		}

		currentChunk = append(currentChunk, signal)
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks, nil
}

// Validate validates a log signal
func (p *LogProcessor) Validate(signal interface{}) error {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid signal type")
	}

	if _, ok := sig["id"]; !ok {
		return fmt.Errorf("log signal missing required field: id")
	}

	return nil
}

// TraceSignalProcessor handles trace signal processing
type TraceSignalProcessor struct {
	BaseProcessor
}

// NewTraceSignalProcessor creates a new trace signal processor
func NewTraceSignalProcessor() *TraceSignalProcessor {
	return &TraceSignalProcessor{
		BaseProcessor: BaseProcessor{signalType: SignalTypeSpans},
	}
}

// ExtractText extracts text from a trace signal
func (p *TraceSignalProcessor) ExtractText(signal interface{}) (string, error) {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid signal type for trace processor")
	}

	var parts []string

	// Extract span name
	if name, ok := sig["name"].(string); ok {
		parts = append(parts, name)
	}

	// Extract from attributes
	if attrs, ok := sig["attributes"].(map[string]interface{}); ok {
		if msg, ok := attrs["message"].(string); ok {
			parts = append(parts, msg)
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, " - "), nil
	}

	// Fallback
	if id, ok := sig["id"]; ok {
		return fmt.Sprintf("Trace span %v", id), nil
	}

	return "Unknown trace signal", nil
}

// Chunk chunks trace signals
func (p *TraceSignalProcessor) Chunk(signals []interface{}, config ChunkingConfig) ([][]interface{}, error) {
	var chunks [][]interface{}
	var currentChunk []interface{}

	for _, signal := range signals {
		// Check if adding this signal would exceed limits
		if len(currentChunk) > 0 && len(currentChunk) >= config.MaxSpansPerChunk {
			chunks = append(chunks, currentChunk)
			currentChunk = []interface{}{}
		}

		currentChunk = append(currentChunk, signal)
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks, nil
}

// Validate validates a trace signal
func (p *TraceSignalProcessor) Validate(signal interface{}) error {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid signal type")
	}

	if _, ok := sig["id"]; !ok {
		return fmt.Errorf("trace signal missing required field: id")
	}

	if _, ok := sig["name"]; !ok {
		return fmt.Errorf("trace signal missing required field: name")
	}

	return nil
}

// MetricProcessor handles metric signal processing
type MetricProcessor struct {
	BaseProcessor
}

// NewMetricProcessor creates a new metric processor
func NewMetricProcessor() *MetricProcessor {
	return &MetricProcessor{
		BaseProcessor: BaseProcessor{signalType: SignalTypeMetrics},
	}
}

// ExtractText extracts text from a metric signal
func (p *MetricProcessor) ExtractText(signal interface{}) (string, error) {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid signal type for metric processor")
	}

	var parts []string

	// Extract metric name
	if name, ok := sig["name"].(string); ok {
		parts = append(parts, name)
	}

	// Extract metric value
	if value, ok := sig["value"]; ok {
		parts = append(parts, fmt.Sprintf("%v", value))
	}

	// Extract unit if available
	if unit, ok := sig["unit"].(string); ok && unit != "" {
		parts = append(parts, unit)
	}

	if len(parts) > 0 {
		return strings.Join(parts, " "), nil
	}

	// Fallback
	if id, ok := sig["id"]; ok {
		return fmt.Sprintf("Metric %v", id), nil
	}

	return "Unknown metric signal", nil
}

// Chunk chunks metric signals
func (p *MetricProcessor) Chunk(signals []interface{}, config ChunkingConfig) ([][]interface{}, error) {
	var chunks [][]interface{}
	var currentChunk []interface{}

	for _, signal := range signals {
		// Check if adding this signal would exceed limits
		if len(currentChunk) > 0 && len(currentChunk) >= config.MaxMetricsPerBatch {
			chunks = append(chunks, currentChunk)
			currentChunk = []interface{}{}
		}

		currentChunk = append(currentChunk, signal)
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks, nil
}

// Validate validates a metric signal
func (p *MetricProcessor) Validate(signal interface{}) error {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid signal type")
	}

	if _, ok := sig["id"]; !ok {
		return fmt.Errorf("metric signal missing required field: id")
	}

	if _, ok := sig["name"]; !ok {
		return fmt.Errorf("metric signal missing required field: name")
	}

	return nil
}

// ProfileProcessor handles profile signal processing
type ProfileProcessor struct {
	BaseProcessor
}

// NewProfileProcessor creates a new profile processor
func NewProfileProcessor() *ProfileProcessor {
	return &ProfileProcessor{
		BaseProcessor: BaseProcessor{signalType: SignalTypeProfiles},
	}
}

// ExtractText extracts text from a profile signal
func (p *ProfileProcessor) ExtractText(signal interface{}) (string, error) {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid signal type for profile processor")
	}

	// Extract profile type
	if profileType, ok := sig["profile_type"].(string); ok {
		return fmt.Sprintf("Profile: %s", profileType), nil
	}

	// Fallback
	if id, ok := sig["id"]; ok {
		return fmt.Sprintf("Profile %v", id), nil
	}

	return "Unknown profile signal", nil
}

// Chunk chunks profile signals
func (p *ProfileProcessor) Chunk(signals []interface{}, config ChunkingConfig) ([][]interface{}, error) {
	// Profiles are typically processed individually
	var chunks [][]interface{}
	for _, signal := range signals {
		chunks = append(chunks, []interface{}{signal})
	}
	return chunks, nil
}

// Validate validates a profile signal
func (p *ProfileProcessor) Validate(signal interface{}) error {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid signal type")
	}

	if _, ok := sig["id"]; !ok {
		return fmt.Errorf("profile signal missing required field: id")
	}

	return nil
}

// =========================
// Processor Registry (DRY Implementation)
// =========================

// ProcessorRegistry manages signal processors
type ProcessorRegistry struct {
	processors map[SignalType]SignalProcessor
}

// NewProcessorRegistry creates a new processor registry
func NewProcessorRegistry() *ProcessorRegistry {
	registry := &ProcessorRegistry{
		processors: make(map[SignalType]SignalProcessor),
	}

	// Register default processors
	registry.Register(NewLogProcessor())
	registry.Register(NewTraceSignalProcessor())
	registry.Register(NewProfileProcessor())

	// Register metric processor for all metric types
	metricProcessor := NewMetricProcessor()
	registry.Register(metricProcessor)
	registry.processors[SignalTypeGaugeMetrics] = metricProcessor
	registry.processors[SignalTypeSumMetrics] = metricProcessor
	registry.processors[SignalTypeHistogramMetrics] = metricProcessor
	registry.processors[SignalTypeSummaryMetrics] = metricProcessor
	registry.processors[SignalTypeExponentialHistogramMetrics] = metricProcessor

	return registry
}

// Register registers a signal processor
func (r *ProcessorRegistry) Register(processor SignalProcessor) {
	r.processors[processor.GetSignalType()] = processor
}

// GetProcessor returns a processor for the given signal type
func (r *ProcessorRegistry) GetProcessor(signalType SignalType) (SignalProcessor, error) {
	processor, exists := r.processors[signalType]
	if !exists {
		return nil, fmt.Errorf("no processor registered for signal type: %s", signalType)
	}
	return processor, nil
}

// GetProcessorForSignal returns a processor based on the signal content
func (r *ProcessorRegistry) GetProcessorForSignal(signal interface{}) (SignalProcessor, error) {
	sig, ok := signal.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid signal format")
	}

	// Try to determine signal type from the signal itself
	if signalType, ok := sig["signal_type"].(string); ok {
		return r.GetProcessor(SignalType(signalType))
	}

	// Fallback: try to determine from signal structure
	if _, ok := sig["body"]; ok {
		return r.GetProcessor(SignalTypeLogs)
	}
	if _, ok := sig["name"]; ok {
		if _, ok := sig["value"]; ok {
			// Check for specific metric type first
			if metricType, ok := sig["metric_type"].(string); ok {
				switch metricType {
				case "gauge":
					return r.GetProcessor(SignalTypeGaugeMetrics)
				case "sum":
					return r.GetProcessor(SignalTypeSumMetrics)
				case "histogram":
					return r.GetProcessor(SignalTypeHistogramMetrics)
				case "summary":
					return r.GetProcessor(SignalTypeSummaryMetrics)
				case "exponential_histogram":
					return r.GetProcessor(SignalTypeExponentialHistogramMetrics)
				}
			}
			// Default to gauge metrics
			return r.GetProcessor(SignalTypeGaugeMetrics)
		}
		return r.GetProcessor(SignalTypeSpans)
	}
	if _, ok := sig["profile_type"]; ok {
		return r.GetProcessor(SignalTypeProfiles)
	}

	return nil, fmt.Errorf("unable to determine signal type")
}

// ProcessSignals processes a batch of signals using the appropriate processor
func (r *ProcessorRegistry) ProcessSignals(signals []interface{}, config ChunkingConfig) ([][]interface{}, error) {
	if len(signals) == 0 {
		return [][]interface{}{}, nil
	}

	// Group signals by type
	signalsByType := make(map[SignalType][]interface{})

	for _, signal := range signals {
		processor, err := r.GetProcessorForSignal(signal)
		if err != nil {
			continue // Skip invalid signals
		}

		signalType := processor.GetSignalType()
		signalsByType[signalType] = append(signalsByType[signalType], signal)
	}

	// Process each group
	var allChunks [][]interface{}
	for signalType, typeSignals := range signalsByType {
		processor, err := r.GetProcessor(signalType)
		if err != nil {
			continue
		}

		chunks, err := processor.Chunk(typeSignals, config)
		if err != nil {
			continue
		}

		allChunks = append(allChunks, chunks...)
	}

	return allChunks, nil
}
