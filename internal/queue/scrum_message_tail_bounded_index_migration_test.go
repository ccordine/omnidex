package queue

import (
	"os"
	"strings"
	"testing"
)

const scrumMessageTailBoundedIndexMigration = "158_scrum_message_tail_bounded_index.sql"

func TestScrumMessageTailBoundedIndexMigrationRemovesGrowingPayload(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + scrumMessageTailBoundedIndexMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE scrum_card_messages IN SHARE ROW EXCLUSIVE MODE",
		"inherited Scrum message tail index differs",
		"substring(inherited_definition FROM 'USING btree .*$') IS DISTINCT FROM",
		"DROP INDEX scrum_card_messages_tail",
		"ON scrum_card_messages(project_id, card_id, ordinal DESC)",
		"INCLUDE ( message_id,role,created_at,source_created_at,timestamp_origin, status,operation_id,content_bytes )",
		"bounded Scrum message tail index was not installed exactly",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("bounded tail-index migration omitted %q", required)
		}
	}
	if strings.Contains(source,
		"INCLUDE ( message_id,role,content,created_at") {
		t.Fatal("bounded tail-index successor still stores growing content bytes")
	}
}

func TestPostgresScrumMessageTailBoundedIndexMigrationIsExactAndFailClosed(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "157")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		ALTER INDEX scrum_card_messages_tail SET (fillfactor=50)
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "158"))
	if err == nil || !strings.Contains(err.Error(), "inherited Scrum message tail index differs") {
		t.Fatalf("changed inherited tail-index error=%v", err)
	}
	var applied int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM schema_migrations WHERE filename=$1
	`, scrumMessageTailBoundedIndexMigration).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("rejected tail-index migration ledger rows=%d want 0", applied)
	}
}

func TestPostgresScrumMessageTailBoundedIndexHasExactProjection(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	var definition string
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_indexdef(to_regclass(current_schema()||'.scrum_card_messages_tail'))
	`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	want := "USING btree (project_id, card_id, ordinal DESC) INCLUDE (message_id, role, created_at, source_created_at, timestamp_origin, status, operation_id, content_bytes)"
	if !strings.Contains(definition, want) || strings.Contains(definition, "role, content, created_at") {
		t.Fatalf("bounded tail-index definition=%q", definition)
	}
}
