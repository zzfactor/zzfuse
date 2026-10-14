package repository

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// BatchError represents an error that occurred during batch processing.
// It includes the operation, attempt count, max retries, and the original error.
type BatchError struct {
	Operation  string
	Attempt    int
	MaxRetries int
	Err        error
}

func (e *BatchError) Error() string {
	return fmt.Sprintf("batch operation '%s' failed after %d attempts (max: %d): %v",
		e.Operation, e.Attempt, e.MaxRetries, e.Err)
}

func (e *BatchError) Unwrap() error {
	return e.Err
}

// RetryableError marks an error as retryable for the retry logic.
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable error: %v", e.Err)
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryableError returns true if the error is considered retryable.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's explicitly marked as retryable
	if _, ok := err.(*RetryableError); ok {
		return true
	}

	// Check for common retryable error patterns
	errStr := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"rate limit",
		"too many requests",
		"service unavailable",
		"internal server error",
		"bad gateway",
		"gateway timeout",
	}

	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(len(s) == len(substr) ||
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsSubstringRetry(s, substr)))
}

func containsSubstringRetry(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxRetries  int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	ErrorPolicy string
	Jitter      bool
}

// RetryWithBackoff executes a function with exponential backoff retry logic.
// It respects the worker's error policy: 'retry' will retry, 'continue' logs and skips, 'stop' aborts immediately.
func (w *EmbeddingWorker) RetryWithBackoff(ctx context.Context, operation string, fn func() error) error {
	config := RetryConfig{
		MaxRetries:  w.MaxRetries,
		BaseDelay:   w.RetryBackoffBase,
		MaxDelay:    w.RetryBackoffMax,
		ErrorPolicy: w.BatchErrorPolicy,
		Jitter:      true,
	}
	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		err := fn()
		if err == nil {
			if attempt > 0 {
				w.LogRetrySuccess(operation, attempt)
			}
			return nil
		}
		lastErr = err
		w.LogRetryAttempt(operation, attempt+1, config.MaxRetries, err)
		if config.ErrorPolicy == "stop" {
			w.Logger.Error("BatchErrorPolicy=stop: aborting worker", "operation", operation, "error", err)
			return &BatchError{Operation: operation, Attempt: attempt + 1, MaxRetries: config.MaxRetries, Err: err}
		}
		if config.ErrorPolicy == "continue" {
			w.Logger.Error("BatchErrorPolicy=continue: skipping failed batch", "operation", operation, "error", err)
			return nil // Log and skip
		}
		if !IsRetryableError(err) || attempt == config.MaxRetries {
			return &BatchError{Operation: operation, Attempt: attempt + 1, MaxRetries: config.MaxRetries, Err: err}
		}
		delay := calculateBackoffDelay(attempt, config)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return &BatchError{Operation: operation, Attempt: config.MaxRetries + 1, MaxRetries: config.MaxRetries, Err: lastErr}
}

// RetryWithConfig executes a function with configurable retry logic
func RetryWithConfig(ctx context.Context, operation string, fn func() error, config RetryConfig) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the operation
		err := fn()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if we should retry based on error policy
		if !shouldRetry(err, config.ErrorPolicy) {
			return &BatchError{
				Operation:  operation,
				Attempt:    attempt + 1,
				MaxRetries: config.MaxRetries,
				Err:        err,
			}
		}

		// Don't sleep on the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate delay with exponential backoff
		delay := calculateBackoffDelay(attempt, config)

		// Wait with context cancellation support
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// Continue to next attempt
		}
	}

	return &BatchError{
		Operation:  operation,
		Attempt:    config.MaxRetries + 1,
		MaxRetries: config.MaxRetries,
		Err:        lastErr,
	}
}

// shouldRetry determines if an error should be retried based on the error policy
func shouldRetry(err error, policy string) bool {
	switch policy {
	case "continue":
		// Continue processing other items, don't retry
		return false
	case "stop":
		// Stop processing immediately, don't retry
		return false
	case "retry":
		// Retry if the error is retryable
		return IsRetryableError(err)
	default:
		// Default to retry behavior
		return IsRetryableError(err)
	}
}

// calculateBackoffDelay calculates the delay for exponential backoff
func calculateBackoffDelay(attempt int, config RetryConfig) time.Duration {
	// Exponential backoff: base * 2^attempt
	delay := float64(config.BaseDelay) * math.Pow(2, float64(attempt))

	// Add jitter to prevent thundering herd
	if config.Jitter {
		jitter := rand.Float64() * 0.1 * delay // 10% jitter
		delay += jitter
	}

	// Cap at maximum delay
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	return time.Duration(delay)
}

// BatchProcessor handles batch operations with retry and error handling
type BatchProcessor struct {
	worker *EmbeddingWorker
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(worker *EmbeddingWorker) *BatchProcessor {
	return &BatchProcessor{worker: worker}
}

// ProcessBatch processes a batch of items with retry logic
func (bp *BatchProcessor) ProcessBatch(ctx context.Context, items []interface{}, processor func([]interface{}) error) error {
	fmt.Println("ProcessBatch: starting (stdout)", bp.worker.SignalType, bp.worker.ConfigID, bp.worker.WorkerID)

	// Process with retry logic
	result := bp.worker.RetryWithBackoff(ctx, "batch_processing", func() error {
		return processor(items)
	})
	return result
}

// ProcessBatchWithFlush processes batches with flush interval control
func (bp *BatchProcessor) ProcessBatchWithFlush(ctx context.Context, items []interface{}, processor func([]interface{}) error) error {
	// Process the batch
	err := bp.ProcessBatch(ctx, items, processor)
	if err != nil {
		return err
	}

	// Apply flush interval if configured
	if bp.worker.FlushInterval > 0 {
		timer := time.NewTimer(bp.worker.FlushInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// Continue after flush interval
		}
	}

	return nil
}

// MakeRetryable wraps an error to mark it as retryable
func MakeRetryable(err error) error {
	return &RetryableError{Err: err}
}

// IsBatchError checks if an error is a BatchError
func IsBatchError(err error) bool {
	_, ok := err.(*BatchError)
	return ok
}

// GetBatchErrorDetails extracts details from a BatchError
func GetBatchErrorDetails(err error) (operation string, attempt int, maxRetries int, originalErr error) {
	if batchErr, ok := err.(*BatchError); ok {
		return batchErr.Operation, batchErr.Attempt, batchErr.MaxRetries, batchErr.Err
	}
	return "", 0, 0, err
}

// ShouldRetryOperation determines if an operation should be retried based on error type and policy
func ShouldRetryOperation(err error, policy string) bool {
	if err == nil {
		return false
	}

	switch policy {
	case "continue":
		return false
	case "stop":
		return false
	case "retry":
		return IsRetryableError(err)
	default:
		return IsRetryableError(err)
	}
}

// LogRetryAttempt logs retry attempt information
func (w *EmbeddingWorker) LogRetryAttempt(operation string, attempt int, maxRetries int, err error) {
	w.Logger.Debug("Retry attempt",
		"operation", operation,
		"attempt", attempt,
		"max_retries", maxRetries,
		"error", err.Error(),
		"signal_type", w.SignalType,
		"worker_id", w.WorkerID)
}

// LogRetrySuccess logs successful retry information
func (w *EmbeddingWorker) LogRetrySuccess(operation string, attempt int) {
	w.Logger.Debug("Retry successful",
		"operation", operation,
		"attempt", attempt,
		"signal_type", w.SignalType,
		"worker_id", w.WorkerID)
}
