// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trazrexporter // import "github.com/medxops/trazr-otel/trazrexporter"

import (
	"context"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/trazrexporter/internal"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// profilesExporter exports OpenTelemetry pprof profiles to PostgreSQL using the repository ProfileWriterService.
type profilesExporter struct {
	baseExporter
	profileWriter repository.ProfileWriterService
}

// newProfilesExporter creates a profiles exporter without initializing the SRM.
// The SRM is obtained from the TrazrExtension via Host.GetExtensions() during start().
func newProfilesExporter(logger *zap.Logger, cfg *Config) *profilesExporter {
	return &profilesExporter{
		baseExporter: baseExporter{
			logger:     logger,
			cfg:        cfg,
			signalType: "profiles",
		},
	}
}

// start initializes the exporter by obtaining the ServiceResourceManager from the TrazrExtension.
// This is called by the OTel Collector after all extensions have started.
func (e *profilesExporter) start(ctx context.Context, host component.Host) error {
	if err := e.initializeSRM(ctx, host); err != nil {
		return err
	}

	// Get the profile writer service
	profileWriter, err := e.srm.GetProfileWriter()
	if err != nil {
		return fmt.Errorf("failed to get profile writer: %w", err)
	}
	e.profileWriter = profileWriter

	e.logger.Info("Profiles exporter started successfully")
	return nil
}

// pushProfileData isolates each profile's reachable dictionary and context before
// schema routing. Validation/routing errors precede all writes; mixed-schema
// requests cannot copy unused foreign dictionary data into another schema.
func (e *profilesExporter) pushProfileData(ctx context.Context, pd pprofile.Profiles) error {
	if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
		span.AddEvent("pushProfileData called", trace.WithAttributes(
			attribute.Int("num_resource_profiles", pd.ResourceProfiles().Len()),
		))
	}

	rows, err := repository.CompactProfiles(pd)
	if err != nil {
		return fmt.Errorf("isolate profiles: %w", err)
	}
	groups := make(map[string][]repository.ProfileEntry)
	resourceSchemas := make(map[int]string)
	// All rows are validated and isolated before any schema's writes begin.
	// Empty resources and orphan dictionary values cannot cross this boundary.
	for _, row := range rows {
		res := row.Data.ResourceProfiles().At(0).Resource()
		schema, ok := resourceSchemas[row.ResourceIndex]
		if !ok {
			name, namespace := internal.GetServiceName(res.Attributes()), internal.GetServiceNamespace(res.Attributes())
			resolved, found, err := e.srm.GetSchema(name, namespace)
			if err != nil {
				return fmt.Errorf("resolve profile schema: %w", err)
			}
			if !found || resolved == "" {
				return fmt.Errorf("profile resource has no target schema")
			}
			schema = resolved
			resourceSchemas[row.ResourceIndex] = schema
		}
		entry, err := repository.ProfileEntryFromProfiles(row.Data, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("convert profile: %w", err)
		}
		groups[schema] = append(groups[schema], entry)
	}

	// Insert profiles to database
	for schemaName, entries := range groups {
		e.logger.Debug("Inserting profiles to schema",
			zap.String("schema_name", schemaName),
			zap.Int("entry_count", len(entries)))

		_, err := e.profileWriter.InsertProfiles(ctx, entries, schemaName)
		if err != nil {
			return fmt.Errorf("insert profiles (schema=%s): %w", schemaName, err)
		}
	}

	e.logger.Debug("pushProfileData completed successfully",
		zap.Int("total_schemas_processed", len(groups)))

	return nil
}
