package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

func TestRetryWithBackoff(t *testing.T) {
	tests := []struct {
		name             string
		maxRetries       int
		errorPolicy      string
		shouldSucceed    bool
		expectedAttempts int
		failUntil        int // Number of attempts to fail before succeeding
	}{
		{
			name:             "success on first attempt",
			maxRetries:       3,
			errorPolicy:      "retry",
			shouldSucceed:    true,
			expectedAttempts: 1,
			failUntil:        0,
		},
		{
			name:             "success after retries",
			maxRetries:       3,
			errorPolicy:      "retry",
			shouldSucceed:    true,
			expectedAttempts: 2,
			failUntil:        1,
		},
		{
			name:             "fail with retry policy",
			maxRetries:       2,
			errorPolicy:      "retry",
			shouldSucceed:    false,
			expectedAttempts: 3,
			failUntil:        3,
		},
		{
			name:             "continue policy skips failed operation",
			maxRetries:       3,
			errorPolicy:      "continue",
			shouldSucceed:    true,
			expectedAttempts: 1,
			failUntil:        1,
		},
		{
			name:             "fail with stop policy",
			maxRetries:       3,
			errorPolicy:      "stop",
			shouldSucceed:    false,
			expectedAttempts: 1,
			failUntil:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := &EmbeddingWorker{
				BaseWorker: &BaseWorker{
					MaxRetries:       tt.maxRetries,
					BatchErrorPolicy: tt.errorPolicy,
					RetryBackoffBase: 10 * time.Millisecond,
					RetryBackoffMax:  100 * time.Millisecond,
					Logger:           util.NoopLogger{},
				},
			}

			attempts := 0

			err := worker.RetryWithBackoff(context.Background(), "test_operation", func() error {
				attempts++
				if attempts <= tt.failUntil {
					return MakeRetryable(fmt.Errorf("temporary error on attempt %d", attempts))
				}
				return nil
			})

			if tt.shouldSucceed && err != nil {
				t.Errorf("Expected success, got error: %v", err)
			}
			if !tt.shouldSucceed && err == nil {
				t.Errorf("Expected error, got success")
			}
			if attempts != tt.expectedAttempts {
				t.Errorf("Expected %d attempts, got %d", tt.expectedAttempts, attempts)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "retryable error",
			err:      &RetryableError{Err: errors.New("test")},
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      errors.New("timeout"),
			expected: true,
		},
		{
			name:     "rate limit error",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "permanent error",
			err:      errors.New("invalid input"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for error: %v", tt.expected, result, tt.err)
			}
		})
	}
}

func TestBatchProcessor(t *testing.T) {
	worker := &EmbeddingWorker{
		BaseWorker: &BaseWorker{
			MaxConcurrentBatches: 2,
			MaxRetries:           2,
			BatchErrorPolicy:     "retry",
			RetryBackoffBase:     10 * time.Millisecond,
			RetryBackoffMax:      50 * time.Millisecond,
			Logger:               util.NoopLogger{},
		},
	}

	processor := NewBatchProcessor(worker)

	// Test concurrent processing
	ctx := context.Background()
	items := []interface{}{1, 2, 3, 4, 5}

	err := processor.ProcessBatch(ctx, items, func(items []interface{}) error {
		// Simulate processing
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
}

func TestBatchErrorDetails(t *testing.T) {
	originalErr := errors.New("original error")
	batchErr := &BatchError{
		Operation:  "test_op",
		Attempt:    3,
		MaxRetries: 5,
		Err:        originalErr,
	}

	operation, attempt, maxRetries, err := GetBatchErrorDetails(batchErr)

	if operation != "test_op" {
		t.Errorf("Expected operation 'test_op', got '%s'", operation)
	}
	if attempt != 3 {
		t.Errorf("Expected attempt 3, got %d", attempt)
	}
	if maxRetries != 5 {
		t.Errorf("Expected maxRetries 5, got %d", maxRetries)
	}
	if err != originalErr {
		t.Errorf("Expected original error, got %v", err)
	}
}

func TestShouldRetryOperation(t *testing.T) {
	retryableErr := MakeRetryable(errors.New("retryable"))
	permanentErr := errors.New("permanent")

	tests := []struct {
		name     string
		err      error
		policy   string
		expected bool
	}{
		{"retry policy with retryable error", retryableErr, "retry", true},
		{"retry policy with permanent error", permanentErr, "retry", false},
		{"continue policy with retryable error", retryableErr, "continue", false},
		{"stop policy with retryable error", retryableErr, "stop", false},
		{"unknown policy defaults to retry", retryableErr, "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldRetryOperation(tt.err, tt.policy)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
