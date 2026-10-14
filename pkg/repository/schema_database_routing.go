package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"go.uber.org/zap"
)

// cacheEntry represents a cached lookup result
type cacheEntry struct {
	serviceName      string
	serviceNamespace string
	schema           string
	found            bool
}

// cacheKey generates a unique key for cache lookup
func cacheKey(serviceName, serviceNamespace string) string {
	return serviceName + "\x00" + serviceNamespace
}

// DatabaseRoutingSchema provides high-performance schema routing based on preprocessed database rules
type DatabaseRoutingSchema struct {
	mu              sync.RWMutex
	routingIdx      *routingIndex
	schemaConfigs   []*database.DatabaseSchemaConfig
	schemaSingleton string
	logger          *zap.Logger
	configRepo      database.SchemaConfigInterface

	// Thread-safe cache using sync.Map for atomic operations
	// Key: "serviceName\x00serviceNamespace", Value: *cacheEntry
	cacheMap sync.Map
}

// Preprocessed routing lookup structures for performance
type routingIndex struct {
	// Direct exact matches - O(1) lookup
	serviceNameExact      map[string]string // serviceName -> schema
	serviceNamespaceExact map[string]string // serviceNamespace -> schema

	// Pattern matches - precompiled for faster matching
	serviceNamePatterns      []patternRule
	serviceNamespacePatterns []patternRule

	// Fallback schemas
	catchAllSchema string // First catch-all schema found
	wildcardSchema string // First wildcard schema found
	hasCatchAll    bool
	hasWildcard    bool
}

type patternRule struct {
	schema      string
	prefix      string
	suffix      string
	contains    string
	hasPrefix   bool
	hasSuffix   bool
	hasContains bool
}

// NewDatabaseRoutingSchema creates a new database routing schema manager
func NewDatabaseRoutingSchema(configRepo database.SchemaConfigInterface, logger *zap.Logger) *DatabaseRoutingSchema {
	return &DatabaseRoutingSchema{
		configRepo: configRepo,
		logger:     logger,
	}
}

// NewDatabaseRoutingSchemWithUtilLogger creates a new database routing schema manager with util.Logger
func NewDatabaseRoutingSchemaWithUtilLogger(configRepo database.SchemaConfigInterface, logger interface{}) *DatabaseRoutingSchema {
	var zapLogger *zap.Logger
	if zl, ok := logger.(*zap.Logger); ok {
		zapLogger = zl
	} else {
		// Fallback to default logger if conversion fails
		zapLogger = zap.L()
	}

	return &DatabaseRoutingSchema{
		configRepo: configRepo,
		logger:     zapLogger,
	}
}

// Initialize loads all enabled routing rules from the database and builds the routing index
func (drs *DatabaseRoutingSchema) Initialize(ctx context.Context) error {
	drs.mu.Lock()
	defer drs.mu.Unlock()

	return drs.initializeInternal(ctx, "initialized")
}

// Reset resets the routing schema configuration and rebuilds the index
func (drs *DatabaseRoutingSchema) Reset(ctx context.Context) error {
	drs.mu.Lock()
	defer drs.mu.Unlock()

	drs.logger.Info("Resetting database routing schema")

	// Clear existing state
	drs.routingIdx = nil
	drs.schemaConfigs = nil
	drs.schemaSingleton = ""

	// Clear cache when configuration changes
	drs.clearCache()

	return drs.initializeInternal(ctx, "reset completed")
}

// initializeInternal contains the common initialization logic for both Initialize and Reset
func (drs *DatabaseRoutingSchema) initializeInternal(ctx context.Context, operation string) error {
	// Load all enabled schema configurations from database
	enabledSchemas, err := drs.configRepo.GetAllEnabled(ctx)
	if err != nil {
		return fmt.Errorf("failed to load enabled schemas: %w", err)
	}

	// Ensure all configured schemas exist in the database
	for _, schemaConfig := range enabledSchemas {
		drs.logger.Info("Ensuring schema exists", zap.String("schema", schemaConfig.DatabaseSchema))
		if err := drs.configRepo.CreateSchema(ctx, schemaConfig.DatabaseSchema); err != nil {
			drs.logger.Error("Failed to ensure schema exists",
				zap.String("schema", schemaConfig.DatabaseSchema),
				zap.String("operation", operation),
				zap.Error(err))
			return fmt.Errorf("failed to ensure schema %s exists: %w", schemaConfig.DatabaseSchema, err)
		}
	}

	// Sort schemas by priority (lower priority number first, catch-all with 9999 goes last)
	sort.Slice(enabledSchemas, func(i, j int) bool {
		return enabledSchemas[i].Priority < enabledSchemas[j].Priority
	})

	drs.schemaConfigs = enabledSchemas

	// Check for singleton optimization (only one enabled schema)
	if len(enabledSchemas) == 1 {
		drs.schemaSingleton = enabledSchemas[0].DatabaseSchema
		drs.logger.Info("Schema singleton optimization enabled",
			zap.String("schema", drs.schemaSingleton),
			zap.Int("priority", enabledSchemas[0].Priority))
	} else {
		drs.schemaSingleton = ""
	}

	// Build the routing index for fast lookups
	drs.buildRoutingIndex()

	drs.logger.Info(fmt.Sprintf("Database routing schema %s", operation),
		zap.Int("schema_count", len(enabledSchemas)),
		zap.Bool("singleton", drs.schemaSingleton != ""))

	return nil
}

// GetAllEnabled returns all enabled schema configurations
func (drs *DatabaseRoutingSchema) GetAllEnabled(ctx context.Context) ([]*database.DatabaseSchemaConfig, error) {
	return drs.schemaConfigs, nil
}

// GetSchema returns the schema name for given service name and namespace using optimized routing
func (drs *DatabaseRoutingSchema) GetSchema(serviceName, serviceNamespace string) (string, bool) {
	drs.mu.RLock()
	defer drs.mu.RUnlock()

	// Fast path: if schemaSingleton is set, return the only resource
	if drs.schemaSingleton != "" {
		return drs.schemaSingleton, true
	}

	// Clean inputs once (avoid repeated cleaning)
	cleanServiceName := drs.cleanRoutingValue(serviceName)
	cleanNamespace := drs.cleanRoutingValue(serviceNamespace)

	// Check cache for recent lookups
	if schema, found, hit := drs.checkCache(cleanServiceName, cleanNamespace); hit {
		return schema, found
	}

	// Check each schema config in priority order to find the first match
	for _, schemaConfig := range drs.schemaConfigs {
		if drs.schemaMatches(*schemaConfig, cleanServiceName, cleanNamespace) {
			// Cache the result before returning
			drs.cacheResult(cleanServiceName, cleanNamespace, schemaConfig.DatabaseSchema, true)
			return schemaConfig.DatabaseSchema, true
		}
	}

	// Cache the negative result
	drs.cacheResult(cleanServiceName, cleanNamespace, "", false)
	return "", false
}

// GetDefaultSchema returns the default schema if singleton is enabled
func (drs *DatabaseRoutingSchema) GetDefaultSchema() (string, bool) {
	drs.mu.RLock()
	defer drs.mu.RUnlock()

	if drs.schemaSingleton != "" {
		return drs.schemaSingleton, true
	}
	return "", false
}

// GetSchemaConfigs returns all loaded schema configurations
func (drs *DatabaseRoutingSchema) GetSchemaConfigs() []*database.DatabaseSchemaConfig {
	drs.mu.RLock()
	defer drs.mu.RUnlock()

	// Return a copy to prevent external modification
	configs := make([]*database.DatabaseSchemaConfig, len(drs.schemaConfigs))
	copy(configs, drs.schemaConfigs)
	return configs
}

// buildRoutingIndex preprocesses all schema configurations into optimized lookup structures
// Schemas are processed in priority order (1=highest, 9999=lowest) so lower priority numbers take precedence
func (drs *DatabaseRoutingSchema) buildRoutingIndex() {
	idx := &routingIndex{
		serviceNameExact:         make(map[string]string),
		serviceNamespaceExact:    make(map[string]string),
		serviceNamePatterns:      make([]patternRule, 0),
		serviceNamespacePatterns: make([]patternRule, 0),
	}

	// Process schemas in priority order (lowest priority number first, catch-all 9999 last)
	for _, schemaConfig := range drs.schemaConfigs {
		schema := schemaConfig.DatabaseSchema
		priority := schemaConfig.Priority

		// Handle catch-all schemas (empty or null routing)
		if schemaConfig.Routing == nil || len(schemaConfig.Routing) == 0 {
			// Always update catch-all schema - later ones (higher priority numbers) override earlier ones
			// This ensures catch-all with priority 9999 becomes the final fallback
			idx.catchAllSchema = schema
			idx.hasCatchAll = true
			drs.logger.Debug("Set catch-all schema",
				zap.String("schema", schema),
				zap.Int("priority", priority))
			continue
		}

		// Process service_name routing
		if serviceNameValue, ok := schemaConfig.Routing["service_name"]; ok {
			drs.processRoutingField(idx, "service_name", serviceNameValue, schema, priority)
		}

		// Process service_namespace routing
		if serviceNamespaceValue, ok := schemaConfig.Routing["service_namespace"]; ok {
			drs.processRoutingField(idx, "service_namespace", serviceNamespaceValue, schema, priority)
		}

		// Process pattern fields for service_name
		drs.processPatternFields(idx, "service_name", schemaConfig.Routing, schema, priority)

		// Process pattern fields for service_namespace
		drs.processPatternFields(idx, "service_namespace", schemaConfig.Routing, schema, priority)
	}

	drs.routingIdx = idx

	drs.logger.Debug("Routing index built with priority order",
		zap.Int("exact_service_names", len(idx.serviceNameExact)),
		zap.Int("exact_service_namespaces", len(idx.serviceNamespaceExact)),
		zap.Int("service_name_patterns", len(idx.serviceNamePatterns)),
		zap.Int("service_namespace_patterns", len(idx.serviceNamespacePatterns)),
		zap.Bool("has_catch_all", idx.hasCatchAll),
		zap.Bool("has_wildcard", idx.hasWildcard))
}

// processRoutingField handles exact matches and wildcards for a specific field
// Priority-aware: only sets values if not already set (lower priority numbers processed first)
func (drs *DatabaseRoutingSchema) processRoutingField(idx *routingIndex, fieldName string, fieldValue interface{}, schema string, priority int) {
	// Handle string values (exact match or wildcard)
	if fieldStr, ok := fieldValue.(string); ok {
		if fieldStr == "*" {
			// Wildcard - set as fallback if not already set
			if !idx.hasWildcard {
				idx.wildcardSchema = schema
				idx.hasWildcard = true
			}
		} else {
			// Exact match
			cleanValue := drs.cleanRoutingValue(fieldStr)
			if cleanValue != "" {
				if fieldName == "service_name" {
					idx.serviceNameExact[cleanValue] = schema
				} else if fieldName == "service_namespace" {
					idx.serviceNamespaceExact[cleanValue] = schema
				}
			}
		}
		return
	}

	// Handle array values (multiple exact matches)
	if fieldList, ok := fieldValue.([]interface{}); ok {
		for _, item := range fieldList {
			if itemStr, ok := item.(string); ok {
				cleanValue := drs.cleanRoutingValue(itemStr)
				if cleanValue != "" {
					switch fieldName {
					case "service_name":
						if _, exists := idx.serviceNameExact[cleanValue]; !exists {
							idx.serviceNameExact[cleanValue] = schema
							drs.logger.Debug("Set exact service name match",
								zap.String("value", cleanValue),
								zap.String("schema", schema),
								zap.Int("priority", priority))
						}
					case "service_namespace":
						if _, exists := idx.serviceNamespaceExact[cleanValue]; !exists {
							idx.serviceNamespaceExact[cleanValue] = schema
							drs.logger.Debug("Set exact service namespace match",
								zap.String("value", cleanValue),
								zap.String("schema", schema),
								zap.Int("priority", priority))
						}
					}
				}
			}
		}
	}
}

// processPatternFields handles pattern-based routing (prefix_, suffix_, contains_)
// Priority-aware: patterns are added in priority order, with higher priority patterns checked first
func (drs *DatabaseRoutingSchema) processPatternFields(idx *routingIndex, fieldName string, routing map[string]interface{}, schema string, priority int) {
	rule := patternRule{schema: schema}
	hasPattern := false

	// Check for prefix pattern
	if prefix, ok := routing[fieldName+"_prefix"]; ok {
		if prefixStr, ok := prefix.(string); ok {
			rule.prefix = prefixStr
			rule.hasPrefix = true
			hasPattern = true
		}
	}

	// Check for suffix pattern
	if suffix, ok := routing[fieldName+"_suffix"]; ok {
		if suffixStr, ok := suffix.(string); ok {
			rule.suffix = suffixStr
			rule.hasSuffix = true
			hasPattern = true
		}
	}

	// Check for contains pattern
	if contains, ok := routing[fieldName+"_contains"]; ok {
		if containsStr, ok := contains.(string); ok {
			rule.contains = containsStr
			rule.hasContains = true
			hasPattern = true
		}
	}

	// Add rule to appropriate pattern list if any patterns were found
	// Patterns are added in priority order (higher priority first)
	if hasPattern {
		if fieldName == "service_name" {
			idx.serviceNamePatterns = append(idx.serviceNamePatterns, rule)
		} else if fieldName == "service_namespace" {
			idx.serviceNamespacePatterns = append(idx.serviceNamespacePatterns, rule)
		}
		drs.logger.Debug("Added pattern rule",
			zap.String("field", fieldName),
			zap.String("schema", schema),
			zap.Int("priority", priority),
			zap.Bool("has_prefix", rule.hasPrefix),
			zap.Bool("has_suffix", rule.hasSuffix),
			zap.Bool("has_contains", rule.hasContains))
	}
}

// schemaMatches checks if a schema config matches the given service name and namespace
func (drs *DatabaseRoutingSchema) schemaMatches(config database.DatabaseSchemaConfig, serviceName, serviceNamespace string) bool {
	// Catch-all schemas (empty routing) match everything
	if config.Routing == nil || len(config.Routing) == 0 {
		return true
	}

	// Check if this config has any routing rules
	hasServiceNameRouting := config.Routing["service_name"] != nil ||
		config.Routing["service_name_prefix"] != nil ||
		config.Routing["service_name_suffix"] != nil ||
		config.Routing["service_name_contains"] != nil

	hasNamespaceRouting := config.Routing["service_namespace"] != nil ||
		config.Routing["service_namespace_prefix"] != nil ||
		config.Routing["service_namespace_suffix"] != nil ||
		config.Routing["service_namespace_contains"] != nil

	// If no routing rules, it's a catch-all
	if !hasServiceNameRouting && !hasNamespaceRouting {
		return true
	}

	// Check exact service_name match
	if serviceNameValue, ok := config.Routing["service_name"]; ok {
		if serviceName == "" || !drs.matchesRoutingValue(serviceNameValue, serviceName) {
			return false
		}
	} else if hasServiceNameRouting {
		// Has service name patterns but no exact match - check patterns
		if serviceName == "" || !drs.matchesRoutingPatterns(config.Routing, "service_name", serviceName) {
			return false
		}
	}

	// Check exact service_namespace match
	if namespaceValue, ok := config.Routing["service_namespace"]; ok {
		if serviceNamespace == "" || !drs.matchesRoutingValue(namespaceValue, serviceNamespace) {
			return false
		}
	} else if hasNamespaceRouting {
		// Has namespace patterns but no exact match - check patterns
		if serviceNamespace == "" || !drs.matchesRoutingPatterns(config.Routing, "service_namespace", serviceNamespace) {
			return false
		}
	}

	return true
}

// matchesRoutingValue checks if a value matches a routing field value (exact, wildcard, array)
func (drs *DatabaseRoutingSchema) matchesRoutingValue(routingValue interface{}, value string) bool {
	if fieldStr, ok := routingValue.(string); ok {
		if fieldStr == "*" {
			return true
		}
		return drs.cleanRoutingValue(fieldStr) == value
	}

	if fieldList, ok := routingValue.([]interface{}); ok {
		for _, item := range fieldList {
			if itemStr, ok := item.(string); ok {
				if drs.cleanRoutingValue(itemStr) == value {
					return true
				}
			}
		}
	}

	return false
}

// matchesRoutingPatterns checks if a value matches pattern-based routing
func (drs *DatabaseRoutingSchema) matchesRoutingPatterns(routing map[string]interface{}, fieldName string, value string) bool {
	if prefix, ok := routing[fieldName+"_prefix"]; ok {
		if prefixStr, ok := prefix.(string); ok && strings.HasPrefix(value, prefixStr) {
			return true
		}
	}

	if suffix, ok := routing[fieldName+"_suffix"]; ok {
		if suffixStr, ok := suffix.(string); ok && strings.HasSuffix(value, suffixStr) {
			return true
		}
	}

	if contains, ok := routing[fieldName+"_contains"]; ok {
		if containsStr, ok := contains.(string); ok && strings.Contains(value, containsStr) {
			return true
		}
	}

	return false
}

// checkCache checks if a lookup result is in the cache using thread-safe sync.Map
func (drs *DatabaseRoutingSchema) checkCache(serviceName, serviceNamespace string) (string, bool, bool) {
	key := cacheKey(serviceName, serviceNamespace)
	if val, ok := drs.cacheMap.Load(key); ok {
		entry := val.(*cacheEntry)
		return entry.schema, entry.found, true
	}
	return "", false, false
}

// cacheResult stores a lookup result using thread-safe sync.Map
func (drs *DatabaseRoutingSchema) cacheResult(serviceName, serviceNamespace, schema string, found bool) {
	key := cacheKey(serviceName, serviceNamespace)
	entry := &cacheEntry{
		serviceName:      serviceName,
		serviceNamespace: serviceNamespace,
		schema:           schema,
		found:            found,
	}
	drs.cacheMap.Store(key, entry)
}

// clearCache resets the cache (called when routing configuration changes)
// Uses Range/Delete to avoid race conditions with concurrent reads
func (drs *DatabaseRoutingSchema) clearCache() {
	drs.cacheMap.Range(func(key, value interface{}) bool {
		drs.cacheMap.Delete(key)
		return true
	})
}

// cleanRoutingValue extracts the relevant part of a routing value before any comment or whitespace
func (drs *DatabaseRoutingSchema) cleanRoutingValue(value string) string {
	clean := strings.TrimSpace(value)
	if idx := strings.Index(clean, "#"); idx != -1 {
		clean = strings.TrimSpace(clean[:idx])
	}
	if clean == "" || clean == "null" {
		return ""
	}
	return clean
}
