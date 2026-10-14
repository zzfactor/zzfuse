// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trazrexporter // import "github.com/medxops/trazr-otel/trazrexporter"

import (
	"context"
	"fmt"

	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/zzfuseauth"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.uber.org/zap"
)

// baseExporter contains common fields and initialization logic for all exporters.
type baseExporter struct {
	logger         *zap.Logger
	cfg            *Config
	srm            repository.ServiceResourceManagerInterface
	signalType     string
	tenantResolver zzfuseauth.Resolver
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
	if e.cfg == nil {
		return fmt.Errorf("exporter configuration is unavailable")
	}
	resolver, ok := host.GetExtensions()[e.cfg.Authenticator].(zzfuseauth.Resolver)
	if !ok {
		return fmt.Errorf("configured tenant ingestion authenticator is unavailable")
	}
	e.tenantResolver = resolver

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

func (e *baseExporter) tenantSchema(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if e.tenantResolver == nil {
		return "", consumererror.NewPermanent(zzfuseauth.ErrUnauthenticated)
	}
	tenant, err := e.tenantResolver.ResolveTenant(ctx)
	if err != nil {
		return "", consumererror.NewPermanent(err)
	}
	return tenant.Schema, nil
}

func (e *baseExporter) tenantIngressSchema(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if e.tenantResolver == nil {
		return "", consumererror.NewPermanent(zzfuseauth.ErrUnauthenticated)
	}
	tenant, err := e.tenantResolver.ResolveIngressTenant(ctx)
	if err != nil {
		return "", consumererror.NewPermanent(err)
	}
	return tenant.Schema, nil
}

// shutdown handles graceful shutdown.
// Note: The ServiceResourceManager is owned by the TrazrExtension and will be shut down there.
func (e *baseExporter) shutdown(_ context.Context) error {
	return nil
}
