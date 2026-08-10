package queue

import "testing"

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
