package queue

import (
	"os"
	"strings"
	"testing"
)

const taskLedgerInlineExecutionMigration = "113_task_ledger_inline_execution.sql"

func TestTaskLedgerInlineExecutionMigrationIsNarrowAndTyped(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + taskLedgerInlineExecutionMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"ALTER TABLE task_nodes ADD COLUMN inline_execution boolean NOT NULL DEFAULT false",
		"ADD CONSTRAINT task_nodes_inline_execution_kind_check",
		"CHECK (NOT inline_execution OR kind = 'task')",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("inline-execution migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"working_set", "ON CONFLICT", "UPDATE task_nodes SET"} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("inline-execution migration introduced forbidden %q", forbidden)
		}
	}
}
