// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trazrexporter // import "github.com/medxops/trazr-otel/trazrexporter"

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/trazrexporter/internal"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type metricsExporter struct {
	baseExporter
	tablesConfig internal.MetricTablesConfigMapper
	metricWriter repository.MetricWriterService
}

// newMetricsExporter creates a metrics exporter without initializing the SRM.
// The SRM is obtained from the TrazrExtension via Host.GetExtensions() during start().
func newMetricsExporter(logger *zap.Logger, cfg *Config) *metricsExporter {
	tablesConfig := generateMetricTablesConfigMapper(cfg)
	return &metricsExporter{
		baseExporter: baseExporter{
			logger:     logger,
			cfg:        cfg,
			signalType: "metrics",
		},
		tablesConfig: tablesConfig,
	}
}

// start initializes the exporter by obtaining the ServiceResourceManager from the TrazrExtension.
// This is called by the OTel Collector after all extensions have started.
func (e *metricsExporter) start(ctx context.Context, host component.Host) error {
	if err := e.initializeSRM(ctx, host); err != nil {
		return err
	}

	// Get the metric writer service
	metricWriter, err := e.srm.GetMetricWriter()
	if err != nil {
		return fmt.Errorf("failed to get metric writer: %w", err)
	}
	e.metricWriter = metricWriter

	e.logger.Info("Metrics exporter started successfully")
	return nil
}

func generateMetricTablesConfigMapper(cfg *Config) internal.MetricTablesConfigMapper {
	return internal.MetricTablesConfigMapper{
		pmetric.MetricTypeGauge:                cfg.MetricsTables.Gauge,
		pmetric.MetricTypeSum:                  cfg.MetricsTables.Sum,
		pmetric.MetricTypeSummary:              cfg.MetricsTables.Summary,
		pmetric.MetricTypeHistogram:            cfg.MetricsTables.Histogram,
		pmetric.MetricTypeExponentialHistogram: cfg.MetricsTables.ExponentialHistogram,
	}
}

// pushMetricsData exports metrics data to PostgreSQL using sequential filtering.
//
// The function implements three-phase sequential filtering:
// 1. Resource filtering with signal type (most restrictive, done once per ResourceMetrics)
// 2. Scope filtering (done once per ScopeMetrics, only on resource matches)
// 3. Attribute filtering (done per Metric DataPoint, only on scope matches)
//
// This approach maximizes performance through early filtering and supports routing to multiple schemas.
func (e *metricsExporter) pushMetricsData(ctx context.Context, md pmetric.Metrics) error {
	if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
		span.AddEvent("pushMetricsData called", trace.WithAttributes(
			attribute.Int("num_resource_metrics", md.ResourceMetrics().Len()),
		))
	}

	// Separate groups for each metric type and schema
	gaugeGroups := make(map[string][]repository.GaugeMetricEntry)                  // key: schemaName
	sumGroups := make(map[string][]repository.SumMetricEntry)                      // key: schemaName
	histogramGroups := make(map[string][]repository.HistogramMetricEntry)          // key: schemaName
	summaryGroups := make(map[string][]repository.SummaryMetricEntry)              // key: schemaName
	expHistGroups := make(map[string][]repository.ExponentialHistogramMetricEntry) // key: schemaName

	for i := 0; i < md.ResourceMetrics().Len(); i++ {
		metrics := md.ResourceMetrics().At(i)
		resAttr := metrics.Resource().Attributes()

		serviceName := internal.GetServiceName(resAttr) // Original service name
		serviceNamespace := internal.GetServiceNamespace(resAttr)
		schema, found, err := e.srm.GetSchema(serviceName, serviceNamespace)
		if err != nil {
			return fmt.Errorf("failed to get schema for service %s: %w", serviceName, err)
		}
		if !found {
			e.logger.Debug("Skipping ResourceMetrics - schema not found",
				zap.String("service_name", serviceName),
				zap.String("service_namespace", serviceNamespace))
			continue // Skip entire ResourceMetrics if schema not found
		}

		// Marshal resource attributes once per ResourceMetrics (immutable)
		resourceAttrsJSON := internal.SafeJSON(internal.AttributesToMap(resAttr))
		resourceSchemaURL := internal.PtrFromString(metrics.SchemaUrl())

		for j := 0; j < metrics.ScopeMetrics().Len(); j++ {
			rs := metrics.ScopeMetrics().At(j)
			scope := rs.Scope()

			// Marshal scope attributes once per ScopeMetrics
			scopeAttrsJSON := internal.SafeJSON(internal.AttributesToMap(scope.Attributes()))
			scopeName := scope.Name()
			scopeVersion := scope.Version()
			scopeDroppedAttrCount := int32(scope.DroppedAttributesCount())
			scopeSchemaURL := rs.SchemaUrl()

			for k := 0; k < rs.Metrics().Len(); k++ {
				r := rs.Metrics().At(k)

				switch r.Type() {
				case pmetric.MetricTypeGauge:
					for l := 0; l < r.Gauge().DataPoints().Len(); l++ {
						dp := r.Gauge().DataPoints().At(l)

						e.logger.Debug("Processing Gauge DataPoint",
							zap.String("metric_name", r.Name()),
							zap.Int("data_point_index", l))

						timestamp := internal.NormalizeTimestamp(dp.Timestamp().AsTime())
						flags := int32(dp.Flags())
						exemplars := internal.ConvertExemplars(dp.Exemplars())
						entry := repository.GaugeMetricEntry{
							TimestampNS:           timestamp.Time.UnixNano(),
							Timestamp:             timestamp.Time,
							// Service entity fields (OTel v1.39.0)
							ServiceName:           serviceName, // Original service name
							ServiceNamespace:      internal.GetServiceNamespacePtr(resAttr),
							ServiceInstanceID:     internal.GetServiceInstanceIDPtr(resAttr),
							Name:                  internal.PtrFromString(r.Name()),
							Description:           internal.PtrFromString(r.Description()),
							Unit:                  internal.PtrFromString(r.Unit()),
							Attributes:            internal.SafeJSON(internal.AttributesToMap(dp.Attributes())),
							Value:                 internal.PtrFromFloat64(internal.GetValue(dp.IntValue(), dp.DoubleValue(), dp.ValueType())),
							Flags:                 &flags,
							Exemplars:             exemplars,
							ResourceAttributes:    resourceAttrsJSON,
							ScopeName:             internal.PtrFromString(scopeName),
							ScopeVersion:          internal.PtrFromString(scopeVersion),
							ScopeAttributes:       scopeAttrsJSON,
							ScopeDroppedAttrCount: internal.PtrFromInt32(scopeDroppedAttrCount),
							Metadata:              []byte("{}"),
							ResourceSchemaURL:     resourceSchemaURL,
							ScopeSchemaURL:        internal.PtrFromString(scopeSchemaURL),
						}

						gaugeGroups[schema] = append(gaugeGroups[schema], entry)
					}
				case pmetric.MetricTypeSum:
					for l := 0; l < r.Sum().DataPoints().Len(); l++ {
						dp := r.Sum().DataPoints().At(l)

						e.logger.Debug("Processing Sum DataPoint",
							zap.String("metric_name", r.Name()),
							zap.Int("data_point_index", l))

						timestamp := internal.NormalizeTimestamp(dp.Timestamp().AsTime())
						flags := int32(dp.Flags())
						exemplars := internal.ConvertExemplars(dp.Exemplars())
						aggTemporality := int32(r.Sum().AggregationTemporality())
						isMonotonic := r.Sum().IsMonotonic()
						startTime := dp.StartTimestamp().AsTime()
						entry := repository.SumMetricEntry{
							TimestampNS:            timestamp.Time.UnixNano(),
							Timestamp:              timestamp.Time,
							// Service entity fields (OTel v1.39.0)
							ServiceName:            serviceName, // Original service name
							ServiceNamespace:       internal.GetServiceNamespacePtr(resAttr),
							ServiceInstanceID:      internal.GetServiceInstanceIDPtr(resAttr),
							ResourceAttributes:     resourceAttrsJSON,
							ScopeName:              internal.PtrFromString(scopeName),
							ScopeVersion:           internal.PtrFromString(scopeVersion),
							ScopeAttributes:        scopeAttrsJSON,
							ScopeDroppedAttrCount:  internal.PtrFromInt32(scopeDroppedAttrCount),
							Name:                   internal.PtrFromString(r.Name()),
							Description:            internal.PtrFromString(r.Description()),
							Unit:                   internal.PtrFromString(r.Unit()),
							Attributes:             internal.SafeJSON(internal.AttributesToMap(dp.Attributes())),
							StartTime:              &startTime,
							Value:                  internal.PtrFromFloat64(internal.GetValue(dp.IntValue(), dp.DoubleValue(), dp.ValueType())),
							Flags:                  &flags,
							Exemplars:              exemplars,
							AggregationTemporality: &aggTemporality,
							IsMonotonic:            &isMonotonic,
							Metadata:               []byte("{}"),
							ResourceSchemaURL:      resourceSchemaURL,
							ScopeSchemaURL:         internal.PtrFromString(scopeSchemaURL),
						}

						sumGroups[schema] = append(sumGroups[schema], entry)
					}
				case pmetric.MetricTypeHistogram:
					for l := 0; l < r.Histogram().DataPoints().Len(); l++ {
						dp := r.Histogram().DataPoints().At(l)

						e.logger.Debug("Processing Histogram DataPoint",
							zap.String("metric_name", r.Name()),
							zap.Int("data_point_index", l))

						timestamp := internal.NormalizeTimestamp(dp.Timestamp().AsTime())
						flags := int32(dp.Flags())
						exemplars := internal.ConvertExemplars(dp.Exemplars())
						aggTemporality := int32(r.Histogram().AggregationTemporality())
						startTime := dp.StartTimestamp().AsTime()
						bucketsJSON, _ := json.Marshal(map[string]interface{}{
							"bucket_counts":   dp.BucketCounts().AsRaw(),
							"explicit_bounds": dp.ExplicitBounds().AsRaw(),
						})
						// Extract optional min/max if present (OTel histogram optional fields)
						var minPtr, maxPtr *float64
						if dp.HasMin() {
							minPtr = internal.PtrFromFloat64(dp.Min())
						}
						if dp.HasMax() {
							maxPtr = internal.PtrFromFloat64(dp.Max())
						}
						entry := repository.HistogramMetricEntry{
							TimestampNS:            timestamp.Time.UnixNano(),
							Timestamp:              timestamp.Time,
							// Service entity fields (OTel v1.39.0)
							ServiceName:            serviceName, // Original service name
							ServiceNamespace:       internal.GetServiceNamespacePtr(resAttr),
							ServiceInstanceID:      internal.GetServiceInstanceIDPtr(resAttr),
							Name:                   internal.PtrFromString(r.Name()),
							Description:            internal.PtrFromString(r.Description()),
							Unit:                   internal.PtrFromString(r.Unit()),
							Attributes:             internal.SafeJSON(internal.AttributesToMap(dp.Attributes())),
							StartTime:              &startTime,
							Count:                  internal.PtrFromInt64(int64(dp.Count())),
							Sum:                    internal.PtrFromFloat64(dp.Sum()),
							Min:                    minPtr,
							Max:                    maxPtr,
							Buckets:                bucketsJSON,
							Flags:                  &flags,
							AggregationTemporality: &aggTemporality,
							Exemplars:              exemplars,
							ResourceAttributes:     resourceAttrsJSON,
							ScopeName:              internal.PtrFromString(scopeName),
							ScopeVersion:           internal.PtrFromString(scopeVersion),
							ScopeAttributes:        scopeAttrsJSON,
							ScopeDroppedAttrCount:  internal.PtrFromInt32(scopeDroppedAttrCount),
							Metadata:               []byte("{}"),
							ResourceSchemaURL:      resourceSchemaURL,
							ScopeSchemaURL:         internal.PtrFromString(scopeSchemaURL),
						}

						histogramGroups[schema] = append(histogramGroups[schema], entry)
					}
				case pmetric.MetricTypeExponentialHistogram:
					for l := 0; l < r.ExponentialHistogram().DataPoints().Len(); l++ {
						dp := r.ExponentialHistogram().DataPoints().At(l)

						e.logger.Debug("Processing ExponentialHistogram DataPoint",
							zap.String("metric_name", r.Name()),
							zap.Int("data_point_index", l))

						timestamp := internal.NormalizeTimestamp(dp.Timestamp().AsTime())
						flags := int32(dp.Flags())
						exemplars := internal.ConvertExemplars(dp.Exemplars())
						aggTemporality := int32(r.ExponentialHistogram().AggregationTemporality())
						startTime := dp.StartTimestamp().AsTime()
						posBucketCounts, _ := json.Marshal(dp.Positive().BucketCounts().AsRaw())
						negBucketCounts, _ := json.Marshal(dp.Negative().BucketCounts().AsRaw())
						entry := repository.ExponentialHistogramMetricEntry{
							TimestampNS:            timestamp.Time.UnixNano(),
							Timestamp:              timestamp.Time,
							// Service entity fields (OTel v1.39.0)
							ServiceName:            serviceName, // Original service name
							ServiceNamespace:       internal.GetServiceNamespacePtr(resAttr),
							ServiceInstanceID:      internal.GetServiceInstanceIDPtr(resAttr),
							ResourceAttributes:     resourceAttrsJSON,
							ScopeName:              internal.PtrFromString(scopeName),
							ScopeVersion:           internal.PtrFromString(scopeVersion),
							ScopeAttributes:        scopeAttrsJSON,
							ScopeDroppedAttrCount:  internal.PtrFromInt32(scopeDroppedAttrCount),
							Name:                   internal.PtrFromString(r.Name()),
							Description:            internal.PtrFromString(r.Description()),
							Unit:                   internal.PtrFromString(r.Unit()),
							Attributes:             internal.SafeJSON(internal.AttributesToMap(dp.Attributes())),
							StartTime:              &startTime,
							Count:                  internal.PtrFromInt64(int64(dp.Count())),
							Sum:                    internal.PtrFromFloat64(dp.Sum()),
							Scale:                  internal.PtrFromInt32(dp.Scale()),
							ZeroCount:              internal.PtrFromInt64(int64(dp.ZeroCount())),
							ZeroThreshold:          internal.PtrFromFloat64(dp.ZeroThreshold()),
							PositiveOffset:         internal.PtrFromInt32(dp.Positive().Offset()),
							PositiveBucketCounts:   posBucketCounts,
							NegativeOffset:         internal.PtrFromInt32(dp.Negative().Offset()),
							NegativeBucketCounts:   negBucketCounts,
							Exemplars:              exemplars,
							Flags:                  &flags,
							Min:                    internal.PtrFromFloat64(dp.Min()),
							Max:                    internal.PtrFromFloat64(dp.Max()),
							AggregationTemporality: &aggTemporality,
							Metadata:               []byte("{}"),
							ResourceSchemaURL:      resourceSchemaURL,
							ScopeSchemaURL:         internal.PtrFromString(scopeSchemaURL),
						}

						expHistGroups[schema] = append(expHistGroups[schema], entry)
					}
				case pmetric.MetricTypeSummary:
					for l := 0; l < r.Summary().DataPoints().Len(); l++ {
						dp := r.Summary().DataPoints().At(l)

						e.logger.Debug("Processing Summary DataPoint",
							zap.String("metric_name", r.Name()),
							zap.Int("data_point_index", l))

						timestamp := internal.NormalizeTimestamp(dp.Timestamp().AsTime())
						flags := int32(dp.Flags())
						startTime := dp.StartTimestamp().AsTime()
						quantilesJSON := internal.ConvertValueAtQuantileToJSONB(dp.QuantileValues())
						entry := repository.SummaryMetricEntry{
							TimestampNS:           timestamp.Time.UnixNano(),
							Timestamp:             timestamp.Time,
							// Service entity fields (OTel v1.39.0)
							ServiceName:           serviceName, // Original service name
							ServiceNamespace:      internal.GetServiceNamespacePtr(resAttr),
							ServiceInstanceID:     internal.GetServiceInstanceIDPtr(resAttr),
							Name:                  internal.PtrFromString(r.Name()),
							Description:           internal.PtrFromString(r.Description()),
							Unit:                  internal.PtrFromString(r.Unit()),
							Attributes:            internal.SafeJSON(internal.AttributesToMap(dp.Attributes())),
							StartTime:             &startTime,
							Count:                 internal.PtrFromInt64(int64(dp.Count())),
							Sum:                   internal.PtrFromFloat64(dp.Sum()),
							Quantiles:             quantilesJSON,
							Flags:                 &flags,
							ResourceAttributes:    resourceAttrsJSON,
							ScopeName:             internal.PtrFromString(scopeName),
							ScopeVersion:          internal.PtrFromString(scopeVersion),
							ScopeAttributes:       scopeAttrsJSON,
							ScopeDroppedAttrCount: internal.PtrFromInt32(scopeDroppedAttrCount),
							Metadata:              []byte("{}"),
							ResourceSchemaURL:     resourceSchemaURL,
							ScopeSchemaURL:        internal.PtrFromString(scopeSchemaURL),
						}

						summaryGroups[schema] = append(summaryGroups[schema], entry)
					}
				}
			}
		}
	}

	// Insert all metric types to their respective schemas
	for schemaName, entries := range gaugeGroups {
		e.logger.Debug("Inserting gauge metrics to schema",
			zap.String("schema_name", schemaName),
			zap.Int("entry_count", len(entries)))

		if _, err := e.metricWriter.InsertGaugeMetrics(ctx, entries, schemaName); err != nil {
			return fmt.Errorf("insert gauge metrics (schema=%s): %w", schemaName, err)
		}
	}
	for schemaName, entries := range sumGroups {
		e.logger.Debug("Inserting sum metrics to schema",
			zap.String("schema_name", schemaName),
			zap.Int("entry_count", len(entries)))

		if _, err := e.metricWriter.InsertSumMetrics(ctx, entries, schemaName); err != nil {
			return fmt.Errorf("insert sum metrics (schema=%s): %w", schemaName, err)
		}
	}
	for schemaName, entries := range histogramGroups {
		e.logger.Debug("Inserting histogram metrics to schema",
			zap.String("schema_name", schemaName),
			zap.Int("entry_count", len(entries)))

		if _, err := e.metricWriter.InsertHistogramMetrics(ctx, entries, schemaName); err != nil {
			return fmt.Errorf("insert histogram metrics (schema=%s): %w", schemaName, err)
		}
	}
	for schemaName, entries := range summaryGroups {
		e.logger.Debug("Inserting summary metrics to schema",
			zap.String("schema_name", schemaName),
			zap.Int("entry_count", len(entries)))

		if _, err := e.metricWriter.InsertSummaryMetrics(ctx, entries, schemaName); err != nil {
			return fmt.Errorf("insert summary metrics (schema=%s): %w", schemaName, err)
		}
	}
	for schemaName, entries := range expHistGroups {
		e.logger.Debug("Inserting exponential histogram metrics to schema",
			zap.String("schema_name", schemaName),
			zap.Int("entry_count", len(entries)))

		if _, err := e.metricWriter.InsertExponentialHistogramMetrics(ctx, entries, schemaName); err != nil {
			return fmt.Errorf("insert exponential histogram metrics (schema=%s): %w", schemaName, err)
		}
	}

	e.logger.Debug("pushMetricsData completed successfully",
		zap.Int("total_gauge_schemas", len(gaugeGroups)),
		zap.Int("total_sum_schemas", len(sumGroups)),
		zap.Int("total_histogram_schemas", len(histogramGroups)),
		zap.Int("total_summary_schemas", len(summaryGroups)),
		zap.Int("total_exp_hist_schemas", len(expHistGroups)))

	return nil
}
