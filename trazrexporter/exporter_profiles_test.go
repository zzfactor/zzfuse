package trazrexporter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/pkg/repository/mocks"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/xexporter"
	"go.opentelemetry.io/collector/pdata/pprofile"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

type captureProfileWriter struct {
	entries []repository.ProfileEntry
	schemas []string
	groups  map[string][]repository.ProfileEntry
}

func (w *captureProfileWriter) InsertProfiles(_ context.Context, entries []repository.ProfileEntry, schema string) ([]string, error) {
	if w.groups == nil {
		w.groups = make(map[string][]repository.ProfileEntry)
	}
	w.groups[schema] = append(w.groups[schema], entries...)
	w.entries = append(w.entries, entries...)
	w.schemas = append(w.schemas, schema)
	return []string{"1"}, nil
}
func TestProfilesExporterPreservesContextDictionaryAndSource(t *testing.T) {
	raw, err := os.ReadFile("../pkg/repository/testdata/current-profiles.pb")
	if err != nil {
		t.Fatal(err)
	}
	pd, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Fresh strings must be interned only in storage copies, never caller data.
	rp := pd.ResourceProfiles().At(0)
	rp.Resource().Attributes().PutStr("fresh-resource-key", "fresh-resource-value")
	rp.ScopeProfiles().At(0).Scope().Attributes().PutStr("fresh-scope-key", "fresh-scope-value")
	beforeAttrs := rp.Resource().Attributes().AsRaw()
	beforeScope := rp.ScopeProfiles().At(0).Scope().Attributes().AsRaw()
	beforeStrings := append([]string(nil), pd.Dictionary().StringTable().AsRaw()...)
	writer := &captureProfileWriter{}
	exp := newProfilesExporter(zap.NewNop(), createDefaultConfig().(*Config))
	exp.profileWriter = writer
	ctx := authenticatedTestContext(t, &exp.baseExporter, "profile_test")
	exp.srm = &mocks.MockServiceResourceManager{GetSchemaFunc: func(name, ns string) (string, bool, error) {
		if name != "profile-review" || ns != "modern-team" {
			t.Fatal("resource routing context lost")
		}
		return "profile_test", true, nil
	}}
	if err = exp.pushProfileData(ctx, pd); err != nil {
		t.Fatal(err)
	}
	if len(writer.entries) != 3 || len(writer.schemas) != 1 || writer.schemas[0] != "profile_test" {
		t.Fatalf("unexpected writes %+v", writer)
	}
	if !reflect.DeepEqual(beforeAttrs, rp.Resource().Attributes().AsRaw()) || !reflect.DeepEqual(beforeScope, rp.ScopeProfiles().At(0).Scope().Attributes().AsRaw()) || !reflect.DeepEqual(beforeStrings, pd.Dictionary().StringTable().AsRaw()) {
		t.Fatal("exporter mutated input while interning strings")
	}
	for _, e := range writer.entries {
		stored, err := repository.DecodeStoredProfile(e)
		if err != nil {
			t.Fatal(err)
		}
		if stored.ProfileCount() != 1 || stored.Dictionary().MappingTable().Len() != pd.Dictionary().MappingTable().Len() || stored.Dictionary().FunctionTable().Len() != pd.Dictionary().FunctionTable().Len() || stored.Dictionary().StackTable().Len() != pd.Dictionary().StackTable().Len() || stored.Dictionary().AttributeTable().Len() != pd.Dictionary().AttributeTable().Len() {
			t.Fatal("shared dictionary tables discarded")
		}
		if e.ServiceNamespace == nil || *e.ServiceNamespace != "modern-team" || e.ServiceInstanceID == nil || *e.ServiceInstanceID != "modern-instance" {
			t.Fatal("service context omitted")
		}
		p := stored.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0)
		if p.Samples().At(0).Values().At(3) != 9007199254740993 || p.Samples().At(0).Values().At(2) != -5 || p.Samples().At(0).Values().At(1) != 0 || !bytes.Equal(p.OriginalPayload().AsRaw(), []byte{0, 1, 127, 128, 255, 10, 13}) {
			t.Fatal("sample values or original payload changed")
		}
	}
}
func TestProfilesExporterRejectsInvalidDictionaryBeforeWrites(t *testing.T) {
	raw, _ := os.ReadFile("../pkg/repository/testdata/current-profiles.pb")
	pd, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(1).Samples().At(0).SetLinkIndex(-1)
	writer := &captureProfileWriter{}
	exp := newProfilesExporter(zap.NewNop(), createDefaultConfig().(*Config))
	exp.profileWriter = writer
	ctx := authenticatedTestContext(t, &exp.baseExporter, "profile_test")
	exp.srm = &mocks.MockServiceResourceManager{GetSchemaFunc: func(string, string) (string, bool, error) { return "profile_test", true, nil }}
	if err := exp.pushProfileData(ctx, pd); err == nil || len(writer.entries) != 0 {
		t.Fatal("invalid batch was partially written")
	}
}
func TestFactorySupportsProfiles(t *testing.T) {
	factory, ok := NewFactory().(xexporter.Factory)
	if !ok {
		t.Fatal("factory does not expose profiles")
	}
	if factory.ProfilesStability() != component.StabilityLevelDevelopment {
		t.Fatal("profiles stability wrong")
	}
	cfg := factory.CreateDefaultConfig()
	set := exporter.Settings{ID: component.NewID(factory.Type()), TelemetrySettings: component.TelemetrySettings{Logger: zap.NewNop(), MeterProvider: metricnoop.NewMeterProvider(), TracerProvider: tracenoop.NewTracerProvider()}}
	exp, err := factory.CreateProfiles(context.Background(), set, cfg)
	if err != nil || exp == nil {
		t.Fatalf("create profiles: %v", err)
	}
}
func TestExporterQueueConfigurationOptional(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			cfg := createDefaultConfig().(*Config)
			c := confmap.NewFromStringMap(map[string]any{"sending_queue": map[string]any{"enabled": enabled, "num_consumers": 2, "queue_size": 3}})
			if err := c.Unmarshal(cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.QueueSettings.HasValue() != enabled {
				t.Fatal("queue enabled state ignored")
			}
			if enabled && cfg.QueueSettings.Get().QueueSize != 3 {
				b, _ := json.Marshal(cfg)
				t.Fatalf("queue overrides lost: %s", b)
			}
		})
	}
}

func TestProfilesExporterBindsSpoofedResourceLabelsToOneTenant(t *testing.T) {
	raw, _ := os.ReadFile("../pkg/repository/testdata/current-profiles.pb")
	pd, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	pd.ResourceProfiles().At(0).Resource().Attributes().PutStr("tenant.secret", "tenant-a-secret")
	pd.ResourceProfiles().At(1).Resource().Attributes().PutStr("service.namespace", "other-tenant")
	pd.ResourceProfiles().At(1).Resource().Attributes().PutStr("tenant.secret", "tenant-b-secret")
	pd.Dictionary().StringTable().Append("orphan-secret")
	writer := &captureProfileWriter{}
	exp := newProfilesExporter(zap.NewNop(), createDefaultConfig().(*Config))
	exp.profileWriter = writer
	ctx := authenticatedTestContext(t, &exp.baseExporter, "tenant_a")
	exp.srm = &mocks.MockServiceResourceManager{GetSchemaFunc: func(_, namespace string) (string, bool, error) {
		if namespace == "other-tenant" {
			return "tenant_b", true, nil
		}
		return "tenant_a", true, nil
	}}
	if err := exp.pushProfileData(ctx, pd); err != nil {
		t.Fatal(err)
	}
	if len(writer.entries) != 3 || len(writer.schemas) != 1 || len(writer.groups["tenant_a"]) != 3 || len(writer.groups["tenant_b"]) != 0 {
		t.Fatal("resource labels overrode authenticated tenant schema")
	}
	for _, entries := range writer.groups {
		for _, entry := range entries {
			decoded, err := repository.DecodeStoredProfile(entry)
			if err != nil {
				t.Fatal(err)
			}
			view, err := (&pprofile.JSONMarshaler{}).MarshalProfiles(decoded)
			if err != nil {
				t.Fatal(err)
			}
			foreign := "tenant-b-secret"
			own := "tenant-a-secret"
			if entry.ServiceNamespace != nil && *entry.ServiceNamespace == "other-tenant" {
				foreign, own = own, foreign
			}
			if bytes.Contains(view, []byte(foreign)) || bytes.Contains(view, []byte("orphan-secret")) || !bytes.Contains(view, []byte(own)) {
				t.Fatal("isolated dictionary leaked foreign/orphan strings or discarded local data")
			}
		}
	}
}
func TestProfilesExporterRejectsUnauthenticatedResourceBeforeWrites(t *testing.T) {
	raw, _ := os.ReadFile("../pkg/repository/testdata/current-profiles.pb")
	pd, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	pd.ResourceProfiles().At(1).Resource().Attributes().PutStr("service.namespace", "unrouted")
	writer := &captureProfileWriter{}
	exp := newProfilesExporter(zap.NewNop(), createDefaultConfig().(*Config))
	exp.profileWriter = writer
	_ = authenticatedTestContext(t, &exp.baseExporter, "tenant_a")
	exp.srm = &mocks.MockServiceResourceManager{GetSchemaFunc: func(_, namespace string) (string, bool, error) { return "tenant_a", namespace != "unrouted", nil }}
	if err := exp.pushProfileData(context.Background(), pd); err == nil || len(writer.entries) != 0 {
		t.Fatal("unauthenticated resource batch was partially written")
	}
}

func TestProfilesExporterOmitsForeignEmptyResourceDictionary(t *testing.T) {
	raw, err := os.ReadFile("../pkg/repository/testdata/current-profiles.pb")
	if err != nil {
		t.Fatal(err)
	}
	pd, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	foreign := pd.ResourceProfiles().At(1)
	foreign.Resource().Attributes().PutStr("service.namespace", "other-tenant")
	foreign.ScopeProfiles().At(0).Profiles().RemoveIf(func(pprofile.Profile) bool { return true })
	// This resource has no profiles, but the shared dictionary still contains
	// its resource-only interned string "extreme" from the complete fixture.
	writer := &captureProfileWriter{}
	exp := newProfilesExporter(zap.NewNop(), createDefaultConfig().(*Config))
	exp.profileWriter = writer
	ctx := authenticatedTestContext(t, &exp.baseExporter, "tenant_a")
	exp.srm = &mocks.MockServiceResourceManager{GetSchemaFunc: func(_, namespace string) (string, bool, error) {
		if namespace == "other-tenant" {
			return "tenant_b", true, nil
		}
		return "tenant_a", true, nil
	}}
	if err := exp.pushProfileData(ctx, pd); err != nil || len(writer.entries) != 2 || len(writer.groups["tenant_a"]) != 2 {
		t.Fatal("isolated profiles from populated tenant were not written", err)
	}
	for _, entry := range writer.entries {
		decoded, err := repository.DecodeStoredProfile(entry)
		if err != nil {
			t.Fatal(err)
		}
		view, err := (&pprofile.JSONMarshaler{}).MarshalProfiles(decoded)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(view, []byte("other-tenant")) || bytes.Contains(view, []byte("extreme")) {
			t.Fatal("empty foreign resource dictionary leaked")
		}
	}
	pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().RemoveIf(func(pprofile.Profile) bool { return true })
	if err := exp.pushProfileData(ctx, pd); err != nil || len(writer.entries) != 2 {
		t.Fatal("entirely profile-empty request should be a no-op", err)
	}
}
