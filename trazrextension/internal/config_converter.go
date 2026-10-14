package internal

import (
	"encoding/json"
	"time"
)

// ConfigConverter handles conversion between database types and HTTP response types
type ConfigConverter struct{}

// NewConfigConverter creates a new config converter
func NewConfigConverter() *ConfigConverter {
	return &ConfigConverter{}
}

// ConvertToResponse converts any config type to a generic ConfigResponse
func (c *ConfigConverter) ConvertToResponse(config interface{}, configType ConfigType) (*ConfigResponse, error) {
	if config == nil {
		return nil, nil
	}

	// Convert to map for generic field access
	configMap, err := c.toMap(config)
	if err != nil {
		return nil, err
	}

	response := &ConfigResponse{
		ID:          c.getInt64(configMap, "id"),
		Name:        c.getString(configMap, "name"),
		Description: c.getString(configMap, "description"),
		Enabled:     c.getBool(configMap, "enabled"),
		Priority:    c.getPriority(configMap, configType),
		CreatedAt:   c.getTimeString(configMap, "created_at"),
		UpdatedAt:   c.getTimeString(configMap, "updated_at"),
		Config:      map[string]interface{}{"type": string(configType)},
		Metadata:    c.getMetadata(configMap, configType),
	}

	return response, nil
}

// ConvertSliceToResponse converts a slice of configs to ConfigResponse slice
func (c *ConfigConverter) ConvertSliceToResponse(configs interface{}, configType ConfigType) ([]ConfigResponse, error) {
	// Handle different slice types using reflection-like approach
	configSlice, err := c.toSlice(configs)
	if err != nil {
		return nil, err
	}

	result := make([]ConfigResponse, 0, len(configSlice))
	for _, config := range configSlice {
		if response, err := c.ConvertToResponse(config, configType); err == nil && response != nil {
			result = append(result, *response)
		}
	}

	return result, nil
}

// Helper methods
func (c *ConfigConverter) toMap(config interface{}) (map[string]interface{}, error) {
	jsonBytes, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *ConfigConverter) toSlice(configs interface{}) ([]interface{}, error) {
	// Convert typed slice to interface{} slice
	jsonBytes, err := json.Marshal(configs)
	if err != nil {
		return nil, err
	}

	var result []interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *ConfigConverter) getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (c *ConfigConverter) getInt64(m map[string]interface{}, key string) int64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return 0
}

func (c *ConfigConverter) getBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

func (c *ConfigConverter) getTimeString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if timeStr, ok := val.(string); ok {
			return timeStr
		}
		if t, ok := val.(time.Time); ok {
			return t.Format(time.RFC3339)
		}
	}
	return ""
}

func (c *ConfigConverter) getPriority(m map[string]interface{}, configType ConfigType) int {
	// Only schema configs have priority
	if configType == ConfigTypeSchemas {
		return int(c.getInt64(m, "priority"))
	}
	return 0
}

func (c *ConfigConverter) getMetadata(m map[string]interface{}, configType ConfigType) map[string]interface{} {
	metadata := make(map[string]interface{})

	switch configType {
	case ConfigTypeSchemas:
		if schema := c.getString(m, "schema"); schema != "" {
			metadata["database_schema"] = schema
			metadata["routing"] = c.getString(m, "routing")
			metadata["priority"] = c.getInt64(m, "priority")
		}
	case ConfigTypeProcessors:
		metadata["database_id"] = c.getInt64(m, "database_id")
		metadata["vectorstore_id"] = c.getInt64(m, "vectorstore_id")
		metadata["template_id"] = c.getInt64(m, "template_id")
		metadata["processing_mode"] = c.getString(m, "processing_mode")
		metadata["routing"] = c.getString(m, "routing")
		metadata["filter"] = c.getString(m, "filter")
	case ConfigTypeEmbeddings:
		metadata["provider"] = c.getString(m, "provider")
		metadata["model"] = c.getString(m, "model")
		metadata["dimension"] = c.getInt64(m, "dimension")
		metadata["max_tokens"] = c.getInt64(m, "max_tokens")
	case ConfigTypeTemplates:
		if content, ok := m["content"].(map[string]interface{}); ok {
			metadata["content"] = content
			metadata["has_content"] = len(content) > 0
		}
	case ConfigTypeVectorStores:
		metadata["provider"] = c.getString(m, "provider")
		metadata["embedding_id"] = c.getInt64(m, "embedding_id")
		metadata["template_id"] = c.getInt64(m, "template_id")
	}

	return metadata
}
