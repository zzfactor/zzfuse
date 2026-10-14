package trazrexporter

import (
	"context"
	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/pkg/repository/mocks"
	"github.com/medxops/trazr-otel/zzfuseauth"
	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/ptrace"
	request "go.opentelemetry.io/collector/pdata/xpdata/request"
	"go.uber.org/zap"
	"os"
	"strings"
	"testing"
)

func TestFreshIngestionRejectsCopiedQueueReceiptAllSignals(t *testing.T) {
	base := &baseExporter{}
	ctx := authenticatedTestContext(t, base, "tenant_a")
	encoded, err := request.MarshalLogs(ctx, plog.NewLogs())
	if err != nil {
		t.Fatal(err)
	}
	replayed, _, err := request.UnmarshalLogs(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if client.FromContext(replayed).Auth != nil {
		t.Fatal("test requires queue authentication loss")
	}
	if schema, err := base.tenantSchema(replayed); err != nil || schema != "tenant_a" {
		t.Fatalf("legitimate WAL delivery rejected: %s %v", schema, err)
	}
	// Nil inner exporters ensure rejection occurs before any enqueue or delivery.
	for signal, consume := range map[string]func(context.Context) error{
		"logs": func(ctx context.Context) error {
			return (&authenticatedLogs{base: base}).ConsumeLogs(ctx, plog.NewLogs())
		},
		"traces": func(ctx context.Context) error {
			return (&authenticatedTraces{base: base}).ConsumeTraces(ctx, ptrace.NewTraces())
		},
		"metrics": func(ctx context.Context) error {
			return (&authenticatedMetrics{base: base}).ConsumeMetrics(ctx, pmetric.NewMetrics())
		},
		"profiles": func(ctx context.Context) error {
			return (&authenticatedProfiles{base: base}).ConsumeProfiles(ctx, pprofile.NewProfiles())
		},
	} {
		t.Run(signal, func(t *testing.T) {
			if err := consume(replayed); err == nil {
				t.Fatal("copied receipt authorized fresh payload")
			}
		})
	}
}

func testAuthenticator(t testing.TB, schema string) *zzfuseauth.Authenticator {
	t.Helper()
	a, err := zzfuseauth.New(&zzfuseauth.Config{SigningKey: configopaque.String(strings.Repeat("test-key-", 5)), Credentials: map[string]zzfuseauth.Credential{
		"test": {Token: configopaque.String(strings.Repeat("test-token-", 4)), Schema: schema, Epoch: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type captureLogWriter struct{ schemas []string }

func (w *captureLogWriter) InsertLogs(_ context.Context, _ []repository.LogEntry, schema string) ([]string, error) {
	w.schemas = append(w.schemas, schema)
	return []string{"1"}, nil
}

type captureTraceWriter struct{ schemas []string }

func (w *captureTraceWriter) InsertSpans(_ context.Context, _ []repository.TraceEntry, schema string) ([]string, error) {
	w.schemas = append(w.schemas, schema)
	return []string{"1"}, nil
}

type captureGaugeWriter struct {
	repository.MetricWriterService
	schemas []string
}

func (w *captureGaugeWriter) InsertGaugeMetrics(_ context.Context, _ []repository.GaugeMetricEntry, schema string) ([]string, error) {
	w.schemas = append(w.schemas, schema)
	return []string{"1"}, nil
}

func TestAllSignalsUseTrustedSchemaIgnoreProducerRoutingAndRejectMissingIdentity(t *testing.T) {
	router := &mocks.MockServiceResourceManager{GetSchemaFunc: func(string, string) (string, bool, error) {
		t.Fatal("producer-controlled resource routing consulted")
		return "tenant_b", true, nil
	}}
	for _, tenant := range []string{"tenant_a", "tenant_b"} {
		t.Run(tenant, func(t *testing.T) {
			other := "tenant_b"
			if tenant == other {
				other = "tenant_a"
			}
			logs := plog.NewLogs()
			rl := logs.ResourceLogs().AppendEmpty()
			rl.Resource().Attributes().PutStr("service.name", other)
			rl.Resource().Attributes().PutStr("service.namespace", other)
			rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().SetStr("private " + tenant)
			logWriter := &captureLogWriter{}
			le := newLogsExporter(zap.NewNop(), createDefaultConfig().(*Config))
			le.srm = router
			le.logWriter = logWriter
			ctx := authenticatedTestContext(t, &le.baseExporter, tenant)
			if err := le.pushLogsData(ctx, logs); err != nil {
				t.Fatal(err)
			}
			if len(logWriter.schemas) != 1 || logWriter.schemas[0] != tenant {
				t.Fatal("log tenant escaped")
			}
			if err := le.pushLogsData(context.Background(), logs); err == nil || len(logWriter.schemas) != 1 {
				t.Fatal("unauthenticated logs written")
			}
			traces := ptrace.NewTraces()
			rs := traces.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", other)
			rs.Resource().Attributes().PutStr("service.namespace", other)
			rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("private " + tenant)
			traceWriter := &captureTraceWriter{}
			te := newTracesExporter(zap.NewNop(), createDefaultConfig().(*Config))
			te.srm = router
			te.traceWriter = traceWriter
			ctx = authenticatedTestContext(t, &te.baseExporter, tenant)
			if err := te.pushTraceData(ctx, traces); err != nil {
				t.Fatal(err)
			}
			if len(traceWriter.schemas) != 1 || traceWriter.schemas[0] != tenant {
				t.Fatal("trace tenant escaped")
			}
			if err := te.pushTraceData(context.Background(), traces); err == nil {
				t.Fatal("unauthenticated traces accepted")
			}
			metrics := pmetric.NewMetrics()
			rm := metrics.ResourceMetrics().AppendEmpty()
			rm.Resource().Attributes().PutStr("service.name", other)
			rm.Resource().Attributes().PutStr("service.namespace", other)
			rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty().SetEmptyGauge().DataPoints().AppendEmpty().SetDoubleValue(1)
			metricWriter := &captureGaugeWriter{}
			me := newMetricsExporter(zap.NewNop(), createDefaultConfig().(*Config))
			me.srm = router
			me.metricWriter = metricWriter
			ctx = authenticatedTestContext(t, &me.baseExporter, tenant)
			if err := me.pushMetricsData(ctx, metrics); err != nil {
				t.Fatal(err)
			}
			if len(metricWriter.schemas) != 1 || metricWriter.schemas[0] != tenant {
				t.Fatal("metric tenant escaped")
			}
			if err := me.pushMetricsData(context.Background(), metrics); err == nil {
				t.Fatal("unauthenticated metrics accepted")
			}
			raw, err := os.ReadFile("../pkg/repository/testdata/current-profiles.pb")
			if err != nil {
				t.Fatal(err)
			}
			profiles, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < profiles.ResourceProfiles().Len(); i++ {
				profiles.ResourceProfiles().At(i).Resource().Attributes().PutStr("service.name", other)
				profiles.ResourceProfiles().At(i).Resource().Attributes().PutStr("service.namespace", other)
			}
			profileWriter := &captureProfileWriter{}
			pe := newProfilesExporter(zap.NewNop(), createDefaultConfig().(*Config))
			pe.srm = router
			pe.profileWriter = profileWriter
			ctx = authenticatedTestContext(t, &pe.baseExporter, tenant)
			if err := pe.pushProfileData(ctx, profiles); err != nil {
				t.Fatal(err)
			}
			if len(profileWriter.schemas) != 1 || profileWriter.schemas[0] != tenant || len(profileWriter.entries) != 3 {
				t.Fatal("profile tenant escaped")
			}
			if err := pe.pushProfileData(context.Background(), profiles); err == nil || len(profileWriter.entries) != 3 {
				t.Fatal("unauthenticated profiles written")
			}
		})
	}
}

func authenticatedTestContext(t testing.TB, e *baseExporter, schema string) context.Context {
	t.Helper()
	a := testAuthenticator(t, schema)
	e.tenantResolver = a
	ctx, err := a.Authenticate(context.Background(), map[string][]string{"authorization": {"Bearer " + strings.Repeat("test-token-", 4)}})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
