package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestAblationTerminalAuthorityIsSingleAndExact(t *testing.T) {
	revision := cognition.WorldRevision{
		EpisodeID: cognition.EpisodeID("episode-" + strings.Repeat("a", 64)),
		Number:    1, SHA256: strings.Repeat("b", 64),
	}
	execution := ablationExecution{Revision: revision}
	if err := setPendingAblationTerminal(&execution, revision, "done", true, ""); err != nil {
		t.Fatal(err)
	}
	if err := setPendingAblationTerminal(&execution, revision, "done", true, ""); err == nil {
		t.Fatal("duplicate terminal authority was accepted")
	}
	execution.Outcome = Outcome{Terminal: true, GoalSatisfied: true, PublicOutcome: "changed"}
	if err := appendPendingAblationTerminal(nil, execution); err == nil ||
		!strings.Contains(err.Error(), "differs") {
		t.Fatalf("changed terminal error = %v", err)
	}
}
