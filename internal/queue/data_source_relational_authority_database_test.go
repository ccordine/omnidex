package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func TestRelationalDataSourceMigrationPreservesConnectionAuthorityAndDropsRetiredProfileControls(t *testing.T) {
	ctx := context.Background()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "114")); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 123000000, time.UTC)
	updatedAt := createdAt.Add(2 * time.Hour)
	lastTestAt := createdAt.Add(time.Hour)
	catalogUpdatedAt := createdAt.Add(90 * time.Minute)
	legacy := DataSourceRecord{
		ID: "source-one", Name: "Exact source", Driver: "postgres",
		Host: "db.internal", Port: 5433, DatabaseName: "analytics", Username: "reader",
		Password: "exact secret", SSLMode: "require", UseDSN: true,
		DSN: "postgres://reader:exact@db.internal:5433/analytics", ReadOnly: true,
		LastTestStatus: "ok", LastTestMessage: "Exact retained result.", LastTestAt: &lastTestAt,
		CatalogUpdatedAt: &catalogUpdatedAt, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	legacyPayload := marshalLegacyDataSourceRecords(t, legacy)
	snapshot, err := datasource.NewSchemaSnapshot("source-one", "Exact source", []datasource.RelationDefinition{{
		Schema: "public", Name: "events", Kind: datasource.RelationTable,
		Columns: []datasource.ColumnDefinition{{
			Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: datasource.TypeInteger,
		}},
	}}, catalogUpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPayload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_settings(key,value) VALUES
		('data_sources',$1::jsonb),
		('data_source_catalog:source-one',$2::jsonb)
	`, legacyPayload, snapshotPayload); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "115")); err != nil {
		t.Fatal(err)
	}

	stored, err := repository.GetDataSource(ctx, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != legacy.ID || stored.Name != legacy.Name || stored.Driver != legacy.Driver ||
		stored.Host != legacy.Host || stored.Port != legacy.Port || stored.DatabaseName != legacy.DatabaseName ||
		stored.Username != legacy.Username || stored.Password != legacy.Password ||
		stored.DSN != legacy.DSN || stored.LastTestStatus != legacy.LastTestStatus ||
		stored.LastTestMessage != legacy.LastTestMessage || !stored.CreatedAt.Equal(createdAt) ||
		!stored.UpdatedAt.Equal(updatedAt) || stored.LastTestAt == nil || !stored.LastTestAt.Equal(lastTestAt) ||
		stored.CatalogUpdatedAt == nil || !stored.CatalogUpdatedAt.Equal(catalogUpdatedAt) {
		t.Fatalf("migrated source=%+v legacy=%+v", stored, legacy)
	}
	var retiredProfileColumns int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='data_sources'
		  AND column_name IN ('domain','context_prompt','privacy_mode')
	`).Scan(&retiredProfileColumns); err != nil {
		t.Fatal(err)
	}
	if retiredProfileColumns != 0 {
		t.Fatalf("retired data-source profile columns=%d", retiredProfileColumns)
	}
	storedSnapshot, ready, err := repository.GetDataSourceSchemaSnapshot(ctx, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ready || storedSnapshot.SourceID != snapshot.SourceID ||
		storedSnapshot.Fingerprint != snapshot.Fingerprint || len(storedSnapshot.Relations) != 1 {
		t.Fatalf("migrated schema snapshot=%+v ready=%t", storedSnapshot, ready)
	}
	var legacyRows int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM workspace_settings
		WHERE key='data_sources' OR key LIKE 'data_source_catalog:%'
	`).Scan(&legacyRows); err != nil {
		t.Fatal(err)
	}
	if legacyRows != 0 {
		t.Fatalf("legacy workspace authority rows=%d", legacyRows)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_settings(key,value) VALUES('data_sources','[]'::jsonb)
	`); err == nil || !strings.Contains(err.Error(), "workspace_settings_retired_data_source_authority_absent") {
		t.Fatalf("retired workspace authority write error=%v", err)
	}
}

func TestRelationalDataSourceMigrationRejectsInvalidLegacyAuthorityAtomically(t *testing.T) {
	ctx := context.Background()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "114")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	legacy := canonicalizeDataSourceRecord(DataSourceRecord{
		ID: "source-one", Name: "Valid source", Driver: "postgres", Host: "localhost", Port: 5432,
		DatabaseName: "fixture", Username: "reader", SSLMode: "prefer", ReadOnly: true,
		CreatedAt: now, UpdatedAt: now,
	})
	payload := marshalLegacyDataSourceRecords(t, legacy)
	retiredCatalog, err := json.Marshal(map[string]any{
		"source_id": "source-one", "source_name": "Valid source", "tables": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_settings(key,value) VALUES
		('data_sources',$1::jsonb),
		('data_source_catalog:source-one',$2::jsonb)
	`, payload, retiredCatalog); err != nil {
		t.Fatal(err)
	}
	err = repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "115"))
	if err == nil || !strings.Contains(err.Error(), "legacy schema snapshot data_source_catalog:source-one is invalid") {
		t.Fatalf("migration error=%v", err)
	}
	var tableCount, columnCount, settingCount, ledgerCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema=current_schema() AND table_name='data_sources'
	`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='ai_channels' AND column_name='data_source_id'
	`).Scan(&columnCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM workspace_settings
		WHERE key='data_sources' OR key LIKE 'data_source_catalog:%'
	`).Scan(&settingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM schema_migrations WHERE filename='115_data_source_relational_authority.sql'
	`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 || columnCount != 0 || settingCount != 2 || ledgerCount != 0 {
		t.Fatalf("rejected migration changed authority table=%d column=%d setting=%d ledger=%d",
			tableCount, columnCount, settingCount, ledgerCount)
	}
}

func TestBoundChatChannelSnapshotsImmutableRelationalDataSourceAuthority(t *testing.T) {
	ctx, repository := relationalDataSourceTestRepository(t)
	source, err := repository.CreateDataSource(ctx, DataSourceUpsert{
		Name: "Bound source", Driver: "postgres", Host: "localhost", Port: 5432,
		DatabaseName: "fixture", Username: "reader", SSLMode: "prefer",
	})
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "database-chat", Scope: model.ChannelScopeUser, Name: "Database chat",
		WorkspaceRoot: "/srv/workspaces/database-chat", DataSourceID: model.DataSourceID(source.ID),
		Mode: model.ChannelModeAssistant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if channel.DataSourceID != model.DataSourceID(source.ID) {
		t.Fatalf("channel=%+v source=%+v", channel, source)
	}
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Count the exact rows.")
	if err != nil {
		t.Fatal(err)
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		t.Fatal(err)
	}
	if binding.DataSourceID != channel.DataSourceID {
		t.Fatalf("turn metadata=%+v channel=%+v", binding, channel)
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE ai_channels SET data_source_id=NULL WHERE id=$1
	`, channel.ID); err == nil || !strings.Contains(err.Error(), "binding is immutable") {
		t.Fatalf("channel data-source rebinding error=%v", err)
	}
	if err := repository.DeleteDataSource(ctx, source.ID); err == nil {
		t.Fatal("bound data source was deleted")
	}
}

func TestRelationalDataSourceSchemaSnapshotLifecycleUsesOneAuthority(t *testing.T) {
	ctx, repository := relationalDataSourceTestRepository(t)
	source, err := repository.CreateDataSource(ctx, DataSourceUpsert{
		Name: "Catalog source", Driver: "postgres", Host: "localhost", Port: 5432,
		DatabaseName: "fixture", Username: "reader", SSLMode: "prefer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ready, err := repository.GetDataSourceSchemaSnapshot(ctx, source.ID); err != nil || ready {
		t.Fatalf("empty schema snapshot ready=%t err=%v", ready, err)
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE data_sources
		SET schema_catalog=jsonb_build_object('source_id',$2::text,'tables','[]'::jsonb)
		WHERE id=$1
	`, source.ID, source.ID); err == nil ||
		!strings.Contains(err.Error(), "data_sources_schema_snapshot_shape_check") {
		t.Fatalf("retired catalog shape error=%v", err)
	}
	snapshot, err := datasource.NewSchemaSnapshot(source.ID, source.Name, []datasource.RelationDefinition{{
		Schema: "public", Name: "events", Kind: datasource.RelationTable,
		Columns: []datasource.ColumnDefinition{{
			Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: datasource.TypeInteger,
		}},
	}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveDataSourceSchemaSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, ready, err := repository.GetDataSourceSchemaSnapshot(ctx, source.ID)
	if err != nil || !ready || loaded.Fingerprint != snapshot.Fingerprint || loaded.CapturedAt.IsZero() {
		t.Fatalf("loaded schema snapshot=%+v ready=%t err=%v", loaded, ready, err)
	}
	if err := repository.DeleteDataSourceSchemaSnapshot(ctx, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, ready, err := repository.GetDataSourceSchemaSnapshot(ctx, source.ID); err != nil || ready {
		t.Fatalf("deleted schema snapshot ready=%t err=%v", ready, err)
	}
}

func TestRelationalDataSourceCRUDMutatesOneExactRow(t *testing.T) {
	ctx, repository := relationalDataSourceTestRepository(t)
	created, err := repository.CreateDataSource(ctx, DataSourceUpsert{
		Name: "Original source", Driver: "postgres", Host: "original.internal", Port: 5432,
		DatabaseName: "original", Username: "reader", Password: "retained secret", SSLMode: "prefer",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repository.UpdateDataSource(ctx, created.ID, DataSourceUpsert{
		Name: "Updated source", Driver: "postgres", Host: "updated.internal", Port: 5433,
		DatabaseName: "updated", Username: "reader", Password: "", SSLMode: "prefer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != "Updated source" ||
		updated.Host != "updated.internal" || updated.Port != 5433 ||
		updated.Password != "retained secret" {
		t.Fatalf("updated source=%+v created=%+v", updated, created)
	}
	tested, err := repository.UpdateDataSourceTestResult(ctx, created.ID, " ok ", " exact result ")
	if err != nil {
		t.Fatal(err)
	}
	if tested.LastTestStatus != "ok" || tested.LastTestMessage != "exact result" || tested.LastTestAt == nil {
		t.Fatalf("test result source=%+v", tested)
	}
	if err := repository.DeleteDataSource(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetDataSource(ctx, created.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted source lookup error=%v", err)
	}
}

func relationalDataSourceTestRepository(t *testing.T) (context.Context, *Repository) {
	t.Helper()
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "117")); err != nil {
		t.Fatal(err)
	}
	return ctx, repository
}
