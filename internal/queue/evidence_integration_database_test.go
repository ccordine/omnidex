package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func evidenceDatabaseURL(t *testing.T) string {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for isolated PostgreSQL evidence coverage")
	}
	return databaseURL
}

func recordExactLLMEvidenceFixture(
	ctx context.Context,
	repository *Repository,
	record exactLLMEvidenceFixtureRecord,
) (LLMCallEvidence, error) {
	opening, err := repository.ReserveLLMCallEvidence(ctx, record.LLMCallOpeningRecord)
	if err != nil {
		return LLMCallEvidence{}, err
	}
	return repository.FinalizeLLMCallEvidence(ctx, record.receipt(opening.ID))
}

func listAllLLMCallEvidenceForJob(
	ctx context.Context,
	repository *Repository,
	jobID int64,
) ([]LLMCallEvidence, error) {
	items := make([]LLMCallEvidence, 0)
	afterID := int64(0)
	for {
		page, err := repository.ListLLMCallEvidenceForJob(
			ctx, jobID, afterID, MaxLLMCallEvidencePageSize,
		)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return items, nil
		}
		items = append(items, page...)
		afterID = page[len(page)-1].ID
	}
}

func freshEvidenceRepository(
	t *testing.T,
	databaseURL string,
) (*pgxpool.Pool, *Repository) {
	t.Helper()
	schema := "omnidex_evidence_test_" + evidenceNonce(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatalf("install fresh evidence schema %q: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop evidence test schema %q: %v", schema, err)
		}
		pool.Close()
	})
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return pool, New(pool, authority)
}

func evidenceNonce(t *testing.T) string {
	t.Helper()
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value[:])
}
