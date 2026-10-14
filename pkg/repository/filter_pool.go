package repository

import (
	"sync"
)

// FilterPool provides a pool of reusable filter instances for high-frequency operations
type FilterPool struct {
	pool sync.Pool
}

// NewFilterPool creates a new filter pool
func NewFilterPool() *FilterPool {
	return &FilterPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &DatabaseSignalFilter{}
			},
		},
	}
}

// Get retrieves a filter from the pool
func (fp *FilterPool) Get() *DatabaseSignalFilter {
	return fp.pool.Get().(*DatabaseSignalFilter)
}

// Put returns a filter to the pool
func (fp *FilterPool) Put(filter *DatabaseSignalFilter) {
	// Reset the filter before putting it back
	if filter.FilterConfig != nil {
		filter.FilterConfig = nil
	}
	fp.pool.Put(filter)
}

// ParseAndGet parses a config and returns a filter from the pool
func (fp *FilterPool) ParseAndGet(config map[string]interface{}) (*DatabaseSignalFilter, error) {
	filter := fp.Get()

	// Parse the config
	filterConfig, err := ParseFilterConfig(config)
	if err != nil {
		fp.Put(filter) // Return to pool on error
		return nil, err
	}

	filter.FilterConfig = filterConfig
	return filter, nil
}

// Global filter pool for common use cases
var globalFilterPool = NewFilterPool()

// GetFilterFromPool retrieves a filter from the global pool
func GetFilterFromPool() *DatabaseSignalFilter {
	return globalFilterPool.Get()
}

// PutFilterToPool returns a filter to the global pool
func PutFilterToPool(filter *DatabaseSignalFilter) {
	globalFilterPool.Put(filter)
}

// ParseFilterFromPool parses a config and returns a filter from the global pool
func ParseFilterFromPool(config map[string]interface{}) (*DatabaseSignalFilter, error) {
	return globalFilterPool.ParseAndGet(config)
}
