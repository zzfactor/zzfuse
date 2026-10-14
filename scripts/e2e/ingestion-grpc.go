// A real OTLP gRPC client for the task-owned full-stack ingestion boundary test.
// Run from trazr-otel with JSON on stdin. Credentials are never printed.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/pprofile/pprofileotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type input struct {
	Endpoint     string            `json:"endpoint"`
	RunID        string            `json:"run_id"`
	Tokens       map[string]string `json:"tokens"`
	ProfilesFile string            `json:"profiles_file"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var cfg input
	if err := json.NewDecoder(os.Stdin).Decode(&cfg); err != nil {
		return fmt.Errorf("invalid gRPC driver input")
	}
	host, _, err := net.SplitHostPort(cfg.Endpoint)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("gRPC driver requires the disposable loopback Collector endpoint")
	}
	if cfg.RunID == "" || len(cfg.Tokens) != 2 {
		return fmt.Errorf("gRPC driver requires a run ID and exactly two tenant credentials")
	}
	if cfg.ProfilesFile == "" {
		cfg.ProfilesFile = "../pkg/repository/testdata/current-profiles.pb"
	}
	raw, err := os.ReadFile(cfg.ProfilesFile)
	if err != nil {
		return fmt.Errorf("profile fixture unavailable")
	}
	profiles, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
	if err != nil {
		return err
	}
	rows, err := repository.CompactProfiles(profiles)
	if err != nil || len(rows) == 0 {
		return fmt.Errorf("profile fixture cannot be isolated")
	}
	conn, err := grpc.NewClient(cfg.Endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	service := "zzfuse-ingestion-grpc-" + cfg.RunID
	ids := make([]string, 0, len(cfg.Tokens))
	for id, token := range cfg.Tokens {
		if len(token) < 32 {
			return fmt.Errorf("test tenant token is missing")
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	profileIDs := map[string]string{}
	for _, id := range ids {
		other := ids[0]
		if other == id {
			other = ids[1]
		}
		now := pcommon.NewTimestampFromTime(time.Now().UTC().Truncate(time.Millisecond))
		logs := plog.NewLogs()
		rl := logs.ResourceLogs().AppendEmpty()
		rl.Resource().Attributes().PutStr("service.name", service)
		rl.Resource().Attributes().PutStr("service.namespace", other)
		log := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
		log.SetTimestamp(now)
		log.Body().SetStr("grpc " + id + " " + cfg.RunID)
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", service)
		rs.Resource().Attributes().PutStr("service.namespace", other)
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetName("grpc " + id + " " + cfg.RunID)
		span.SetStartTimestamp(now)
		span.SetEndTimestamp(now + 1_000_000)
		var traceID pcommon.TraceID
		var spanID pcommon.SpanID
		if _, err = rand.Read(traceID[:]); err != nil {
			return err
		}
		if _, err = rand.Read(spanID[:]); err != nil {
			return err
		}
		span.SetTraceID(traceID)
		span.SetSpanID(spanID)
		metrics := pmetric.NewMetrics()
		rm := metrics.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", service)
		rm.Resource().Attributes().PutStr("service.namespace", other)
		metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		metric.SetName("grpc_" + id)
		dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(now)
		dp.SetDoubleValue(42)
		pd := pprofile.NewProfiles()
		rows[0].Data.CopyTo(pd)
		rp := pd.ResourceProfiles().At(0)
		rp.Resource().Attributes().PutStr("service.name", service)
		rp.Resource().Attributes().PutStr("service.namespace", other)
		var profileID pprofile.ProfileID
		if _, err = rand.Read(profileID[:]); err != nil {
			return err
		}
		rp.ScopeProfiles().At(0).Profiles().At(0).SetProfileID(profileID)
		profileIDs[id] = hex.EncodeToString(profileID[:])
		sends := map[string]func(context.Context) error{
			"logs": func(c context.Context) error {
				response, e := plogotlp.NewGRPCClient(conn).Export(c, plogotlp.NewExportRequestFromLogs(logs))
				if e == nil && response.PartialSuccess().RejectedLogRecords() != 0 {
					return fmt.Errorf("logs partially rejected")
				}
				return e
			},
			"traces": func(c context.Context) error {
				response, e := ptraceotlp.NewGRPCClient(conn).Export(c, ptraceotlp.NewExportRequestFromTraces(traces))
				if e == nil && response.PartialSuccess().RejectedSpans() != 0 {
					return fmt.Errorf("traces partially rejected")
				}
				return e
			},
			"metrics": func(c context.Context) error {
				response, e := pmetricotlp.NewGRPCClient(conn).Export(c, pmetricotlp.NewExportRequestFromMetrics(metrics))
				if e == nil && response.PartialSuccess().RejectedDataPoints() != 0 {
					return fmt.Errorf("metrics partially rejected")
				}
				return e
			},
			"profiles": func(c context.Context) error {
				response, e := pprofileotlp.NewGRPCClient(conn).Export(c, pprofileotlp.NewExportRequestFromProfiles(pd))
				if e == nil && response.PartialSuccess().RejectedProfiles() != 0 {
					return fmt.Errorf("profiles partially rejected")
				}
				return e
			},
		}
		for signal, send := range sends {
			for _, token := range []string{"", "incorrect-test-ingestion-credential"} {
				attempt := ctx
				if token != "" {
					attempt = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
				}
				if err = send(attempt); status.Code(err) != codes.Unauthenticated {
					return fmt.Errorf("%s missing/wrong credential was not rejected with Unauthenticated", signal)
				}
			}
			valid := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+cfg.Tokens[id], "x-tenant-schema", other, "x-zzfuse-ingestion-receipt", "forged")
			if err = send(valid); err != nil {
				return fmt.Errorf("%s authenticated tenant %s failed with %s", signal, id, status.Code(err))
			}
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "grpc_auth_passed", "service_name": service, "tenants": ids, "records_per_tenant": map[string]int{"logs": 1, "spans": 1, "gauge_metrics": 1, "profiles": 1}, "profile_ids": profileIDs})
}
