package queue

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPostgresLegacyPublicCatalogMatchesFrozenShape(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	tx, err := fixture.Pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `SET LOCAL search_path TO public,pg_catalog`); err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadLegacyCatalogSnapshot(t.Context(), tx, "public")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("legacy catalog sha256=%s shape=%d/%d/%d/%d/%d",
		snapshot.SHA256, snapshot.Tables, snapshot.Sequences, snapshot.Indexes,
		snapshot.Functions, snapshot.Triggers,
	)
	if snapshot.Tables != legacyExpectedTableCount ||
		snapshot.Sequences != legacyExpectedSequenceCount ||
		snapshot.Indexes != legacyExpectedIndexCount ||
		snapshot.Functions != legacyExpectedFunctionCount ||
		snapshot.Triggers != legacyExpectedTriggerCount {
		t.Fatalf("legacy shape=%+v differs from frozen audit", snapshot)
	}
	if snapshot.SHA256 != legacyExpectedCatalogSHA256 {
		t.Fatalf("legacy catalog sha256=%s want %s",
			snapshot.SHA256, legacyExpectedCatalogSHA256)
	}
	extensions, err := loadLegacyExtensions(t.Context(), tx)
	if err != nil {
		t.Fatal(err)
	}
	if got := legacyExtensionDescriptorSHA256(extensions); got != legacyExpectedExtensionSHA256 {
		t.Fatalf("legacy extension descriptors sha256=%s want %s",
			got, legacyExpectedExtensionSHA256)
	}
}
