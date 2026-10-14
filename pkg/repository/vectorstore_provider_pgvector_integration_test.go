//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// The current project image and normal tenant/vector DDL suffice. No historical
// database.sql bootstrap or compatibility hash function is installed by the test.
func TestPGVectorCurrentDDLWriteAndBaseline(t *testing.T) {
	dsn := os.Getenv("TEST_PGVECTOR_WRITE_DSN")
	if os.Getenv("TRAZR_IT") != "1" || dsn == "" {
		t.Skip("requires TRAZR_IT=1 and TEST_PGVECTOR_WRITE_DSN for a disposable project database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var encoding string
	if err := db.QueryRowContext(ctx, "SHOW server_encoding").Scan(&encoding); err != nil || encoding != "UTF8" {
		t.Fatal("fixture must use normal UTF-8 storage", err)
	}
	var helperCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_proc WHERE proname='hash_content_sha256'`).Scan(&helperCount); err != nil || helperCount != 0 {
		t.Fatal("fixture must have no legacy hash helper", err)
	}
	content := "café🙂\r\nsecond line, a quoted value: ' telemetry"
	vector := []float32{0.12500003, -0.9876543, 1.9e-8, 0.000012345678}
	metadata := map[string]interface{}{"source": "real-vector-regression", "nested": map[string]interface{}{"unicode": "café🙂"}}
	initializer := &database.SchemaConfigRepository{DB: db}
	schemas := []string{fmt.Sprintf("vector_write_a_%d", time.Now().UnixNano()), fmt.Sprintf("vector_write_b_%d", time.Now().UnixNano())}
	for _, tenant := range schemas {
		if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+tenant); err != nil {
			t.Fatal(err)
		}
		defer func(schema string) {
			if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
				t.Error(err)
			}
		}(tenant)
		if err := initializer.EnsureOtelDataTables(ctx, db, tenant); err != nil {
			t.Fatal(err)
		}
	}
	// Keep tenant schemas off search_path. Only explicit table resolution is valid.
	if _, err := db.ExecContext(ctx, "SET search_path TO public"); err != nil {
		t.Fatal(err)
	}
	for i, tenant := range schemas {
		t.Run(tenant, func(t *testing.T) {
			store := &PGVectorCustomStore{db: db, schema: tenant, table: "regression_vectors", embeddingDimension: len(vector)}
			if err := store.EnsureTable(ctx); err != nil {
				t.Fatal(err)
			}
			timestampNS := time.Now().UnixNano()
			id, err := store.InsertEmbeddingRaw(ctx, timestampNS, vector, "vector-service", content, metadata)
			if err != nil || id != 1 {
				t.Fatalf("current-DDL write id=%d error=%v", id, err)
			}
			tableRef, err := database.BuildSafeTableReference(tenant, store.table)
			if err != nil {
				t.Fatal(err)
			}
			var storedHash, sqlHash, storedVector, storedContent, serviceName string
			var storedNS int64
			var storedMetadata []byte
			// PostgreSQL's built-in bytea hash provides an independent digest oracle
			// without installing pgcrypto or the historical wrapper function.
			query := fmt.Sprintf("SELECT timestamp_ns,service_name,content,content_hash,encode(pg_catalog.sha256(convert_to(content,'UTF8')),'hex'),embedding::text,metadata FROM %s WHERE id=$1", tableRef)
			if err := db.QueryRowContext(ctx, query, id).Scan(&storedNS, &serviceName, &storedContent, &storedHash, &sqlHash, &storedVector, &storedMetadata); err != nil {
				t.Fatal(err)
			}
			if storedNS != timestampNS || serviceName != "vector-service" || storedContent != content || storedHash != sqlHash {
				t.Fatal("stored vector identity/content/hash changed")
			}
			decoded, err := store.parsePGVectorString(storedVector)
			if err != nil || len(decoded) != len(vector) {
				t.Fatal("stored vector shape changed", err)
			}
			for j, want := range vector {
				if math.Float32bits(decoded[j]) != math.Float32bits(want) {
					t.Fatalf("float32 component %d changed: got %.10g want %.10g", j, decoded[j], want)
				}
			}
			var meta map[string]interface{}
			if err := json.Unmarshal(storedMetadata, &meta); err != nil || !reflect.DeepEqual(meta, metadata) {
				t.Fatal("stored metadata changed", err)
			}
			baseline, err := store.FindBaselineByContentHash(ctx, content)
			if err != nil || len(baseline) != 1 {
				t.Fatalf("tenant baseline rows=%d err=%v", len(baseline), err)
			}
			doc := baseline[0].(Document)
			if doc.ID != strconv.FormatInt(id, 10) || doc.PageContent != content || !reflect.DeepEqual(doc.Metadata, metadata) {
				t.Fatal("baseline identity/content/context changed")
			}
			if absent, err := store.FindBaselineByContentHash(ctx, content+" "); err != nil || len(absent) != 0 {
				t.Fatal("hash lookup normalized distinct content", err)
			}
			if i == 0 {
				foreign := &PGVectorCustomStore{db: db, schema: schemas[1], table: store.table, embeddingDimension: len(vector)}
				if err := foreign.EnsureTable(ctx); err != nil {
					t.Fatal(err)
				}
				if rows, err := foreign.FindBaselineByContentHash(ctx, content); err != nil || len(rows) != 0 {
					t.Fatal("tenant baseline query leaked another schema", err)
				}
			}
			if err := store.DeleteEmbedding(ctx, strconv.FormatInt(id, 10)); err != nil {
				t.Fatal(err)
			}
			if rows, err := store.FindBaselineByContentHash(ctx, content); err != nil || len(rows) != 0 {
				t.Fatal("deleted vector remains visible", err)
			}
		})
	}
}
