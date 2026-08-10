package queue

import (
	"os"
	"strings"
	"testing"
)

func TestWorkingSetReacquisitionMigrationExtendsOneEventSourcedAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/047_working_set_reacquisition.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"LOCK TABLE working_sets, working_set_items, working_set_events IN ACCESS EXCLUSIVE MODE",
		"ADD COLUMN reacquisition_count BIGINT",
		"UPDATE working_set_items",
		"working_set_items_reacquisition_count_check",
		"released item reactivation requires one exact reacquisition",
		"reacquisition count can advance only on released-to-resident transition",
		"ADD COLUMN reacquired_item_id TEXT",
		"working_set_events_reacquisition_metadata_check",
		"working_set_items_require_reacquisition_events",
		"working_set_events_require_reacquisition_item",
		"working-set item reacquisition has no exact immutable event history",
		"command_kind = 'reacquire' AND event_kind = 'reacquired'",
		"expected_reacquisition_count",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("working-set reacquisition migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"DROP CONSTRAINT working_set_items_working_set_id_ref_uri_ref_version_ref_relation_key",
		"DROP INDEX", "ON CONFLICT", "CREATE TABLE working_set_reacquisitions",
	} {
		if strings.Contains(strings.ToUpper(schema), strings.ToUpper(forbidden)) {
			t.Fatalf("working-set reacquisition migration introduced forbidden path %q", forbidden)
		}
	}
	base, err := os.ReadFile("../../migrations/030_working_sets.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(base), "UNIQUE (working_set_id, ref_uri, ref_version, ref_relation)") {
		t.Fatal("exact-reference uniqueness constraint is not present in the authoritative working-set schema")
	}
}

func TestWorkingSetQueuePersistsReacquisitionOnExistingItemRow(t *testing.T) {
	t.Parallel()
	load, err := os.ReadFile("working_set_load.go")
	if err != nil {
		t.Fatal(err)
	}
	diff, err := os.ReadFile("working_set_diff.go")
	if err != nil {
		t.Fatal(err)
	}
	apply, err := os.ReadFile("working_set_apply_tx.go")
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"load": string(load), "diff": string(diff), "apply": string(apply),
	} {
		if !strings.Contains(source, "reacquisition_count") {
			t.Fatalf("working-set %s path omits explicit reacquisition metadata", name)
		}
	}
	if strings.Contains(strings.ToUpper(string(diff)), "ON CONFLICT") {
		t.Fatal("working-set diff added an upsert that could hide duplicate item rows")
	}
}
