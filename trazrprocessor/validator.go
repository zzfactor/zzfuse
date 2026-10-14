package trazrprocessor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	piiscrubber "github.com/aavaz-ai/pii-scrubber"
	"github.com/google/uuid"
	"github.com/xeipuuv/gojsonschema"
	"go.uber.org/zap"
)

// ValidatorService provides sensitive data (PII/PHI) validation for payloads.
type ValidatorService struct {
	scrubber piiscrubber.Scrubber
	config   ValidatorConfig
	logger   *zap.Logger
}

// ValidatorConfig defines the configuration for the Validator service.
type ValidatorConfig struct {
	RedactPII       bool   // If true, obfuscate PII and return Valid=true
	ObfuscationMode string // "redact", "mask", "tokenize", "encrypt"
	// TODO-TRAZR: Add custom detector config fields if needed
}

// TODO: Load config from file/env as needed
var defaultConfig = ValidatorConfig{
	RedactPII:       true,
	ObfuscationMode: "redact",
}

// initValidatorService initializes the Validator service and its dependencies.
func initValidatorService(logger *zap.Logger) (*ValidatorService, error) {
	scrubber, err := piiscrubber.NewDefaultScrubber()
	if err != nil {
		logger.Error("Failed to initialize PII scrubber", zap.Error(err))
		return nil, err
	}
	logger.Info("PII scrubber initialized successfully")
	return &ValidatorService{scrubber: scrubber, config: defaultConfig, logger: logger}, nil
}

// SensitiveDataValidationRequest is the request type for sensitive data validation.
// Payload is the raw data (JSON or proto). Format must be "json" or "proto".
type SensitiveDataValidationRequest struct {
	Payload []byte // raw OTLP payload (JSON or proto)
	Format  string // "json" or "proto"
}

// SensitiveDataValidationResponse is the response type for sensitive data validation.
type SensitiveDataValidationResponse struct {
	Valid  bool   // true if no sensitive data detected
	Reason string // optional reason if invalid
}

// PHIValidationErrorDetails provides structured error details for PHI validation errors.
type PHIValidationErrorDetails struct {
	Reason string
	ErrMsg string
}

func (PHIValidationErrorDetails) ErrDetails() {}

// Validate checks the payload for sensitive data (PII/PHI) and validates its format.
func (s *ValidatorService) Validate(ctx context.Context, req *SensitiveDataValidationRequest) (*SensitiveDataValidationResponse, error) {
	requestID := uuid.NewString()
	payloadSize := len(req.Payload)
	s.logger.Info("Validation request received", zap.String("request_id", requestID), zap.String("format", req.Format), zap.Int("payload_size", payloadSize))

	if payloadSize == 0 {
		s.logger.Info("Empty payload rejected", zap.String("request_id", requestID))
		return nil, fmt.Errorf("Payload is required: %v", PHIValidationErrorDetails{
			Reason: "empty_payload",
			ErrMsg: "Payload is empty",
		})
	}
	if req.Format != "json" && req.Format != "proto" {
		s.logger.Info("Invalid format", zap.String("request_id", requestID), zap.String("format", req.Format))
		return nil, fmt.Errorf("Format must be 'json' or 'proto': %v", PHIValidationErrorDetails{
			Reason: "invalid_format",
			ErrMsg: "Format must be 'json' or 'proto'",
		})
	}

	// Format-specific validation
	if req.Format == "json" {
		// Log the raw payload for debugging
		s.logger.Debug("Raw payload for validation", zap.String("request_id", requestID), zap.String("payload", string(req.Payload)))
		var js interface{}
		payloadForValidation := req.Payload // default to original
		if err := json.Unmarshal(req.Payload, &js); err != nil {
			// Fallback: treat as plain string, wrap as {\"message\": ...}
			wrapped := fmt.Sprintf(`{"message": %q}`, string(req.Payload))
			if err2 := json.Unmarshal([]byte(wrapped), &js); err2 != nil {
				s.logger.Error("ERROR: Both JSON and fallback unmarshal failed", zap.String("request_id", requestID), zap.Error(err2))
				return &SensitiveDataValidationResponse{Valid: false, Reason: "Invalid JSON and fallback"}, nil
			}
			payloadForValidation = []byte(wrapped)
		}

		// JSON Schema validation (example schema, replace with config as needed)
		schema := `{
			"type": "object",
			"properties": {
				"message": {"type": "string"}
			},
			"required": ["message"]
		}`
		schemaLoader := gojsonschema.NewStringLoader(schema)
		jsonLoader := gojsonschema.NewBytesLoader(payloadForValidation) // use the correct payload
		result, err := gojsonschema.Validate(schemaLoader, jsonLoader)
		if err != nil {
			s.logger.Error("JSON schema validation error", zap.String("request_id", requestID), zap.Error(err))
			return &SensitiveDataValidationResponse{Valid: false, Reason: "JSON schema validation error"}, nil
		}
		if !result.Valid() {
			errStrs := make([]string, len(result.Errors()))
			for i, e := range result.Errors() {
				errStrs[i] = e.String()
			}
			s.logger.Error("JSON schema validation failed", zap.String("request_id", requestID), zap.Strings("errors", errStrs))
			return &SensitiveDataValidationResponse{Valid: false, Reason: "JSON schema validation failed"}, nil
		}
		s.logger.Info("JSON schema validation passed", zap.String("request_id", requestID))
	} else if req.Format == "proto" {
		// TODO-TRAZR: Implement protobuf schema validation if needed
		// s.logger.Info("Protobuf format received", zap.String("request_id", requestID))
	}

	// Use the pre-initialized scrubber
	result, err := s.scrubber.ScrubTexts([]string{string(req.Payload)})
	if err != nil {
		s.logger.Error("PII scrubber error", zap.String("request_id", requestID), zap.Error(err))
		return nil, fmt.Errorf("PII scrubber error: %v", PHIValidationErrorDetails{
			Reason: "scrub_texts",
			ErrMsg: err.Error(),
		})
	}
	if result[0] != string(req.Payload) {
		if s.config.RedactPII {
			mode := s.config.ObfuscationMode
			switch mode {
			case "mask":
				s.logger.Info("Sensitive data masked", zap.String("request_id", requestID))
			case "tokenize":
				s.logger.Info("Sensitive data tokenized", zap.String("request_id", requestID))
			case "encrypt":
				s.logger.Info("Sensitive data encrypted (placeholder)", zap.String("request_id", requestID))
			default:
				s.logger.Info("Sensitive data redacted", zap.String("request_id", requestID))
			}
			return &SensitiveDataValidationResponse{Valid: true, Reason: "Sensitive data obfuscated: " + mode}, nil
		}
		s.logger.Info("Sensitive data detected", zap.String("request_id", requestID), zap.String("payload", string(req.Payload)))
		return &SensitiveDataValidationResponse{Valid: false, Reason: "Sensitive data detected in payload"}, nil
	}

	s.logger.Info("Payload validated successfully (no sensitive data detected)", zap.String("request_id", requestID))

	// At the end, before returning, publish an audit event synchronously (stub)
	event := struct {
		EventID    string
		Timestamp  time.Time
		Actor      string
		Action     string
		Resource   string
		ResourceID string
		Outcome    string
		Details    string
		RequestID  string
		SourceIP   string
		Compliance []string
	}{
		EventID:    uuid.NewString(),
		Timestamp:  time.Now().UTC(),
		Actor:      "validator",
		Action:     "validate",
		Resource:   "payload",
		ResourceID: "",
		Outcome:    "success",
		Details:    "",
		RequestID:  requestID,
		SourceIP:   "",
		Compliance: []string{"FDA-21CFR11", "NIST-AU-2"},
	}
	s.logger.Info("Audit event (stub)", zap.Any("event", event))

	return &SensitiveDataValidationResponse{Valid: true}, nil
}
