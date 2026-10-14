package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

// ResourceType represents different types of resources that need circuit breaker protection
type ResourceType string

const (
	ResourceTypeDatabase    ResourceType = "database"
	ResourceTypeVectorStore ResourceType = "vectorstore"
	ResourceTypeEmbedding   ResourceType = "embedding"
	ResourceTypeWorker      ResourceType = "worker"
)

// Circuit breaker cleanup configuration
const (
	defaultCBCleanupInterval = 5 * time.Minute  // How often to run cleanup
	defaultCBIdleTTL         = 30 * time.Minute // Remove circuit breakers idle for this long
	maxCircuitBreakers       = 1000             // Maximum number of circuit breakers before forced cleanup
)

// ResourceCircuitBreakerManager manages circuit breakers for multiple resource types
type ResourceCircuitBreakerManager struct {
	circuitBreakers map[string]*ResourceCircuitBreaker
	mu              sync.RWMutex
	logger          util.Logger

	// Cleanup configuration
	cleanupInterval time.Duration
	idleTTL         time.Duration
	cleanupCancel   context.CancelFunc
	cleanupWg       sync.WaitGroup
}

// ResourceCircuitBreaker extends the base circuit breaker with resource-specific features
type ResourceCircuitBreaker struct {
	*util.CircuitBreaker
	resourceType     ResourceType
	resourceName     string
	resourceProvider string
	successCount     int64
	failureCount     int64
	lastSuccessTime  time.Time
	lastFailureTime  time.Time
	logger           util.Logger
	mu               sync.RWMutex
}

// NewResourceCircuitBreakerManager creates a new unified circuit breaker manager
func NewResourceCircuitBreakerManager(logger util.Logger) *ResourceCircuitBreakerManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ResourceCircuitBreakerManager{
		circuitBreakers: make(map[string]*ResourceCircuitBreaker),
		logger:          logger,
		cleanupInterval: defaultCBCleanupInterval,
		idleTTL:         defaultCBIdleTTL,
		cleanupCancel:   cancel,
	}

	// Start background cleanup goroutine
	m.cleanupWg.Add(1)
	go m.runCleanupLoop(ctx)

	return m
}

// NewResourceCircuitBreakerManagerWithConfig creates a manager with custom cleanup settings
func NewResourceCircuitBreakerManagerWithConfig(logger util.Logger, cleanupInterval, idleTTL time.Duration) *ResourceCircuitBreakerManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ResourceCircuitBreakerManager{
		circuitBreakers: make(map[string]*ResourceCircuitBreaker),
		logger:          logger,
		cleanupInterval: cleanupInterval,
		idleTTL:         idleTTL,
		cleanupCancel:   cancel,
	}

	// Start background cleanup goroutine
	m.cleanupWg.Add(1)
	go m.runCleanupLoop(ctx)

	return m
}

// runCleanupLoop periodically removes idle circuit breakers
func (m *ResourceCircuitBreakerManager) runCleanupLoop(ctx context.Context) {
	defer m.cleanupWg.Done()

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.cleanupIdleCircuitBreakers()
		}
	}
}

// cleanupIdleCircuitBreakers removes circuit breakers that have been idle too long
func (m *ResourceCircuitBreakerManager) cleanupIdleCircuitBreakers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	toDelete := make([]string, 0)

	for key, cb := range m.circuitBreakers {
		cb.mu.RLock()
		lastActivity := cb.lastSuccessTime
		if cb.lastFailureTime.After(lastActivity) {
			lastActivity = cb.lastFailureTime
		}
		cb.mu.RUnlock()

		// Remove if idle for too long
		if !lastActivity.IsZero() && now.Sub(lastActivity) > m.idleTTL {
			toDelete = append(toDelete, key)
		}
	}

	// Also perform forced cleanup if we have too many circuit breakers
	if len(m.circuitBreakers) > maxCircuitBreakers {
		m.logger.Info("Circuit breaker count exceeds maximum, performing forced cleanup",
			"current_count", len(m.circuitBreakers),
			"max_count", maxCircuitBreakers)
		// Sort by last activity and remove oldest 25%
		m.forceCleanupOldest(len(m.circuitBreakers) / 4)
	}

	for _, key := range toDelete {
		delete(m.circuitBreakers, key)
	}

	if len(toDelete) > 0 {
		m.logger.Info("Cleaned up idle circuit breakers",
			"removed_count", len(toDelete),
			"remaining_count", len(m.circuitBreakers))
	}
}

// forceCleanupOldest removes the oldest N circuit breakers (must be called with lock held)
func (m *ResourceCircuitBreakerManager) forceCleanupOldest(count int) {
	if count <= 0 || len(m.circuitBreakers) == 0 {
		return
	}

	type cbWithTime struct {
		key          string
		lastActivity time.Time
	}

	// Collect all circuit breakers with their last activity time
	items := make([]cbWithTime, 0, len(m.circuitBreakers))
	for key, cb := range m.circuitBreakers {
		cb.mu.RLock()
		lastActivity := cb.lastSuccessTime
		if cb.lastFailureTime.After(lastActivity) {
			lastActivity = cb.lastFailureTime
		}
		cb.mu.RUnlock()
		items = append(items, cbWithTime{key: key, lastActivity: lastActivity})
	}

	// Sort by last activity (oldest first)
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].lastActivity.Before(items[i].lastActivity) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	// Remove oldest entries
	removed := 0
	for i := 0; i < len(items) && removed < count; i++ {
		delete(m.circuitBreakers, items[i].key)
		removed++
	}
}

// GetOrCreateCircuitBreaker gets an existing circuit breaker or creates a new one
func (m *ResourceCircuitBreakerManager) GetOrCreateCircuitBreaker(
	resourceType ResourceType,
	resourceName, resourceProvider string,
) *ResourceCircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s_%s_%s", resourceType, resourceName, resourceProvider)

	if cb, exists := m.circuitBreakers[key]; exists {
		return cb
	}

	// Create new circuit breaker with resource-specific settings
	baseCB := m.getDefaultCircuitBreaker(resourceType)

	cb := &ResourceCircuitBreaker{
		CircuitBreaker:   baseCB,
		resourceType:     resourceType,
		resourceName:     resourceName,
		resourceProvider: resourceProvider,
		logger:           m.logger,
	}

	m.circuitBreakers[key] = cb

	m.logger.Info("Created circuit breaker for resource",
		"resource_type", resourceType,
		"resource_name", resourceName,
		"resource_provider", resourceProvider)

	return cb
}

// getDefaultCircuitBreaker returns appropriate circuit breaker settings for each resource type
func (m *ResourceCircuitBreakerManager) getDefaultCircuitBreaker(resourceType ResourceType) *util.CircuitBreaker {
	switch resourceType {
	case ResourceTypeDatabase:
		// Database: Conservative settings due to critical nature
		return util.NewCircuitBreakerWithConfig(3, 60*time.Second) // 3 failures, 60s timeout
	case ResourceTypeVectorStore:
		// Vector Store: Moderate settings for network operations
		return util.NewCircuitBreakerWithConfig(5, 30*time.Second) // 5 failures, 30s timeout
	case ResourceTypeEmbedding:
		// Embedding: Moderate settings for API calls
		return util.NewCircuitBreakerWithConfig(5, 45*time.Second) // 5 failures, 45s timeout
	case ResourceTypeWorker:
		// Worker: Liberal settings for processing operations
		return util.NewCircuitBreakerWithConfig(10, 15*time.Second) // 10 failures, 15s timeout
	default:
		// Default: Conservative settings
		return util.NewCircuitBreakerWithConfig(5, 30*time.Second)
	}
}

// ExecuteWithMetrics executes an operation with circuit breaker protection and enhanced metrics
func (cb *ResourceCircuitBreaker) ExecuteWithMetrics(operation func() error) error {
	startTime := time.Now()

	// Check if circuit breaker is open
	if cb.IsOpen() {
		cb.logger.Error("Circuit breaker is open for resource",
			"resource_type", cb.resourceType,
			"resource_name", cb.resourceName,
			"resource_provider", cb.resourceProvider,
			"failure_count", cb.GetFailureCount())

		return fmt.Errorf("circuit breaker is open for %s resource %s", cb.resourceType, cb.resourceName)
	}

	// Execute operation with circuit breaker protection
	err := cb.Execute(operation)

	// Update metrics based on result
	if err != nil {
		cb.mu.Lock()
		cb.failureCount++
		cb.lastFailureTime = time.Now()
		cb.mu.Unlock()

		cb.logger.Error("Resource operation failed",
			"resource_type", cb.resourceType,
			"resource_name", cb.resourceName,
			"resource_provider", cb.resourceProvider,
			"error", err,
			"failure_count", cb.failureCount,
			"circuit_breaker_state", cb.GetState())

	} else {
		cb.mu.Lock()
		cb.successCount++
		cb.lastSuccessTime = time.Now()
		cb.mu.Unlock()

		cb.logger.Info("Resource operation succeeded",
			"resource_type", cb.resourceType,
			"resource_name", cb.resourceName,
			"resource_provider", cb.resourceProvider,
			"duration", time.Since(startTime))
	}

	return err
}

// GetStats returns statistics for the circuit breaker
func (cb *ResourceCircuitBreaker) GetStats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	total := cb.successCount + cb.failureCount
	successRate := 0.0
	if total > 0 {
		successRate = float64(cb.successCount) / float64(total)
	}

	return map[string]interface{}{
		"resource_type":     cb.resourceType,
		"resource_name":     cb.resourceName,
		"resource_provider": cb.resourceProvider,
		"state":             cb.GetState(),
		"failure_count":     cb.failureCount,
		"success_count":     cb.successCount,
		"total_operations":  total,
		"success_rate":      successRate,
		"last_success_time": cb.lastSuccessTime,
		"last_failure_time": cb.lastFailureTime,
	}
}

// GetAllStats returns statistics for all circuit breakers
func (m *ResourceCircuitBreakerManager) GetAllStats() map[string]map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]map[string]interface{})
	for key, cb := range m.circuitBreakers {
		stats[key] = cb.GetStats()
	}

	return stats
}

// GetStatsByResourceType returns statistics for a specific resource type
func (m *ResourceCircuitBreakerManager) GetStatsByResourceType(resourceType ResourceType) map[string]map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]map[string]interface{})
	for key, cb := range m.circuitBreakers {
		if cb.resourceType == resourceType {
			stats[key] = cb.GetStats()
		}
	}

	return stats
}

// ResetCircuitBreaker resets a specific circuit breaker
func (m *ResourceCircuitBreakerManager) ResetCircuitBreaker(
	resourceType ResourceType,
	resourceName, resourceProvider string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s_%s_%s", resourceType, resourceName, resourceProvider)

	if _, exists := m.circuitBreakers[key]; exists {
		m.logger.Info("Reset circuit breaker for resource",
			"resource_type", resourceType,
			"resource_name", resourceName,
			"resource_provider", resourceProvider)

		return nil
	}

	return fmt.Errorf("circuit breaker not found for resource %s %s", resourceType, resourceName)
}

// Close cleans up the circuit breaker manager and stops the cleanup goroutine
func (m *ResourceCircuitBreakerManager) Close() {
	// Stop the cleanup goroutine
	if m.cleanupCancel != nil {
		m.cleanupCancel()
	}
	m.cleanupWg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.circuitBreakers = make(map[string]*ResourceCircuitBreaker)
	m.logger.Info("Closed resource circuit breaker manager")
}

// RemoveCircuitBreaker explicitly removes a circuit breaker (e.g., when a worker is shut down)
func (m *ResourceCircuitBreakerManager) RemoveCircuitBreaker(
	resourceType ResourceType,
	resourceName, resourceProvider string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s_%s_%s", resourceType, resourceName, resourceProvider)
	if _, exists := m.circuitBreakers[key]; exists {
		delete(m.circuitBreakers, key)
		m.logger.Info("Removed circuit breaker for resource",
			"resource_type", resourceType,
			"resource_name", resourceName,
			"resource_provider", resourceProvider)
	}
}

// GetCircuitBreakerCount returns the current number of circuit breakers
func (m *ResourceCircuitBreakerManager) GetCircuitBreakerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.circuitBreakers)
}

// HealthCheck returns health status for all resources
func (m *ResourceCircuitBreakerManager) HealthCheck() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	health := make(map[string]interface{})
	resourceTypeHealth := make(map[ResourceType]map[string]interface{})

	for key, cb := range m.circuitBreakers {
		stats := cb.GetStats()
		state := cb.GetState()

		// Initialize resource type map if needed
		if resourceTypeHealth[cb.resourceType] == nil {
			resourceTypeHealth[cb.resourceType] = make(map[string]interface{})
		}

		// Determine health status
		healthy := state == util.CircuitBreakerClosed
		successRate := stats["success_rate"].(float64)

		resourceTypeHealth[cb.resourceType][cb.resourceName] = map[string]interface{}{
			"healthy":      healthy,
			"state":        state,
			"success_rate": successRate,
			"key":          key,
		}
	}

	// Convert to final format
	for resourceType, resources := range resourceTypeHealth {
		health[string(resourceType)] = resources
	}

	return health
}
