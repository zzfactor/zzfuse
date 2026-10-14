package content

import (
	"sync"
	"time"
)

// SimpleLRUCache is a minimal LRU cache for configs/templates
// Not thread-safe for concurrent writes; wrap with mutex if needed
// (for demo purposes, not production-grade)
type SimpleLRUCache struct {
	data   map[interface{}]cacheEntry
	access []interface{}
	size   int
	mu     sync.Mutex
}

type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

func NewSimpleLRUCache(size int) *SimpleLRUCache {
	return &SimpleLRUCache{
		data:   make(map[interface{}]cacheEntry),
		access: make([]interface{}, 0, size),
		size:   size,
	}
}

func (c *SimpleLRUCache) Get(key interface{}) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.data[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	// Move key to end (most recently used)
	c.moveToEnd(key)
	return entry.value, true
}

func (c *SimpleLRUCache) Set(key, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.data) >= c.size {
		// Remove least recently used
		oldest := c.access[0]
		c.access = c.access[1:]
		delete(c.data, oldest)
	}
	c.data[key] = cacheEntry{value: value, expiresAt: time.Now().Add(ttl)}
	c.moveToEnd(key)
}

func (c *SimpleLRUCache) moveToEnd(key interface{}) {
	// Remove key if present
	for i, k := range c.access {
		if k == key {
			c.access = append(c.access[:i], c.access[i+1:]...)
			break
		}
	}
	c.access = append(c.access, key)
}
