package queue

import (
	"os"
	"strings"
	"testing"
)

func TestUnresolvedCognitionActionHasNoEpisodeOnlyAuthorityPath(t *testing.T) {
	raw, err := os.ReadFile("cognition_action_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "authority model.StepAttemptAuthority") ||
		!strings.Contains(source, "requireActiveStepAttemptTx(ctx, tx, authority)") ||
		!strings.Contains(source, "record.ActionFor(authority)") {
		t.Fatal("unresolved cognition action does not require and apply exact active attempt authority")
	}
}
