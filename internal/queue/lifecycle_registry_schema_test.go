package queue

import (
	"os"
	"strings"
	"testing"
)

func TestCrossAggregateLifecycleMigrationAddsTypedCancellation(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/036_scrum_channel_operations.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"'cancel_job'",
		"DROP CONSTRAINT job_lifecycle_operations_kind_check",
		"DROP CONSTRAINT job_lifecycle_operations_check1",
		"DROP CONSTRAINT job_lifecycle_operations_check3",
		"command_payload ?& ARRAY['operation_id', 'job_id', 'reason']",
		"result_job_status = 'canceled'",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("cross-aggregate lifecycle migration omitted %q", required)
		}
	}
}
