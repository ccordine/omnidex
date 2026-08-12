package cognitiongauntlet

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func TestAblationCallJournalSealRequiresExactEvidence(t *testing.T) {
	journal := newAblationCallJournal()
	journal.order = append(journal.order, "call-missing-evidence")
	journal.attempts["call-missing-evidence"] = cognitionpolicy.CallAttempt{}
	journal.results["call-missing-evidence"] = cognitionpolicy.CallResult{}

	if _, err := journal.freeze(); err == nil ||
		!strings.Contains(err.Error(), "lacks exact call evidence") {
		t.Fatalf("seal missing evidence error = %v", err)
	}
}

func TestAblationCallJournalSealIsImmutable(t *testing.T) {
	journal := newAblationCallJournal()
	calls, err := journal.freeze()
	if err != nil {
		t.Fatalf("seal empty journal: %v", err)
	}
	if calls == nil || len(calls) != 0 {
		t.Fatalf("sealed calls = %#v, want explicit empty slice", calls)
	}

	if _, err := journal.Start(context.Background(), cognitionpolicy.CallAttempt{}); err == nil ||
		!strings.Contains(err.Error(), "sealed") {
		t.Fatalf("start after seal error = %v", err)
	}
	if err := journal.Finish(
		context.Background(), cognitionpolicy.CallAttempt{}, cognitionpolicy.CallResult{},
		cognitionpolicy.CallEvidence{},
	); err == nil || !strings.Contains(err.Error(), "sealed") {
		t.Fatalf("finish after seal error = %v", err)
	}
}
