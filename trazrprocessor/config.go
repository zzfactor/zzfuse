package trazrprocessor

import (
	"go.opentelemetry.io/collector/component"
)

// Config defines the configuration for the processor.
type Config struct {
	component.Config
	// Add custom config fields here
}

// createDefaultConfig returns the default config for the processor.
func createDefaultConfig() component.Config {
	return &Config{}
}
