package repository

import (
	"context"
	"testing"
)

type tenantHistogramRepository struct {
	MetricRepository
	schema string
}

func (r *tenantHistogramRepository) InsertExponentialHistogramMetrics(_ context.Context, _ []ExponentialHistogramMetricEntry, schema string) ([]string, error) {
	r.schema = schema
	return []string{"1"}, nil
}

type tenantHistogramRouter struct {
	DatabaseRoutingSchemaInterface
	calls int
}

func (r *tenantHistogramRouter) GetSchema(string, string) (string, bool) {
	r.calls++
	return "tenant_b", true
}

func TestExponentialHistogramExplicitTenantOverridesProducerRouting(t *testing.T) {
	for _, explicit := range []string{"tenant_a", ""} {
		t.Run(map[bool]string{true: "authenticated_schema", false: "legacy_routing"}[explicit != ""], func(t *testing.T) {
			repo := &tenantHistogramRepository{}
			router := &tenantHistogramRouter{}
			writer := NewMetricWriterServiceWithEventBus(repo, nil, router)
			entries := []ExponentialHistogramMetricEntry{{ServiceName: "tenant-b-service", ResourceAttributes: []byte(`{"service.namespace":"tenant_b"}`)}}
			if _, err := writer.InsertExponentialHistogramMetrics(context.Background(), entries, explicit); err != nil {
				t.Fatal(err)
			}
			if explicit != "" {
				if repo.schema != "tenant_a" || router.calls != 0 {
					t.Fatalf("explicit tenant overridden: schema=%s routing_calls=%d", repo.schema, router.calls)
				}
			} else if repo.schema != "tenant_b" || router.calls != 1 {
				t.Fatal("legacy routing without an explicit schema changed")
			}
		})
	}
}
