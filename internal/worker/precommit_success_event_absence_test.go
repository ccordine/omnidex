package worker

import (
	"os"
	"strings"
	"testing"
)

func TestWorkersDoNotPublishDomainSuccessBeforeLifecycleCommit(t *testing.T) {
	for _, path := range []string{
		"objective_turn_runtime.go", "external_agent.go", "v3_coding_driver_verification.go",
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			`"objective_completed"`, `"external_agent_completed"`, `"coding_completed"`,
		} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("%s still publishes pre-commit success event %s", path, forbidden)
			}
		}
	}
}
