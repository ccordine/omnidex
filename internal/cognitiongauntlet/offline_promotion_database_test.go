package cognitiongauntlet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
		t.Context(), config, filepath.Join("..", "..", "migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupOfflinePromotionDatabase(t, database) })
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
	return OfflinePromotionConfig{
		Schema: OfflinePromotionConfigSchemaV1, DatabaseURL: databaseURL,
		OllamaEndpoint: "http://127.0.0.1:11434", InferenceTimeoutSeconds: 60,
		Spec: InitialMicrogauntletsV1()[0], Variant: VariantFullCognition, Surface: SurfaceSymbolic,
		RatGeneration: mustRatGeneration(t), RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
		PublicOutputDirectory: t.TempDir(), PrivateOutputDirectory: privateDirectory,
		LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "context-projection.v1",
	}
}

func cleanupOfflinePromotionDatabase(t *testing.T, database *offlinePromotionDatabase) {
	t.Helper()
	ctx := context.Background()
	_ = database.revokeInference(ctx)
	_, _ = database.adminPool.Exec(ctx, "DROP OWNED BY "+pgx.Identifier{database.inferenceRole}.Sanitize())
	_, _ = database.adminPool.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{database.schema}.Sanitize()+" CASCADE")
	_, _ = database.adminPool.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{database.hostSchema}.Sanitize()+" CASCADE")
	_, _ = database.adminPool.Exec(ctx, "DROP ROLE "+pgx.Identifier{database.inferenceRole}.Sanitize())
	database.close()
}
