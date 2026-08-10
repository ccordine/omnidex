package cognition

import (
	"errors"
	"testing"
)

func TestCompletionCheckAndResultFailLoudly(t *testing.T) {
	t.Parallel()
	check := testCompletionCheck("check.goal")
	if err := check.Validate(); err != nil {
		t.Fatalf("validate completion check: %v", err)
	}
	invalidCheck := check
	invalidCheck.ID = "check\x00goal"
	if err := invalidCheck.Validate(); !errors.Is(err, ErrInvalidCompletionCheck) {
		t.Fatalf("invalid check error = %v, want ErrInvalidCompletionCheck", err)
	}
	if _, err := NewCompletionResult(
		"obligation-root", check, testRevision(1), CompletionSatisfied, nil,
	); !errors.Is(err, ErrInvalidCompletionResult) {
		t.Fatalf("evidenceless satisfaction error = %v, want ErrInvalidCompletionResult", err)
	}
	if _, err := NewCompletionResult(
		"obligation-root", check, testRevision(1), CompletionOutcome("unknown"), nil,
	); !errors.Is(err, ErrInvalidCompletionResult) {
		t.Fatalf("unknown outcome error = %v, want ErrInvalidCompletionResult", err)
	}
}

func TestCompletionResultCannotSatisfyAnotherObligationOrRevision(t *testing.T) {
	t.Parallel()
	evidence := testEvidenceRef(t)
	spec := testObligationSpec(t, "obligation-root", "")
	spec.SupportingRefs = []EvidenceRef{evidence}
	obligation := obligationFromSpec(spec, 1)
	obligation.Status = ObligationActive
	result, err := NewCompletionResult(
		obligation.ID, obligation.CompletionCheck, evidence.Revision,
		CompletionSatisfied, []EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongObligation := obligation
	wrongObligation.ID = "obligation-other"
	if err := result.ValidateFor(wrongObligation, evidence.Revision, []EvidenceRef{evidence}); !errors.Is(err, ErrInvalidCompletionResult) {
		t.Fatalf("obligation mismatch error = %v, want ErrInvalidCompletionResult", err)
	}
	newerRevision := evidence.Revision
	newerRevision.Number++
	if err := result.ValidateFor(obligation, newerRevision, []EvidenceRef{evidence}); !errors.Is(err, ErrInvalidCompletionResult) {
		t.Fatalf("revision mismatch error = %v, want ErrInvalidCompletionResult", err)
	}
}
