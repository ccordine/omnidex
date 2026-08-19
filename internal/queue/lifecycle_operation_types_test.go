package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestLifecycleOperationIdentityAndContentAreIndependentAuthorities(t *testing.T) {
	id, err := NewLifecycleOperationID("test", "job-41", "replan")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLifecycleOperationID(string(id)); err != nil {
		t.Fatal(err)
	}
	command := ReplanJobCommand{OperationID: id, JobID: 41, Feedback: "Original feedback"}
	descriptor, err := describeLifecycleOperation(id, LifecycleReplanJob, command)
	if err != nil {
		t.Fatal(err)
	}
	changed := command
	changed.Feedback = "Changed feedback"
	changedDescriptor, err := describeLifecycleOperation(id, LifecycleReplanJob, changed)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.ID != changedDescriptor.ID || descriptor.SHA256 == changedDescriptor.SHA256 {
		t.Fatalf("identity/content binding original=%+v changed=%+v", descriptor, changedDescriptor)
	}
}

func TestRoleplayCompletionKnowledgeRecipientsAreExactAndBounded(t *testing.T) {
	id, err := NewLifecycleOperationID("roleplay-recipient-validation")
	if err != nil {
		t.Fatal(err)
	}
	authority := model.StepAttemptAuthority{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker",
	}
	character := model.RoleplayCharacterID("rpc_0123456789abcdef0123456789abcdef")
	base := CompleteStepCommand{
		OperationID: id, Authority: authority, StepID: 1, Output: "response",
		ContextKey: "objective_result", ContextValue: "result",
		RoleplayFacts: []string{"A new fictional fact."},
	}

	valid := base
	valid.RoleplayKnowledgeCharacterIDs = []model.RoleplayCharacterID{character}
	if _, err := normalizeCompleteStepCommand(valid); err != nil {
		t.Fatalf("valid recipient rejected: %v", err)
	}

	withoutFacts := base
	withoutFacts.RoleplayFacts = nil
	withoutFacts.RoleplayKnowledgeCharacterIDs = []model.RoleplayCharacterID{character}
	if _, err := normalizeCompleteStepCommand(withoutFacts); err == nil {
		t.Fatal("knowledge recipient without new canon facts was accepted")
	}

	duplicated := base
	duplicated.RoleplayKnowledgeCharacterIDs = []model.RoleplayCharacterID{character, character}
	if _, err := normalizeCompleteStepCommand(duplicated); err == nil {
		t.Fatal("duplicated knowledge recipient was accepted")
	}

	overBound := base
	overBound.RoleplayKnowledgeCharacterIDs = make(
		[]model.RoleplayCharacterID, roleplay.MaxKnowledgeRecipientsPerTurn+1,
	)
	for index := range overBound.RoleplayKnowledgeCharacterIDs {
		overBound.RoleplayKnowledgeCharacterIDs[index] = character
	}
	if _, err := normalizeCompleteStepCommand(overBound); err == nil {
		t.Fatal("over-bound knowledge recipient list was accepted")
	}
}

func TestLifecycleOperationCommandsRejectMissingOrInvalidAuthority(t *testing.T) {
	validID, err := NewLifecycleOperationID("test", "validation")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		run  func() error
	}{
		{name: "missing identity", run: func() error {
			_, err := describeLifecycleOperation("", LifecycleReplanJob, ReplanJobCommand{})
			return err
		}},
		{name: "nul output", run: func() error {
			_, err := normalizeCompleteStepCommand(CompleteStepCommand{OperationID: validID, StepID: 1, Output: "bad\x00output"})
			return err
		}},
		{name: "context without key", run: func() error {
			_, err := normalizeCompleteStepCommand(CompleteStepCommand{OperationID: validID, StepID: 1, ContextValue: "orphan"})
			return err
		}},
		{name: "roleplay facts without objective completion", run: func() error {
			_, err := normalizeCompleteStepCommand(CompleteStepCommand{
				OperationID: validID, StepID: 1, RoleplayFacts: []string{"A fact."},
			})
			return err
		}},
		{name: "blank failure", run: func() error {
			_, err := normalizeFailStepCommand(FailStepCommand{OperationID: validID, StepID: 1, Error: "  "})
			return err
		}},
		{name: "blank cancellation", run: func() error {
			_, err := normalizeCancelJobCommand(CancelJobCommand{OperationID: validID, JobID: 1})
			return err
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.run(); err == nil {
				t.Fatal("invalid lifecycle command was accepted")
			}
		})
	}
}
