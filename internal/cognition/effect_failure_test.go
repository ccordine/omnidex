package cognition

import (
	"errors"
	"strings"
	"testing"
)

func testRegisteredAction(t *testing.T) RegisteredAction {
	t.Helper()
	schema := testActionSchema(t, EvidenceRequired)
	action, err := NewRegisteredAction(
		"action-1",
		testAttemptRef(),
		schema,
		ActionRequest{Kind: "inspect", Arguments: []ActionArgument{{Name: "target", Value: "entity-1"}}},
		[]EvidenceRef{testEvidenceRef(t)},
	)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func TestEffectIsPublicContentAddressedActionProvenance(t *testing.T) {
	t.Parallel()
	effect, err := NewEffect(
		"action-1", testRevision(2), EffectStateChanged,
		"The authorized target changed.",
	)
	if err != nil {
		t.Fatalf("new effect: %v", err)
	}
	if err := effect.Validate(); err != nil {
		t.Fatalf("validate effect: %v", err)
	}
	for name, mutate := range map[string]func(*Effect){
		"action":  func(value *Effect) { value.ActionID = "" },
		"kind":    func(value *Effect) { value.Kind = EffectKind("latent_predicate") },
		"NUL":     func(value *Effect) { value.Content = "bad\x00content" },
		"hash":    func(value *Effect) { value.ContentSHA256 = strings.Repeat("b", 64) },
		"bounded": func(value *Effect) { value.Content = strings.Repeat("x", MaxEffectBytes+1) },
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := effect
			mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidEffect) {
				t.Fatalf("error = %v, want ErrInvalidEffect", err)
			}
		})
	}
}

func TestActionFailureBindsActionAndUnchangedRevision(t *testing.T) {
	t.Parallel()
	action := testRegisteredAction(t)
	expected := testRevision(1)
	evidence := []EvidenceRef{testEvidenceRef(t)}
	failure, err := NewActionFailure(
		ActionFailurePreconditionFailed,
		action,
		expected,
		"The public precondition is not satisfied.",
		evidence,
	)
	if err != nil {
		t.Fatalf("new action failure: %v", err)
	}
	if err := failure.Validate(action, expected); err != nil {
		t.Fatalf("validate action failure: %v", err)
	}
	if !errors.Is(failure, ErrActionFailed) {
		t.Fatalf("errors.Is(%v, ErrActionFailed) = false", failure)
	}
	evidence[0].SHA256 = strings.Repeat("b", 64)
	if failure.EvidenceRefs[0].SHA256 == evidence[0].SHA256 {
		t.Fatal("action failure retained caller-owned evidence storage")
	}

	for name, mutate := range map[string]func(*ActionFailure){
		"code":     func(value *ActionFailure) { value.Code = ActionFailureCode("hidden_state") },
		"action":   func(value *ActionFailure) { value.ActionID = "action-2" },
		"revision": func(value *ActionFailure) { value.Revision.Number++ },
		"NUL":      func(value *ActionFailure) { value.PublicMessage = "bad\x00message" },
		"bounded": func(value *ActionFailure) {
			value.PublicMessage = strings.Repeat("x", MaxFailureMessageBytes+1)
		},
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := failure.Clone()
			mutate(&candidate)
			if err := candidate.Validate(action, expected); !errors.Is(err, ErrInvalidFailure) {
				t.Fatalf("error = %v, want ErrInvalidFailure", err)
			}
		})
	}
}
