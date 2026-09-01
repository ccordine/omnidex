package db_test

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/database"
)

func TestLifecycleGenerationTriggerUsesOneExplicitExpectedPurpose(t *testing.T) {
	setup := string(database.SetupSQL())
	start := strings.Index(setup, "CREATE FUNCTION require_lifecycle_generation_transition()")
	if start < 0 {
		t.Fatal("authoritative setup lacks lifecycle generation transition trigger function")
	}
	remainder := setup[start:]
	end := strings.Index(remainder, "\n$$;")
	if end < 0 {
		t.Fatal("lifecycle generation transition trigger function has no exact terminator")
	}
	body := remainder[:end]
	for _, required := range []string{
		"expected_transition_purpose TEXT;",
		"expected_transition_purpose := CASE",
		"transition_purpose IS DISTINCT FROM expected_transition_purpose OR",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("lifecycle generation transition trigger lacks %q", required)
		}
	}
	if strings.Contains(body, "IS DISTINCT FROM\n           CASE") {
		t.Fatal("lifecycle generation transition trigger retains ambiguous CASE/OR grammar")
	}
}
