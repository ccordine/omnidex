package worker

import (
	"strings"
	"testing"
)

func TestServiceStateSchemaHasOneCanonicalTransactionalProjection(t *testing.T) {
	t.Parallel()
	statements := directCodingServiceStateSchemaStatements()
	if strings.Contains(statements, "BEGIN;") || strings.Contains(statements, "COMMIT;") {
		t.Fatal("canonical service-state schema body owns transaction mechanics")
	}
	if migration := phpServiceStateMigrationSQL(); migration != "BEGIN;\n"+statements+"COMMIT;\n" {
		t.Fatalf("generic migration is not an exact transaction over the canonical schema:\n%s", migration)
	}
	for _, marker := range []string{
		"CREATE TABLE IF NOT EXISTS " + directCodingServiceStateSchemaTable,
		"VALUES (TRUE, 1)",
		"CREATE TABLE IF NOT EXISTS " + directCodingServiceStateRecordTable,
		"PRIMARY KEY (state_scope, state_key)",
	} {
		if !strings.Contains(statements, marker) {
			t.Fatalf("canonical service-state schema omits %q", marker)
		}
	}
}
