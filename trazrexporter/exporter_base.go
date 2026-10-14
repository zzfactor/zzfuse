// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trazrexporter // import "github.com/medxops/trazr-otel/trazrexporter"

import (
	"context"
	"fmt"

	"github.com/medxops/trazr-otel/pkg/repository"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
)

// baseExporter contains common fields and initialization logic for all exporters.
type baseExporter struct {
	logger     *zap.Logger
	cfg        *Config
	srm        repository.ServiceResourceManagerInterface
	signalType string
}

// initializeSRM initializes the ServiceResourceManager from the TrazrExtension.
// This is called by the OTel Collector after all extensions have started.
// Returns nil if successful, or an error if initialization fails.
func (e *baseExporter) initializeSRM(ctx context.Context, host component.Host) error {
	// Get the TrazrServiceProvider from the extension
	provider, err := getTrazrServiceProvider(host)
	if err != nil {
		return fmt.Errorf("failed to get trazr service provider: %w", err)
	}

	// Wait for the extension to be ready (SRM initialized)
	if err := provider.WaitForReady(ctx); err != nil {
		return fmt.Errorf("trazr services not ready: %w", err)
	}

	// Get the ServiceResourceManager
	srm, err := provider.GetServiceResourceManager()
	if err != nil {
		return fmt.Errorf("failed to get service resource manager: %w", err)
	}
	if srm == nil {
		return fmt.Errorf("trazr service resource manager is unavailable")
	}
	e.srm = srm

	return nil
}

// shutdown handles graceful shutdown.
// Note: The ServiceResourceManager is owned by the TrazrExtension and will be shut down there.
func (e *baseExporter) shutdown(_ context.Context) error {
	return nil
}
