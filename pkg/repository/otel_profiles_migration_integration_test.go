//go:build integration

package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

// This integration test uses only an explicitly supplied disposable database,
// creates an isolated schema, and removes that schema on completion.
func TestProfileStorageMigrationAndRepositoryRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_PROFILE_MIGRATION_DSN")
	if os.Getenv("TRAZR_IT") != "1" || dsn == "" {
		t.Skip("requires TRAZR_IT=1 and TEST_PROFILE_MIGRATION_DSN for a disposable TimescaleDB database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("profile_migration_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	// Actual historical storage was already a hypertable. The older table lacks
	// service entity fields and all storage version/unsigned timing columns.
	oldDDL := fmt.Sprintf(`CREATE TABLE %s.profiles (
 id BIGSERIAL NOT NULL, timestamp_ns BIGINT NOT NULL,
 timestamp TIMESTAMPTZ GENERATED ALWAYS AS (to_timestamp(timestamp_ns / 1000000000.0)) STORED,
 service_name TEXT, profile_type TEXT NOT NULL, duration BIGINT, sample_type TEXT, sample_unit TEXT,
 profile_bytes BYTEA NOT NULL, attributes JSONB DEFAULT '{}', resource_attributes JSONB DEFAULT '{}',
 scope_name TEXT, scope_version TEXT, scope_attributes JSONB DEFAULT '{}', scope_dropped_attr_count INTEGER DEFAULT 0,
 metadata JSONB DEFAULT '{}', resource_schema_url TEXT, scope_schema_url TEXT, PRIMARY KEY(id,timestamp_ns));
 SELECT create_hypertable('%s.profiles','timestamp_ns',chunk_time_interval=>86400000000000,if_not_exists=>TRUE);`, schema, schema)
	if _, err := db.ExecContext(ctx, oldDDL); err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile("testdata/profiles-0.135/legacy-0.135-profile-row.pb")
	if err != nil {
		t.Fatal(err)
	}
	oldTimestamp := int64(1777777777123456789)
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.profiles(timestamp_ns,service_name,profile_type,duration,sample_type,sample_unit,profile_bytes,attributes,metadata) VALUES ($1,'legacy-service','cpu',9000000000,'cpu','nanoseconds',$2,'{"legacy":true}','{"legacy_note":"retain"}')`, schema), oldTimestamp, legacy); err != nil {
		t.Fatal(err)
	}
	var beforeCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+schema+".profiles").Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	// Seed real compressed legacy storage with the historical segment-by layout.
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s.profiles SET (timescaledb.compress,timescaledb.compress_segmentby='service_name,id',timescaledb.compress_orderby='timestamp_ns DESC'); SELECT compress_chunk(c) FROM show_chunks('%s.profiles') c;`, schema, schema)); err != nil {
		t.Fatal(err)
	}
	compressionLayout := func() string {
		t.Helper()
		var layout string
		if err := db.QueryRowContext(ctx, `SELECT segmentby FROM timescaledb_information.hypertable_columnstore_settings WHERE hypertable=$1::regclass`, schema+".profiles").Scan(&layout); err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(layout, " ", "")
	}
	legacyLayout := compressionLayout()
	if legacyLayout != "service_name,id" {
		t.Fatal("fixture lacks historical compression layout")
	}
	type policySnapshot struct {
		ID                      int
		Config, Schedule        string
		InitialStart, NextStart time.Time
		Scheduled               bool
	}
	retentionPolicy := func() policySnapshot {
		t.Helper()
		var p policySnapshot
		if err := db.QueryRowContext(ctx, `SELECT job_id,config::text,schedule_interval::text,initial_start,next_start,scheduled FROM timescaledb_information.jobs WHERE hypertable_schema=$1 AND hypertable_name='profiles' AND proc_name='policy_retention'`, schema).Scan(&p.ID, &p.Config, &p.Schedule, &p.InitialStart, &p.NextStart, &p.Scheduled); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Exercise production bootstrap, including the integer clock, then ensure a
	// repeated initialization preserves a pre-existing custom maintenance policy.
	initializer := &database.SchemaConfigRepository{DB: db}
	if err := initializer.EnsureOtelDataTables(ctx, db, schema); err != nil {
		t.Fatal(err)
	}
	firstPolicy := retentionPolicy()
	if firstPolicy.InitialStart.Before(time.Now().Add(23*time.Hour)) || !strings.Contains(firstPolicy.Config, "2592000000000000") {
		t.Fatal("new profile retention policy did not defer first run or retain 30-day default")
	}
	if _, err := db.ExecContext(ctx, `SELECT alter_job(job_id, schedule_interval=>INTERVAL '2 days',next_start=>now()+INTERVAL '3 days',config=>jsonb_set(config,'{drop_after}','3888000000000000')) FROM timescaledb_information.jobs WHERE job_id=$1`, firstPolicy.ID); err != nil {
		t.Fatal(err)
	}
	existingPolicy := retentionPolicy()
	if err := initializer.EnsureOtelDataTables(ctx, db, schema); err != nil {
		t.Fatal(err)
	}
	if repeated := retentionPolicy(); repeated != existingPolicy {
		t.Fatalf("existing retention config/schedule changed: before=%+v after=%+v", existingPolicy, repeated)
	}

	if actual := compressionLayout(); actual != legacyLayout {
		t.Fatalf("existing compression layout changed from %s to %s", legacyLayout, actual)
	}
	var compressedChunks int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM timescaledb_information.chunks WHERE hypertable_schema=$1 AND hypertable_name='profiles' AND is_compressed`, schema).Scan(&compressedChunks); err != nil || compressedChunks != 1 {
		t.Fatal("legacy compressed chunk was rewritten or lost", err)
	}
	var afterCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+schema+".profiles").Scan(&afterCount); err != nil {
		t.Fatal(err)
	}
	t.Logf("legacy physical rows before/after DDL: %d/%d", beforeCount, afterCount)
	repo := NewProfileRepository(db)
	start, end := time.Unix(0, 0), time.Now().Add(time.Hour)
	q := ProfileQuery{TimeRange: TimeRange{Start: &start, End: &end}, CommonBaseFilters: CommonBaseFilters{Schema: schema, Limit: 10, Direction: "asc"}}
	oldRows, _, _, err := repo.QueryProfiles(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldRows) != 1 || oldRows[0].ProfileStorageVersion != 0 || oldRows[0].ProfileEncoding != ProfileEncodingLegacy || oldRows[0].TimestampNS != oldTimestamp || oldRows[0].DurationNS != 9000000000 || !bytes.Equal(oldRows[0].ProfileBytes, legacy) || oldRows[0].ProfileSchemaVersion != nil || oldRows[0].OtelProfileID != nil {
		t.Fatalf("migration legacy rows count=%d, start=%s end=%s row=%+v", len(oldRows), start, end, func() interface{} {
			if len(oldRows) == 0 {
				return nil
			}
			e := oldRows[0]
			e.ProfileBytes = nil
			return e
		}())
	}
	oldResponse := toProfileEntryResponse(oldRows[0])
	if oldResponse.ProfileDecodeStatus != "legacy-opaque" || oldResponse.ProfileBytes != base64.StdEncoding.EncodeToString(legacy) {
		t.Fatal("legacy API bytes corrupted")
	}
	t.Setenv("SKIP_TEMPLATE_PROCESSING", "true")
	t.Setenv("SKIP_HASH_PROCESSING", "true")
	full := profileFixture(t, "current-profiles.pb")
	entries := []ProfileEntry{}
	for i := 0; i < full.ResourceProfiles().Len(); i++ {
		rp := full.ResourceProfiles().At(i)
		for j := 0; j < rp.ScopeProfiles().Len(); j++ {
			sp := rp.ScopeProfiles().At(j)
			for k := 0; k < sp.Profiles().Len(); k++ {
				row := pprofile.NewProfiles()
				r := row.ResourceProfiles().AppendEmpty()
				rp.Resource().CopyTo(r.Resource())
				r.SetSchemaUrl(rp.SchemaUrl())
				s := r.ScopeProfiles().AppendEmpty()
				sp.Scope().CopyTo(s.Scope())
				s.SetSchemaUrl(sp.SchemaUrl())
				sp.Profiles().At(k).CopyTo(s.Profiles().AppendEmpty())
				full.Dictionary().CopyTo(row.Dictionary())
				entry, err := ProfileEntryFromProfiles(row, time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
				entries = append(entries, entry)
			}
		}
	}
	ids, err := repo.InsertProfiles(ctx, entries, schema)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatal("modern rows not inserted")
	}
	rows, _, _, err := repo.QueryProfiles(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected legacy +3 current rows, got %d", len(rows))
	}
	byID := map[string]ProfileEntry{}
	for _, entry := range rows {
		if entry.OtelProfileID != nil {
			byID[*entry.OtelProfileID] = entry
		}
	}
	for _, expected := range entries {
		actual, ok := byID[*expected.OtelProfileID]
		if !ok {
			t.Fatal("source identity omitted from query")
		}
		if !bytes.Equal(actual.ProfileBytes, expected.ProfileBytes) || *actual.ProfileTimeUnixNano != *expected.ProfileTimeUnixNano || *actual.DurationNano != *expected.DurationNano || actual.TimestampNS != expected.TimestampNS || actual.DurationNS != expected.DurationNS || *actual.ServiceNamespace != *expected.ServiceNamespace || *actual.ServiceInstanceID != *expected.ServiceInstanceID {
			t.Fatal("database changed raw payload, exact unsigned fields or service context")
		}
		if _, err := DecodeStoredProfile(actual); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.EnsureProfilesTable(ctx, db, schema); err != nil {
		t.Fatal(err)
	}
	if repeated := retentionPolicy(); repeated != existingPolicy {
		t.Fatal("profile-only migration changed existing maintenance policy")
	}
	if compressionLayout() != legacyLayout {
		t.Fatal("profile-only migration changed compression layout")
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+schema+".profiles").Scan(&count); err != nil || count != 4 {
		t.Fatal("repeated migration changed stored rows", err)
	}
}
