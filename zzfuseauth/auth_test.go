package zzfuseauth

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/ptrace"
	request "go.opentelemetry.io/collector/pdata/xpdata/request"
)

func fixture() *Config {
	return &Config{SigningKey: configopaque.String(strings.Repeat("test-key-", 5)), Credentials: map[string]Credential{
		"tenant_a": {Token: configopaque.String(strings.Repeat("test-a-", 5)), Schema: "tenant_a", Epoch: 1},
		"tenant_b": {Token: configopaque.String(strings.Repeat("test-b-", 5)), Schema: "tenant_b", Epoch: 1},
	}}
}

func TestAuthenticateAndRejectSpoofedMetadata(t *testing.T) {
	a, err := New(fixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tenant_a", "tenant_b"} {
		ctx := client.NewContext(context.Background(), client.Info{Metadata: client.NewMetadata(map[string][]string{
			ReceiptMetadataKey: {"forged-tenant-b"}, "authorization": {"secret"}, "x-tenant-schema": {"tenant_b"},
		})})
		ctx, err = a.Authenticate(ctx, map[string][]string{"Authorization": {"Bearer " + string(fixture().Credentials[id].Token)}})
		if err != nil {
			t.Fatal(err)
		}
		tenant, err := a.ResolveTenant(ctx)
		if err != nil || tenant.Schema != id {
			t.Fatalf("wrong tenant: %+v %v", tenant, err)
		}
		info := client.FromContext(ctx)
		if len(info.Metadata.Get("authorization")) != 0 || len(info.Metadata.Get("x-tenant-schema")) != 0 {
			t.Fatal("producer metadata retained")
		}
		if strings.Contains(info.Metadata.Get(ReceiptMetadataKey)[0], string(fixture().Credentials[id].Token)) {
			t.Fatal("raw token in receipt")
		}
	}
	for _, sources := range []map[string][]string{nil, {"Authorization": {"Bearer incorrect"}}, {"Authorization": {"Basic test"}}, {"Authorization": {"Bearer " + string(fixture().Credentials["tenant_a"].Token), "Bearer duplicate"}}, {"Authorization": {"Bearer " + string(fixture().Credentials["tenant_a"].Token)}, "authorization": {"Bearer duplicate"}}} {
		if _, err := a.Authenticate(context.Background(), sources); err == nil {
			t.Fatal("invalid authentication accepted")
		}
	}
	for _, marker := range []string{"", "tenant_b", "e30.Zm9yZ2Vk"} {
		ctx := client.NewContext(context.Background(), client.Info{Metadata: client.NewMetadata(map[string][]string{ReceiptMetadataKey: {marker}})})
		if _, err := a.ResolveTenant(ctx); err == nil {
			t.Fatal("forged marker accepted")
		}
	}
}

func TestPersistentRequestContextsAllSignalsAndRotation(t *testing.T) {
	cfg := fixture()
	a, _ := New(cfg)
	ctx, err := a.Authenticate(context.Background(), map[string][]string{"authorization": {"Bearer " + string(cfg.Credentials["tenant_a"].Token)}})
	if err != nil {
		t.Fatal(err)
	}
	encodings := map[string]func() (context.Context, []byte, error){
		"logs": func() (context.Context, []byte, error) {
			b, e := request.MarshalLogs(ctx, plog.NewLogs())
			if e != nil {
				return nil, b, e
			}
			c, _, e := request.UnmarshalLogs(b)
			return c, b, e
		},
		"traces": func() (context.Context, []byte, error) {
			b, e := request.MarshalTraces(ctx, ptrace.NewTraces())
			if e != nil {
				return nil, b, e
			}
			c, _, e := request.UnmarshalTraces(b)
			return c, b, e
		},
		"metrics": func() (context.Context, []byte, error) {
			b, e := request.MarshalMetrics(ctx, pmetric.NewMetrics())
			if e != nil {
				return nil, b, e
			}
			c, _, e := request.UnmarshalMetrics(b)
			return c, b, e
		},
		"profiles": func() (context.Context, []byte, error) {
			b, e := request.MarshalProfiles(ctx, pprofile.NewProfiles())
			if e != nil {
				return nil, b, e
			}
			c, _, e := request.UnmarshalProfiles(b)
			return c, b, e
		},
	}
	for signal, roundtrip := range encodings {
		t.Run(signal, func(t *testing.T) {
			restored, bytes, err := roundtrip()
			if err != nil {
				t.Fatal(err)
			}
			if client.FromContext(restored).Auth != nil {
				t.Fatal("test did not reproduce standard queue Auth loss")
			}
			if strings.Contains(string(bytes), string(cfg.Credentials["tenant_a"].Token)) {
				t.Fatal("raw token serialized")
			}
			tenant, err := a.ResolveTenant(restored)
			if err != nil || tenant.Schema != "tenant_a" {
				t.Fatal("WAL context identity lost", err)
			}
			if _, err := a.ResolveIngressTenant(restored); err == nil {
				t.Fatal("queue receipt authorized fresh ingestion without client authentication")
			}
			rotated := fixture()
			entry := rotated.Credentials["tenant_a"]
			entry.Token = configopaque.String(strings.Repeat("rotated-test-", 4))
			rotated.Credentials["tenant_a"] = entry
			fresh, _ := New(rotated)
			if _, err = fresh.ResolveTenant(restored); err != nil {
				t.Fatal("token rotation invalidated admitted queue identity", err)
			}
			if _, err = fresh.Authenticate(context.Background(), map[string][]string{"authorization": {"Bearer " + string(cfg.Credentials["tenant_a"].Token)}}); err == nil {
				t.Fatal("old API token survived rotation")
			}
			entry.Epoch++
			rotated.Credentials["tenant_a"] = entry
			fresh, _ = New(rotated)
			if _, err = fresh.ResolveTenant(restored); err == nil {
				t.Fatal("revoked receipt epoch accepted")
			}
			delete(rotated.Credentials, "tenant_a")
			fresh, _ = New(rotated)
			if _, err = fresh.ResolveTenant(restored); err == nil {
				t.Fatal("removed credential receipt accepted")
			}
			rotated = fixture()
			rotated.SigningKey = configopaque.String(strings.Repeat("new-key-", 5))
			fresh, _ = New(rotated)
			if _, err = fresh.ResolveTenant(restored); err == nil {
				t.Fatal("old signing key receipt accepted")
			}
		})
	}
}

func TestIngressRequiresMatchingPrivateAuthenticationMarker(t *testing.T) {
	a, _ := New(fixture())
	authenticate := func(id string) context.Context {
		ctx, err := a.Authenticate(context.Background(), map[string][]string{"authorization": {"Bearer " + string(fixture().Credentials[id].Token)}})
		if err != nil {
			t.Fatal(err)
		}
		return ctx
	}
	ctxA, ctxB := authenticate("tenant_a"), authenticate("tenant_b")
	if tenant, err := a.ResolveIngressTenant(ctxA); err != nil || tenant.Schema != "tenant_a" {
		t.Fatalf("authenticated ingress rejected: %+v %v", tenant, err)
	}
	info := client.FromContext(ctxA)
	info.Metadata = client.FromContext(ctxB).Metadata
	if _, err := a.ResolveIngressTenant(client.NewContext(context.Background(), info)); err == nil {
		t.Fatal("tenant_b receipt accepted with tenant_a authentication marker")
	}
	other, _ := New(fixture())
	if _, err := other.ResolveIngressTenant(ctxA); err == nil {
		t.Fatal("another authenticator's private marker accepted")
	}
}
