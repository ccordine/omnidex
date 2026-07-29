package queue

import (
	"os"
	"strings"
	"testing"
)

func TestFeedbackTransportDoesNotInterpretApprovalPhrases(t *testing.T) {
	raw, err := os.ReadFile("repository_step_input.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"isExplicitApprovalFeedback",
		"yes, proceed",
		"i approve",
		"APPROVE: <notes>",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("feedback transport contains forbidden phrase interpretation %q", forbidden)
		}
	}
}
