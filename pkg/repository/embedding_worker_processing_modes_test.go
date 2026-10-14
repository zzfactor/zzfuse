package repository

import (
	"context"
	"testing"
)

func TestEmbeddingWorkerProcessingModes(t *testing.T) {
	// Test the template processing mode
	supportedModes := []string{
		"template",
	}

	factory := NewTransformerFactory()

	for _, mode := range supportedModes {
		t.Run(mode, func(t *testing.T) {
			// Test that we can create a transformer for this mode
			transformer, err := factory.CreateTransformer(mode)
			if err != nil {
				t.Errorf("Failed to create transformer for mode %s: %v", mode, err)
				return
			}

			// The template configuration resolves to the canonical full-span transformer.
			expectedMode := "full_span"
			if transformer.GetMode() != expectedMode {
				t.Errorf("Expected mode %s, got %s", expectedMode, transformer.GetMode())
			}

			// Test that the transformer can handle basic content
			traceContent := &CompleteTraceContent{
				TraceID:     "test-trace-123",
				ServiceName: "test-service",
				Spans: []SpanData{
					{
						SpanID:   "test-span-456",
						TraceID:  "test-trace-123",
						Name:     "test-operation",
						Duration: 1000000, // 1ms
					},
				},
				Logs:     []LogData{},
				Metrics:  []MetricData{},
				Profiles: []ProfileData{},
			}

			processedContents, err := transformer.Transform(context.Background(), traceContent)
			if err != nil {
				t.Errorf("Failed to transform content for mode %s: %v", mode, err)
				return
			}

			// Verify we got some content back
			if len(processedContents) == 0 {
				t.Errorf("No processed content returned for mode %s", mode)
				return
			}

			// Verify the content has the expected mode
			for i, content := range processedContents {
				if content.Mode != expectedMode {
					t.Errorf("Processed content %d has wrong mode: expected %s, got %s", i, expectedMode, content.Mode)
				}
				if content.Content == "" {
					t.Errorf("Processed content %d is empty for mode %s", i, mode)
				}
			}

			t.Logf("Mode %s: Generated %d processed contents", mode, len(processedContents))
		})
	}
}

func TestEmbeddingWorkerUnknownMode(t *testing.T) {
	factory := NewTransformerFactory()

	// Test that unknown modes fall back to default transformer (no error)
	transformer, err := factory.CreateTransformer("unknown_mode")
	if err != nil {
		t.Errorf("Expected no error for unknown mode, but got: %v", err)
		return
	}

	// Verify that we get a valid transformer (should be FullSpanTransformer as default)
	if transformer == nil {
		t.Error("Expected transformer for unknown mode, but got nil")
		return
	}

	// Verify the transformer works with basic content
	traceContent := &CompleteTraceContent{
		TraceID:     "test-trace-123",
		ServiceName: "test-service",
		Spans: []SpanData{
			{
				SpanID:   "test-span-456",
				TraceID:  "test-trace-123",
				Name:     "test-operation",
				Duration: 1000000, // 1ms
			},
		},
		Logs:     []LogData{},
		Metrics:  []MetricData{},
		Profiles: []ProfileData{},
	}

	processedContents, err := transformer.Transform(context.Background(), traceContent)
	if err != nil {
		t.Errorf("Failed to transform content for unknown mode: %v", err)
		return
	}

	// Verify we got some content back
	if len(processedContents) == 0 {
		t.Error("No processed content returned for unknown mode")
		return
	}

	t.Logf("Unknown mode: Generated %d processed contents with transformer type %T", len(processedContents), transformer)
}
