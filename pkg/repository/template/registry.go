package template

import (
	"fmt"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
	"github.com/medxops/trazr-otel/pkg/util"
)

// TemplateRegistry provides centralized template registration and management
type TemplateRegistry interface {
	Register(name string, template *TemplateInfo) error
	Get(name string) (*TemplateInfo, error)
	List() []string
	Remove(name string) error
	Clear() error
	GetStats() map[string]interface{}
}

// TemplateRegistryImpl implements the TemplateRegistry interface
type TemplateRegistryImpl struct {
	templates sync.Map
	logger    util.Logger
	config    *RegistryConfig
}

// RegistryConfig holds configuration for the template registry
type RegistryConfig struct {
	MaxTemplates int           `json:"max_templates"`
	TTL          time.Duration `json:"ttl"`
	EnableStats  bool          `json:"enable_stats"`
}

// DefaultRegistryConfig returns default registry configuration
func DefaultRegistryConfig() *RegistryConfig {
	return &RegistryConfig{
		MaxTemplates: 1000,
		TTL:          1 * time.Hour,
		EnableStats:  true,
	}
}

// NewTemplateRegistry creates a new template registry with default configuration
func NewTemplateRegistry(logger util.Logger) TemplateRegistry {
	return NewTemplateRegistryWithConfig(DefaultRegistryConfig(), logger)
}

// NewTemplateRegistryWithConfig creates a new template registry with custom configuration
func NewTemplateRegistryWithConfig(config *RegistryConfig, logger util.Logger) TemplateRegistry {
	return &TemplateRegistryImpl{
		logger: logger,
		config: config,
	}
}

// Register registers a template with the registry
func (r *TemplateRegistryImpl) Register(name string, template *TemplateInfo) error {
	if name == "" {
		return fmt.Errorf("template name cannot be empty")
	}

	if template == nil {
		return fmt.Errorf("template cannot be nil")
	}

	// Check if we're at capacity
	count := 0
	r.templates.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	if count >= r.config.MaxTemplates {
		return fmt.Errorf("template registry is at capacity (%d templates)", r.config.MaxTemplates)
	}

	// Store the template
	r.templates.Store(name, template)
	r.logger.Debug("Template registered", "name", name, "template_id", template.Template.ID)
	return nil
}

// Get retrieves a template from the registry
func (r *TemplateRegistryImpl) Get(name string) (*TemplateInfo, error) {
	if name == "" {
		return nil, fmt.Errorf("template name cannot be empty")
	}

	if value, exists := r.templates.Load(name); exists {
		if template, ok := value.(*TemplateInfo); ok {
			return template, nil
		}
		return nil, fmt.Errorf("invalid template type for name %s", name)
	}

	return nil, fmt.Errorf("template %s not found", name)
}

// List returns all registered template names
func (r *TemplateRegistryImpl) List() []string {
	var names []string
	r.templates.Range(func(key, value interface{}) bool {
		if name, ok := key.(string); ok {
			names = append(names, name)
		}
		return true
	})
	return names
}

// Remove removes a template from the registry
func (r *TemplateRegistryImpl) Remove(name string) error {
	if name == "" {
		return fmt.Errorf("template name cannot be empty")
	}

	if _, exists := r.templates.LoadAndDelete(name); exists {
		r.logger.Debug("Template removed", "name", name)
		return nil
	}

	return fmt.Errorf("template %s not found", name)
}

// Clear removes all templates from the registry
func (r *TemplateRegistryImpl) Clear() error {
	r.templates = sync.Map{}
	r.logger.Info("Template registry cleared")
	return nil
}

// GetStats returns registry statistics
func (r *TemplateRegistryImpl) GetStats() map[string]interface{} {
	if !r.config.EnableStats {
		return map[string]interface{}{"enabled": false}
	}

	count := 0
	totalUsage := 0
	r.templates.Range(func(key, value interface{}) bool {
		count++
		if template, ok := value.(*TemplateInfo); ok {
			totalUsage += template.Usage
		}
		return true
	})

	return map[string]interface{}{
		"template_count": count,
		"max_templates":  r.config.MaxTemplates,
		"total_usage":    totalUsage,
		"ttl":            r.config.TTL,
	}
}

// TemplateInfo holds template data and metadata for deduplication
type TemplateInfo struct {
	Template       *database.EmbeddingConfigTemplate
	Usage          int                            // Number of vector stores using this template
	ContentEngine  *content.ContentTemplateEngine // Pre-compiled content template engine
	MetadataEngine *content.ContentTemplateEngine // Pre-compiled metadata template engine
	CreatedAt      time.Time
	LastUsed       time.Time
}

// NewTemplateInfo creates a new template info
func NewTemplateInfo(template *database.EmbeddingConfigTemplate, contentEngine, metadataEngine *content.ContentTemplateEngine) *TemplateInfo {
	now := time.Now()
	return &TemplateInfo{
		Template:       template,
		Usage:          1,
		ContentEngine:  contentEngine,
		MetadataEngine: metadataEngine,
		CreatedAt:      now,
		LastUsed:       now,
	}
}

// IncrementUsage increments the usage count and updates last used time
func (ti *TemplateInfo) IncrementUsage() {
	ti.Usage++
	ti.LastUsed = time.Now()
}

// IsExpired checks if the template has expired based on TTL
func (ti *TemplateInfo) IsExpired(ttl time.Duration) bool {
	return time.Since(ti.LastUsed) > ttl
}
