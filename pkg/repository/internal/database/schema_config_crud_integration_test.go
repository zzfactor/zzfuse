//go:build integration

package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestSchemaConfigCRUDRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_SCHEMA_CONFIG_DSN")
	if os.Getenv("TRAZR_IT") != "1" || dsn == "" {
		t.Skip("requires TRAZR_IT=1 and TEST_SCHEMA_CONFIG_DSN for a disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := fmt.Sprintf("schema_config_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	repo := &SchemaConfigRepository{DB: db, schema: schema}
	// Initialize through the same default path used at Collector startup.
	if err := repo.EnsureTableAndDefault(ctx, "otel"); err != nil {
		t.Fatal(err)
	}
	defaultConfig, err := repo.GetByID(ctx, 1)
	if err != nil || defaultConfig == nil || !defaultConfig.Enabled {
		t.Fatalf("default telemetry schema disabled or missing: %+v %v", defaultConfig, err)
	}
	config := &DatabaseSchemaConfig{Name: "tenant_a", DatabaseSchema: "tenant_a", Enabled: false,
		Description: "tenant configuration", Priority: 10,
		Routing:  map[string]interface{}{"service_namespace": "tenant_a"},
		Metadata: map[string]interface{}{"label": "create"}}
	if err := repo.Upsert(ctx, config); err != nil {
		t.Fatal(err)
	}
	assertStored := func() {
		t.Helper()
		stored, err := repo.GetByID(ctx, config.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(stored, config) {
			t.Fatalf("stored config differs from returned config: got=%+v want=%+v", stored, config)
		}
	}
	assertStored()
	id, createdAt, updatedAt := config.ID, config.CreatedAt, config.UpdatedAt
	config.Name = "tenant_a_renamed"
	config.DatabaseSchema = "tenant_a_new"
	config.Enabled = true
	config.Priority = 11
	config.Routing = map[string]interface{}{"service_name": "new-service"}
	config.Metadata = map[string]interface{}{"label": "update"}
	if err := repo.Upsert(ctx, config); err != nil {
		t.Fatal(err)
	}
	assertStored()
	if config.ID != id || !config.CreatedAt.Equal(createdAt) || !config.UpdatedAt.After(updatedAt) {
		t.Fatal("update changed identity/creation time or failed to return updated timestamp")
	}
	if old, err := repo.GetByName(ctx, "tenant_a"); err != nil || old != nil {
		t.Fatal("rename left the old name", err)
	}
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if deleted, err := repo.GetByID(ctx, id); err != nil || deleted != nil {
		t.Fatal("deleted config remains", err)
	}
}
