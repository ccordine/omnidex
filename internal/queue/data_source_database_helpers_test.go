package queue

import (
	"context"
	"testing"
)

func relationalDataSourceTestRepository(t *testing.T) (context.Context, *Repository) {
	t.Helper()
	ctx := t.Context()
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	return ctx, repository
}
