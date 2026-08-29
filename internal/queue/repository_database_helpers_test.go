package queue

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func replanTestRepository(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	return repository, pool, ctx
}
