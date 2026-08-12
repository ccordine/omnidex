package cognitiongauntlet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresOfflineInferenceRoleCannotReadPrivateHostOrOtherSchemas(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run offline process isolation tests")
	}
	config := offlinePromotionTestConfig(t, databaseURL)
	database, err := prepareOfflinePromotionDatabase(
		t.Context(), config, loadRepositoryMigrationBundle(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupOfflinePromotionDatabase(t, database) })
	assertInferenceFenceAuthority(t, database)
	hostStore, err := labyrinthhost.NewStoreInSchema(database.hostAdminPool, database.hostSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := hostStore.InstallSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	otherSchema, err := randomProcessIdentity("cognition_other_")
	if err != nil {
		t.Fatal(err)
	}
	otherID := pgx.Identifier{otherSchema}.Sanitize()
	if _, err := database.adminPool.Exec(t.Context(), "CREATE SCHEMA "+otherID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.adminPool.Exec(t.Context(), "REVOKE ALL ON SCHEMA "+otherID+" FROM PUBLIC"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.adminPool.Exec(
		t.Context(), "CREATE TABLE "+pgx.Identifier{otherSchema, "schema_migrations"}.Sanitize()+"(id INT)",
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.adminPool.Exec(context.Background(), "DROP SCHEMA "+otherID+" CASCADE")
	})
	if err := database.enableInference(t.Context()); err != nil {
		t.Fatal(err)
	}
	restricted, err := promotionPool(t.Context(), database.inferenceURL, database.schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := restricted.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	var currentRole string
	if err := restricted.QueryRow(t.Context(), `SELECT current_user`).Scan(&currentRole); err != nil {
		t.Fatal(err)
	}
	if currentRole != database.inferenceRole {
		t.Fatalf("restricted connection role=%q, want %q", currentRole, database.inferenceRole)
	}
	if err := queue.New(restricted).AuthorizeStepAttempt(t.Context(), database.attempt); err != nil {
		t.Fatalf("restricted role cannot execute production runtime authority: %v", err)
	}
	assertPermissionDenied(t, restricted, database.hostSchema, "episodes")
	assertPermissionDenied(t, restricted, otherSchema, "schema_migrations")
	restricted.Close()
	if err := database.revokeInference(t.Context()); err != nil {
		t.Fatal(err)
	}
	revoked, err := promotionPool(t.Context(), database.inferenceURL, database.schema)
	if err != nil {
		t.Fatal(err)
	}
	defer revoked.Close()
	if err := revoked.Ping(t.Context()); err == nil {
		t.Fatal("revoked inference login opened a new PostgreSQL session")
	}
}

func assertInferenceFenceAuthority(t *testing.T, database *offlinePromotionDatabase) {
	t.Helper()
	registry := pgx.Identifier{
		database.schema, "step_attempt_transaction_fence_authority",
	}.Sanitize()
	var authoritySchema string
	if err := database.adminPool.QueryRow(t.Context(),
		"SELECT authority_schema FROM "+registry+" WHERE singleton",
	).Scan(&authoritySchema); err != nil {
		t.Fatal(err)
	}
	var runtimeUsage, runtimeCreate, authorityUsage, authorityCreate bool
	var executableAuthorityFunctions int
	if err := database.adminPool.QueryRow(t.Context(), `SELECT
		has_schema_privilege($1,$2,'USAGE'),has_schema_privilege($1,$2,'CREATE'),
		has_schema_privilege($1,$3,'USAGE'),has_schema_privilege($1,$3,'CREATE'),
		(SELECT COUNT(*) FROM pg_proc procedures
		 JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
		 WHERE namespaces.nspname=$3 AND
		       has_function_privilege($1,procedures.oid,'EXECUTE'))
	`, database.inferenceRole, database.schema, authoritySchema).Scan(
		&runtimeUsage, &runtimeCreate, &authorityUsage, &authorityCreate,
		&executableAuthorityFunctions,
	); err != nil {
		t.Fatal(err)
	}
	if !runtimeUsage || runtimeCreate || !authorityUsage || authorityCreate ||
		executableAuthorityFunctions != 1 {
		t.Fatalf(
			"inference authority runtime=%t/%t fence=%t/%t functions=%d",
			runtimeUsage, runtimeCreate, authorityUsage, authorityCreate,
			executableAuthorityFunctions,
		)
	}
}

func loadRepositoryMigrationBundle(t *testing.T) queue.MigrationBundle {
	t.Helper()
	directory, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(directory, queue.MigrationManifestName))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	bundle, err := queue.LoadMigrationBundle(directory, hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func assertPermissionDenied(
	t *testing.T,
	pool *pgxpool.Pool,
	schema string,
	table string,
) {
	t.Helper()
	statement := "SELECT COUNT(*) FROM " + pgx.Identifier{schema, table}.Sanitize()
	_, err := pool.Exec(t.Context(), statement)
	var postgres *pgconn.PgError
	if !errors.As(err, &postgres) || postgres.Code != "42501" {
		t.Fatalf("qualified private read error=%v, want SQLSTATE 42501", err)
	}
}

func offlinePromotionTestConfig(t *testing.T, databaseURL string) OfflinePromotionConfig {
	t.Helper()
	privateDirectory := t.TempDir()
	if err := os.Chmod(privateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	publicDirectory := t.TempDir()
	generation := mustRatGeneration(t)
	runtimeFingerprint, err := currentRuntimeFingerprint(generation.Runtime.SourceSHA256)
	if err != nil {
		t.Fatal(err)
	}
	return OfflinePromotionConfig{
		Schema: OfflinePromotionConfigSchemaV2, DatabaseURL: databaseURL,
		OllamaEndpoint: "http://127.0.0.1:11434", InferenceTimeoutSeconds: 60,
		Scenario: mustOfflineScenarioSpec(t, SuiteRetrieve, 17_001),
		Variant:  VariantFullCognition, Surface: SurfaceSymbolic,
		RatGeneration: generation,
		PreparedBrainEvidence: testPreparedBrainEvidenceAuthority(
			t, generation.Fixed.Brain, publicDirectory,
		),
		RuntimeFingerprint: runtimeFingerprint, Repetition: 1,
		OmnidexCommit:         strings.Repeat("a", 40),
		PublicOutputDirectory: publicDirectory, PrivateOutputDirectory: privateDirectory,
		LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "context-projection.v1",
	}
}

func cleanupOfflinePromotionDatabase(t *testing.T, database *offlinePromotionDatabase) {
	t.Helper()
	ctx := context.Background()
	_ = database.revokeInference(ctx)
	_ = database.revokeHost(ctx)
	for _, role := range []string{database.inferenceRole, database.hostRole} {
		_, _ = database.adminPool.Exec(ctx, "DROP OWNED BY "+pgx.Identifier{role}.Sanitize())
	}
	var fenceSchema string
	_ = database.adminPool.QueryRow(ctx,
		`SELECT 'omnidex_host_authority_'||md5($1)`, database.schema,
	).Scan(&fenceSchema)
	for _, schema := range []string{database.schema, database.hostSchema, fenceSchema} {
		if schema != "" {
			_, _ = database.adminPool.Exec(ctx,
				"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
			)
		}
	}
	for _, role := range []string{database.inferenceRole, database.hostRole} {
		_, _ = database.adminPool.Exec(ctx, "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	}
	database.close()
}
