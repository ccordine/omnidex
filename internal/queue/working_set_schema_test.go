package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/workingset"
)

func TestWorkingSetMigrationDefinesOneNormalizedGenerationOwnedAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/030_working_sets.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"CREATE TABLE working_sets",
		"CREATE TABLE working_set_items",
		"CREATE TABLE working_set_memberships",
		"CREATE TABLE working_set_closed_scopes",
		"CREATE TABLE working_set_events",
		"REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT",
		"REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT",
		"REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT",
		"REFERENCES working_set_items(working_set_id, job_id, generation, item_id)",
		"UNIQUE (job_id, generation)",
		"UNIQUE (working_set_id, working_set_version)",
		"UNIQUE (command_id)",
		"payload JSON NOT NULL",
		"(payload ->> 'working_set_id') IS NOT DISTINCT FROM working_set_id",
		"IS NOT DISTINCT FROM working_set_version - 1",
		"command_kind = 'acquire' AND event_kind = 'acquired'",
		"command_kind = 'close_scope' AND event_kind = 'scope_closed'",
		"actor TEXT NOT NULL CHECK (actor = 'code')",
		"max_items BETWEEN 1 AND 4096",
		"max_bytes BETWEEN 1 AND 67108864",
		"working_sets_identity_guard",
		"working_set_items_identity_guard",
		"working_set_closed_scopes_immutable",
		"prevent_working_set_history_truncate",
		"prevent_working_set_event_mutation",
		"BEFORE UPDATE OR DELETE ON working_set_events",
		"BEFORE TRUNCATE ON working_set_events",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("working-set migration omitted %q", required)
		}
	}
	for _, value := range []string{
		string(workingset.StatusActive), string(workingset.StatusClosed),
		string(workingset.ItemResident), string(workingset.ItemReleased), string(workingset.ItemInvalidated),
		string(workingset.CommandAcquire), string(workingset.CommandRetain),
		string(workingset.CommandRelease), string(workingset.CommandTouch),
		string(workingset.CommandInvalidateStale), string(workingset.CommandCloseScope),
		string(workingset.EventAcquired), string(workingset.EventRetained),
		string(workingset.EventReleased), string(workingset.EventTouched),
		string(workingset.EventInvalidatedStale), string(workingset.EventScopeClosed),
	} {
		if !strings.Contains(schema, "'"+value+"'") {
			t.Errorf("working-set migration rejects registered value %q", value)
		}
	}
}

func TestWorkingSetQueueHasNoSnapshotOrUpsertFallback(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("working_set*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		source := strings.ToUpper(string(raw))
		if strings.Contains(source, "ON CONFLICT") {
			t.Fatalf("%s introduced a working-set upsert fallback", name)
		}
		if strings.Contains(source, "SNAPSHOT JSON") || strings.Contains(source, "SNAPSHOT JSONB") {
			t.Fatalf("%s introduced a duplicate snapshot authority", name)
		}
	}
	applyRaw, err := os.ReadFile("working_set_apply_tx.go")
	if err != nil {
		t.Fatal(err)
	}
	apply := string(applyRaw)
	jobLock := strings.Index(apply, "loadTaskLedgerHeaderTx")
	setLock := strings.Index(apply, "loadWorkingSetSnapshotTx")
	if jobLock < 0 || setLock < 0 || jobLock >= setLock {
		t.Fatal("working-set command lock order is not job/task-ledger then working set")
	}
	eventsRaw, err := os.ReadFile("working_set_events.go")
	if err != nil {
		t.Fatal(err)
	}
	events := string(eventsRaw)
	for _, required := range []string{
		"maxWorkingSetEventPageSize = 100", "id>$4", "ORDER BY id ASC LIMIT $5",
	} {
		if !strings.Contains(events, required) {
			t.Fatalf("bounded working-set event history omitted %q", required)
		}
	}
}
