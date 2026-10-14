// Package repository provides OpenTelemetry signal storage with template processing capabilities.
//
// Template processing allows automatic metadata extraction and content generation
// for all signal types (logs, traces, metrics, profiles).
//
// Performance characteristics:
// - Template processing adds ~10-50ms per batch depending on template complexity
// - Memory overhead: ~2x for conversion operations when templates enabled
// - Thread-safe: Yes, when using separate processor instances
// - Can be disabled via SKIP_TEMPLATE_PROCESSING=true environment variable
package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/repository/template"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
)

// Constants for signal hash processing
const (
	DefaultProcessorID = 0
	DefaultHashID      = 0
	DefaultHashFlags   = 0
	DefaultServiceName = "unknown"
)

// TemplateProcessingError represents a structured error for template processing failures
type TemplateProcessingError struct {
	SignalType string
	EntryID    string
	Reason     string
	Cause      error
}

func (e *TemplateProcessingError) Error() string {
	return fmt.Sprintf("template processing failed for %s (entry: %s): %s", e.SignalType, e.EntryID, e.Reason)
}

func (e *TemplateProcessingError) Unwrap() error {
	return e.Cause
}

// TemplateProcessor provides unified template processing for all signal types
type TemplateProcessor struct {
	// Stateless - no mutable state
}

// NewTemplateProcessor creates a new template processor
func NewTemplateProcessor() *TemplateProcessor {
	return &TemplateProcessor{}
}

// getTemplateEngineFromService gets a template engine from the TemplateService
func getTemplateEngineFromService(templateID int64) (*content.ContentTemplateEngine, error) {
	// Try to get the global TemplateService instance
	globalTemplateServiceMutex.RLock()
	globalTS := globalTemplateService
	globalTemplateServiceMutex.RUnlock()

	if globalTS != nil {
		return globalTS.GetTemplateEngine(templateID)
	}

	// Fallback to the global function
	return GetTemplateEngine(templateID)
}

// SignalDataConverter converts any signal entry to template-compatible data
type SignalDataConverter interface {
	ToTemplateData() map[string]interface{}
	GetSignalType() string
	GetMetadata() []byte
	SetMetadata([]byte)
}

// ProcessTemplatesAndPopulateMetadata processes templates for any signal type.
// It processes each entry with configured templates and populates metadata fields.
// Returns processed entries, hashes, and an error if processing fails.
func (tp *TemplateProcessor) ProcessTemplatesAndPopulateMetadata(
	ctx context.Context,
	entries []SignalDataConverter,
	schemaName string,
	signalType string,
) ([]SignalDataConverter, []*database.SignalHash, error) {
	// Get processor configs from the template service
	processorConfigs, err := GetTemplates(schemaName, signalType, "pre_insert")
	if err != nil {
		return nil, nil, &TemplateProcessingError{
			SignalType: signalType,
			EntryID:    "config_fetch",
			Reason:     "failed to get processor configs",
			Cause:      err,
		}
	}

	// If no processor configs found, return original entries
	if len(processorConfigs) == 0 {
		return entries, nil, nil
	}

	// Create a copy of entries to avoid modifying originals
	processedEntries := make([]SignalDataConverter, len(entries))
	copy(processedEntries, entries)

	var hashes []*database.SignalHash

	// Process each entry with each processor config
	for i := range processedEntries {
		// Convert entry to template-compatible data structure
		signalData := processedEntries[i].ToTemplateData()

		// Process with each processor config
		for _, processorConfig := range processorConfigs {
			templateEngine, err := getTemplateEngineFromService(processorConfig.TemplateID)
			if err != nil {
				// Return error instead of silent failure
				return nil, nil, &TemplateProcessingError{
					SignalType: signalType,
					EntryID:    fmt.Sprintf("template_%d", processorConfig.TemplateID),
					Reason:     "failed to get template engine",
					Cause:      err,
				}
			}

			content, err := templateEngine.Render(signalType, signalData)
			if err != nil {
				// Return error instead of silent failure
				return nil, nil, &TemplateProcessingError{
					SignalType: signalType,
					EntryID:    fmt.Sprintf("template_%d", processorConfig.TemplateID),
					Reason:     "failed to render template",
					Cause:      err,
				}
			}

			metadata := tp.extractMetadataFromTemplate(signalData, processorConfig.TemplateID, signalType, schemaName)

			// Store template results in metadata (without hash)
			templateResult := map[string]interface{}{
				"content":     content,
				"metadata":    metadata,
				"template_id": processorConfig.TemplateID,
			}

			existingMetadata := make(map[string]interface{})
			if len(processedEntries[i].GetMetadata()) > 0 {
				if err := json.Unmarshal(processedEntries[i].GetMetadata(), &existingMetadata); err != nil {
					// Log error but continue
					fmt.Printf("WARN: Failed to unmarshal existing metadata: %v\n", err)
				}
			}

			if existing, exists := existingMetadata["templates"]; exists {
				if processingArray, ok := existing.([]interface{}); ok {
					existingMetadata["templates"] = append(processingArray, templateResult)
				} else {
					existingMetadata["templates"] = []interface{}{existing, templateResult}
				}
			} else {
				existingMetadata["templates"] = []interface{}{templateResult}
			}

			if metadataBytes, err := json.Marshal(existingMetadata); err == nil {
				processedEntries[i].SetMetadata(metadataBytes)
			} else {
				return nil, nil, &TemplateProcessingError{
					SignalType: signalType,
					EntryID:    fmt.Sprintf("entry_%d", i),
					Reason:     "failed to marshal metadata",
					Cause:      err,
				}
			}

			// Collect hash for signal_hashes table
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			signalTypeInt := tp.convertSignalTypeToInt16(signalType)
			runID := processorConfig.ID // Use processor ID as run ID

			// Extract timestamp from signal data - prioritize observed_timestamp
			timestampNs := time.Now().UnixNano() // fallback
			if observedTimestampVal, ok := signalData["observed_timestamp"].(*time.Time); ok && observedTimestampVal != nil {
				timestampNs = observedTimestampVal.UnixNano()
			} else if timestampVal, ok := signalData["timestamp_ns"].(int64); ok {
				timestampNs = timestampVal
			}

			hash := database.NewSignalHash(
				contentHash,
				runID,
				signalTypeInt,
				DefaultHashID, // ID will be filled after signal insertion
				database.SignalHashFlagNormal,
				timestampNs,
			)
			hashes = append(hashes, hash)
		}
	}

	// Template processing completed successfully
	return processedEntries, hashes, nil
}

// extractMetadataFromTemplate extracts metadata from template using database metadata fields
func (tp *TemplateProcessor) extractMetadataFromTemplate(
	signalData map[string]interface{},
	templateID int64,
	signalType string,
	schemaName string,
) map[string]interface{} {
	// Get template info from the global template service with processing mode
	templates, err := GetTemplatesForSchemaAndSignalType(context.Background(), schemaName, signalType, "pre_insert")
	if err != nil {
		// Return basic metadata as fallback
		return map[string]interface{}{
			"service_name": signalData["service_name"],
			"signal_type":  signalType,
		}
	}

	// Find the template with matching ID
	for _, templateInfo := range templates {
		if templateInfo.Template.ID == templateID {
			// Get metadata fields for the signal type
			metadataFields, ok := templateInfo.Template.TemplateMetadata[signalType]
			if !ok {
				break
			}

			// Extract metadata based on template's metadata fields
			metadata := make(map[string]interface{})
			for _, field := range metadataFields {
				if value, exists := signalData[field]; exists {
					metadata[field] = value
				}
			}

			return metadata
		}
	}

	// Fallback to basic metadata if template not found
	return map[string]interface{}{
		"service_name": signalData["service_name"],
		"signal_type":  signalType,
	}
}

// convertSignalTypeToInt16 converts signal type string to int16 constant
func (tp *TemplateProcessor) convertSignalTypeToInt16(signalType string) int16 {
	switch signalType {
	case "logs":
		return template.SignalTypeLog
	case "spans", "traces":
		return template.SignalTypeTrace
	case "gauge_metrics":
		return template.SignalTypeGaugeMetric
	case "sum_metrics":
		return template.SignalTypeSumMetric
	case "histogram_metrics":
		return template.SignalTypeHistogramMetric
	case "summary_metrics":
		return template.SignalTypeSummaryMetric
	case "exponential_histogram_metrics":
		return template.SignalTypeExponentialHistogramMetric
	case "counter_metrics":
		return template.SignalTypeCounterMetric
	case "profiles":
		return template.SignalTypeProfile
	default:
		return template.SignalTypeLog // Default fallback
	}
}
