package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func listAllWorkerLLMCallEvidence(
	ctx context.Context,
	repository *queue.Repository,
	jobID int64,
) ([]queue.LLMCallEvidence, error) {
	items := make([]queue.LLMCallEvidence, 0)
	afterID := int64(0)
	for {
		page, err := repository.ListLLMCallEvidenceForJob(
			ctx, jobID, afterID, queue.MaxLLMCallEvidencePageSize,
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

func freshWorkerEvidenceRepository(
	t *testing.T,
	databaseURL string,
) (*pgxpool.Pool, *queue.Repository) {
	t.Helper()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "omnidex_worker_evidence_test_" + hex.EncodeToString(nonce[:])
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatalf("install fresh worker evidence schema %q: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(
			cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		); err != nil {
			t.Errorf("drop worker evidence schema %q: %v", schema, err)
		}
		pool.Close()
	})
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return pool, queue.New(pool, authority)
}
