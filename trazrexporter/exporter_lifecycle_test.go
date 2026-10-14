package trazrexporter

import (
	"context"
	"errors"
	"testing"

	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/pkg/repository/mocks"
	"github.com/medxops/trazr-otel/trazrexporter/testutil"
	"github.com/medxops/trazr-otel/zzfuseauth"
	"go.opentelemetry.io/collector/component"
)

func TestExportersBorrowSharedServiceResourceManager(t *testing.T) {
	t.Parallel()
	shutdownCalls := 0
	manager := &mocks.MockServiceResourceManager{ShutdownFunc: func() error { shutdownCalls++; return nil }}
	readyCalls, managerCalls := 0, 0
	ready := false
	provider := &mocks.MockTrazrServiceProvider{
		WaitForReadyFunc: func(context.Context) error {
			readyCalls++
			ready = true
			return nil
		},
		GetServiceResourceManagerFunc: func() (repository.ServiceResourceManagerInterface, error) {
			if !ready {
				t.Fatal("exporter fetched services before readiness")
			}
			managerCalls++
			return manager, nil
		},
	}
	host := testutil.NewMockHost(provider)
	host.GetExtensions()[component.NewID(zzfuseauth.Type)] = testAuthenticator(t, "otel")
	for _, signal := range []string{"logs", "spans", "metrics", "profiles"} {
		ready = false
		exporter := &baseExporter{signalType: signal, cfg: createDefaultConfig().(*Config)}
		if err := exporter.initializeSRM(context.Background(), host); err != nil {
			t.Fatal(err)
		}
		if exporter.srm != manager {
			t.Fatalf("%s exporter did not borrow the shared instance", signal)
		}
		if err := exporter.shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if readyCalls != 4 || managerCalls != 4 || shutdownCalls != 0 {
		t.Fatalf("readiness %d, manager reads %d, manager shutdowns %d", readyCalls, managerCalls, shutdownCalls)
	}
}

func TestExporterInitializationFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("provider failure")
	for _, stage := range []string{"missing extension", "readiness", "manager", "nil manager", "canceled context"} {
		t.Run(stage, func(t *testing.T) {
			managerCalls := 0
			provider := &mocks.MockTrazrServiceProvider{
				WaitForReadyFunc: func(ctx context.Context) error {
					if stage == "readiness" {
						return failure
					}
					return ctx.Err()
				},
				GetServiceResourceManagerFunc: func() (repository.ServiceResourceManagerInterface, error) {
					managerCalls++
					if stage == "nil manager" {
						return nil, nil
					}
					return nil, failure
				},
			}
			var serviceProvider repository.TrazrServiceProvider = provider
			if stage == "missing extension" {
				serviceProvider = nil
			}
			ctx := context.Background()
			if stage == "canceled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			exporter := &baseExporter{cfg: createDefaultConfig().(*Config)}
			host := testutil.NewMockHost(serviceProvider)
			host.GetExtensions()[component.NewID(zzfuseauth.Type)] = testAuthenticator(t, "otel")
			err := exporter.initializeSRM(ctx, host)
			if err == nil || exporter.srm != nil {
				t.Fatal("initialization accepted unavailable services")
			}
			switch stage {
			case "missing extension":
				if !errors.Is(err, ErrTrazrExtensionNotFound) {
					t.Fatal(err)
				}
			case "readiness", "manager":
				if !errors.Is(err, failure) {
					t.Fatal(err)
				}
			case "canceled context":
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
			if (stage == "missing extension" || stage == "readiness" || stage == "canceled context") && managerCalls != 0 {
				t.Fatal("manager accessed before provider readiness")
			}
		})
	}
}
