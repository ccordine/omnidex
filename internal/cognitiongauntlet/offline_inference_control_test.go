package cognitiongauntlet

import "testing"

func TestInferenceProcessControlHasNoImplicitPauseFallback(t *testing.T) {
	t.Parallel()
	if err := terminalInferenceControl().Validate(); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := checkpointInferenceControl(3, "/private/checkpoint.json")
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Mode != inferenceStopBeforeNextCall || checkpoint.AfterSuccessfulActions != 3 {
		t.Fatalf("checkpoint control=%+v", checkpoint)
	}

	for name, invalid := range map[string]inferenceProcessControl{
		"zero value": {},
		"terminal with boundary": {
			Mode: inferenceRunToTerminal, AfterSuccessfulActions: 1,
		},
		"checkpoint without boundary": {
			Mode: inferenceStopBeforeNextCall, CheckpointPath: "/private/checkpoint.json",
		},
		"checkpoint without path": {
			Mode: inferenceStopBeforeNextCall, AfterSuccessfulActions: 1,
		},
	} {
		name, invalid := name, invalid
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := invalid.Validate(); err == nil {
				t.Fatal("invalid inference control was accepted")
			}
		})
	}
}
