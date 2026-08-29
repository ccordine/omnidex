package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresLegacyCutoverTailProofRestoresExactSequenceState(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	var runtimeSchema string
	if err := pool.QueryRow(t.Context(), `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE SEQUENCE tail_proof_sequence START WITH 41;
		SELECT setval('tail_proof_sequence',41,false);
	`); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	bundle := legacyTailProofTestBundle(
		"SELECT nextval('tail_proof_sequence');\n",
	)
	if err := proveLegacyCutoverTailApplicability(
		t.Context(), tx, runtimeSchema, bundle,
	); err != nil {
		t.Fatal(err)
	}
	var lastValue int64
	var isCalled bool
	if err := tx.QueryRow(t.Context(), `
		SELECT last_value,is_called FROM tail_proof_sequence
	`).Scan(&lastValue, &isCalled); err != nil {
		t.Fatal(err)
	}
	if lastValue != 41 || isCalled {
		t.Fatalf("tail proof sequence state=%d/%t, want 41/false", lastValue, isCalled)
	}
}

func TestPostgresLegacyCutoverTailProofFinishesCleanupAfterCallerCancellation(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	var runtimeSchema string
	if err := pool.QueryRow(t.Context(), `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE SEQUENCE canceled_tail_proof_sequence START WITH 71;
		SELECT setval('canceled_tail_proof_sequence',71,false);
	`); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	proofCtx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err = proveLegacyCutoverTailApplicability(
		proofCtx, tx, runtimeSchema, legacyTailProofTestBundle(
			"SELECT nextval('canceled_tail_proof_sequence');\nSELECT pg_sleep(0.2);\n",
		),
	)
	if err != nil {
		t.Fatalf("tail proof did not finish atomic cleanup after caller cancellation: %v", err)
	}
	if proofCtx.Err() == nil {
		t.Fatal("test proof context did not expire")
	}
	var lastValue int64
	var isCalled bool
	if err := tx.QueryRow(t.Context(), `
		SELECT last_value,is_called FROM canceled_tail_proof_sequence
	`).Scan(&lastValue, &isCalled); err != nil {
		t.Fatal(err)
	}
	if lastValue != 71 || isCalled {
		t.Fatalf("canceled tail proof sequence state=%d/%t, want 71/false", lastValue, isCalled)
	}
}

func TestPostgresLegacyCutoverTailProofEnforcesStandardConformingStrings(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	var runtimeSchema string
	if err := pool.QueryRow(t.Context(), `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), "SET LOCAL standard_conforming_strings TO off"); err != nil {
		t.Fatal(err)
	}
	body := "CREATE TABLE tail_escaped_string_commit_probe (id BIGINT);\n" +
		"SELECT 'a\\'b'; COMMIT; SELECT 'c\\'d';\n"
	err = proveLegacyCutoverTailApplicability(
		t.Context(), tx, runtimeSchema, legacyTailProofTestBundle(body),
	)
	if err == nil || !strings.Contains(err.Error(), "current-tail applicability failed at migration") {
		t.Fatalf("ambiguous tail string migration error=%v", err)
	}
	var relation *string
	if err := tx.QueryRow(t.Context(), `
		SELECT to_regclass('tail_escaped_string_commit_probe')::TEXT
	`).Scan(&relation); err != nil {
		t.Fatalf("tail proof did not leave its transaction usable: %v", err)
	}
	if relation != nil {
		t.Fatalf("ambiguous tail migration committed relation %q", *relation)
	}
	var enforced string
	if err := tx.QueryRow(t.Context(), "SHOW standard_conforming_strings").Scan(&enforced); err != nil {
		t.Fatal(err)
	}
	if enforced != "on" {
		t.Fatalf("tail proof standard_conforming_strings=%q, want on", enforced)
	}
}

func legacyTailProofTestBundle(body string) MigrationBundle {
	entries := make([]migrationBundleEntry, 0, legacyCutoverFinalMigrationPrefix+1)
	manifest := make([]byte, 0, 32*1024)
	for number := 1; number <= legacyCutoverFinalMigrationPrefix+1; number++ {
		name := fmt.Sprintf("%03d_tail_proof_test.sql", number)
		entryBody := []byte("SELECT 1;\n")
		if number > legacyCutoverFinalMigrationPrefix {
			entryBody = []byte(body)
		}
		digest := digestMigrationBytes(entryBody)
		entries = append(entries, migrationBundleEntry{
			name: name, sha256: digest, body: entryBody,
		})
		manifest = fmt.Appendf(manifest, "%s  %s\n", digest, name)
	}
	return MigrationBundle{
		manifestSHA256: digestMigrationBytes(manifest),
		manifest:       manifest,
		entries:        entries,
	}
}
