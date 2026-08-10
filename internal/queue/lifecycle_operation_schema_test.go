package queue

import (
	"os"
	"strings"
	"testing"
)

func TestLifecycleOperationMigrationIsImmutableAndHashBound(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/033_lifecycle_operation_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE job_lifecycle_operations",
		"operation_id TEXT PRIMARY KEY",
		"command_sha256 TEXT NOT NULL",
		"command_payload JSONB NOT NULL",
		"result_job JSONB NOT NULL",
		"FOREIGN KEY (job_id, observed_generation)",
		"FOREIGN KEY (job_id, result_generation)",
		"FOREIGN KEY (job_id, observed_generation, step_id)",
		"prevent_job_lifecycle_operation_mutation",
		"job lifecycle operation records are immutable",
		"BEFORE TRUNCATE ON job_lifecycle_operations",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("lifecycle operation migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ON DELETE CASCADE", "ON CONFLICT"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("lifecycle operation migration contains forbidden fallback %q", forbidden)
		}
	}
}
