package queue

import (
	"strings"
	"testing"
)

func TestDatabaseCognitionMigrationOwnsStationsAndRejectsPartialInstallation(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "115")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE database_evidence_receipts (id BIGINT)`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "116"))
	if err == nil || !strings.Contains(err.Error(), "database_evidence_receipts") {
		t.Fatalf("conflicting migration error=%v", err)
	}
	var ownsDatabaseWork bool
	if err := pool.QueryRow(ctx, `
		SELECT station_owns_portable_work(
			'database_query_intent','database_query_intent','{}'::jsonb
		)
	`).Scan(&ownsDatabaseWork); err != nil {
		t.Fatal(err)
	}
	var ledgerCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM schema_migrations
		WHERE filename='116_database_cognition_authority.sql'
	`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ownsDatabaseWork || ledgerCount != 0 {
		t.Fatalf("rejected migration changed station/ledger authority owns=%t ledger=%d", ownsDatabaseWork, ledgerCount)
	}
}

func TestDatabaseEvidenceReceiptPersistsOnlyBoundImmutableHashedAuthority(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "116")); err != nil {
		t.Fatal(err)
	}
	source := createDatabaseEvidenceSource(t, repository, "Bound source", "credential-password")
	other := createDatabaseEvidenceSource(t, repository, "Other source", "other-password")
	snapshot := databaseEvidenceSchemaSnapshot(t, source)
	if err := repository.SaveDataSourceSchemaSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	otherSnapshot := databaseEvidenceSchemaSnapshot(t, other)
	if err := repository.SaveDataSourceSchemaSnapshot(ctx, otherSnapshot); err != nil {
		t.Fatal(err)
	}
	jobID := seedDatabaseEvidenceBoundJob(t, repository, source.ID)
	assertDatabaseEvidenceJobBindingCannotChange(t, repository, jobID, other.ID)
	evidence := databaseEvidenceResultFixture(t, source.ID, snapshot.Fingerprint)
	receipt, err := repository.SaveDatabaseEvidenceReceipt(ctx, jobID, evidence)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := repository.SaveDatabaseEvidenceReceipt(ctx, jobID, evidence)
	if err != nil || duplicate.ID != receipt.ID {
		t.Fatalf("idempotent receipt=%+v original=%+v err=%v", duplicate, receipt, err)
	}
	var receiptCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM database_evidence_receipts WHERE job_id=$1`, jobID).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 1 {
		t.Fatalf("receipt count=%d want 1", receiptCount)
	}

	mismatched := evidence
	mismatched.Provenance.SourceID = other.ID
	mismatched.Provenance.SchemaFingerprint = otherSnapshot.Fingerprint
	if _, err := repository.SaveDatabaseEvidenceReceipt(ctx, jobID, mismatched); err == nil {
		t.Fatal("cross-source evidence receipt was persisted")
	}
	assertDatabaseEvidenceDirectInsertRejected(t, repository, jobID, other.ID, otherSnapshot.Fingerprint)

	for operation, statement := range map[string]string{
		"update":   `UPDATE database_evidence_receipts SET returned_rows=2 WHERE id=$1`,
		"delete":   `DELETE FROM database_evidence_receipts WHERE id=$1`,
		"truncate": `TRUNCATE database_evidence_receipts`,
	} {
		t.Run(operation, func(t *testing.T) {
			arguments := []any{receipt.ID}
			if operation == "truncate" {
				arguments = nil
			}
			_, err := pool.Exec(ctx, statement, arguments...)
			if err == nil || !strings.Contains(err.Error(), "database evidence receipts are immutable") {
				t.Fatalf("immutable %s error=%v", operation, err)
			}
		})
	}
	assertDatabaseEvidenceJobBindingCannotChange(t, repository, jobID, other.ID)

	assertDatabaseEvidenceReceiptHasNoExecutionOrCredentialPayload(t, repository, receipt.ID)
	assertDatabaseCognitionStationOwnership(t, repository)
}
