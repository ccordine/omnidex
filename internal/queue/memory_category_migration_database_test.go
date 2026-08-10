package queue

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresMigrationBackfillsTypedMemoryCategoriesExactlyOnce(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "061")); err != nil {
		t.Fatal(err)
	}

	var proceduralID, referenceID, categorizedID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO memory_chunks (content,kind) VALUES ('procedure','procedural') RETURNING id
	`).Scan(&proceduralID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO memory_chunks (content,kind) VALUES ('reference','reference') RETURNING id
	`).Scan(&referenceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO memory_chunks (content,kind) VALUES ('categorized','episodic') RETURNING id
	`).Scan(&categorizedID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO tags (name) VALUES
			('project:typed'),('category:pgsql'),('channel:scope'),('category:custom-skill')
			ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id,name
		)
		INSERT INTO memory_chunk_tags (memory_chunk_id,tag_id)
		SELECT $1,id FROM inserted
	`, proceduralID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		WITH category AS (
			INSERT INTO memory_categories (name) VALUES ('existing') RETURNING id
		)
		INSERT INTO memory_chunk_categories (memory_chunk_id,category_id)
		SELECT $1,id FROM category
	`, categorizedID); err != nil {
		t.Fatal(err)
	}

	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "062")); err != nil {
		t.Fatal(err)
	}
	assertMemoryCategoryNames(t, ctx, pool, proceduralID,
		[]string{"custom-skill", "database", "personal", "project", "strategy"})
	assertMemoryCategoryNames(t, ctx, pool, referenceID, []string{"research"})
	assertMemoryCategoryNames(t, ctx, pool, categorizedID, []string{"existing"})

	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "062")); err != nil {
		t.Fatal(err)
	}
	assertMemoryCategoryNames(t, ctx, pool, proceduralID,
		[]string{"custom-skill", "database", "personal", "project", "strategy"})
}

func assertMemoryCategoryNames(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	memoryID int64,
	want []string,
) {
	t.Helper()
	var got []string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(array_agg(categories.name ORDER BY categories.name),ARRAY[]::text[])
		FROM memory_chunk_categories memberships
		JOIN memory_categories categories ON categories.id=memberships.category_id
		WHERE memberships.memory_chunk_id=$1
	`, memoryID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("memory %d categories=%v want %v", memoryID, got, want)
	}
}
