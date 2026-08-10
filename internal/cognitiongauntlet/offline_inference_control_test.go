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
	if checkpoint.Mode != inferenceStopBeforeNextCall ||
		checkpoint.StopBoundary != (inferenceBoundary{Kind: inferenceBoundaryActions, Count: 3}) {
		t.Fatalf("checkpoint control=%+v", checkpoint)
	}
	replacement, err := replacementInferenceControl(
		3, "/private/replacement.json", "/private/source.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Mode != inferenceRunToTerminal ||
		replacement.ResumeBoundary != (inferenceBoundary{Kind: inferenceBoundaryActions, Count: 3}) ||
		replacement.ResumeVerificationPath != "/private/replacement.json" {
		t.Fatalf("replacement control=%+v", replacement)
	}
	chained, err := chainedReplacementInferenceControl(
		5, "/private/next.json", "/private/source.json",
		"/private/verification.json",
		inferenceBoundary{Kind: inferenceBoundaryActions, Count: 3},
	)
	if err != nil {
		t.Fatal(err)
	}
	if chained.StopBoundary.Count != 5 || chained.ResumeBoundary.Count != 3 {
		t.Fatalf("chained control=%+v", chained)
	}
	decisions, err := decisionCheckpointInferenceControl(2, "/private/decision.json")
	if err != nil {
		t.Fatal(err)
	}
	if decisions.StopBoundary != (inferenceBoundary{Kind: inferenceBoundaryDecisions, Count: 2}) {
		t.Fatalf("decision control=%+v", decisions)
	}
	baseline, err := resumeBaselineInferenceControl("/private/resume-baseline")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Mode != inferenceRecordResumeBaseline ||
		baseline.BaselineDirectory != "/private/resume-baseline" {
		t.Fatalf("Resume baseline control=%+v", baseline)
	}

	for name, invalid := range map[string]inferenceProcessControl{
		"zero value": {},
		"terminal with boundary": {
			Mode:         inferenceRunToTerminal,
			StopBoundary: inferenceBoundary{Kind: inferenceBoundaryActions, Count: 1},
		},
		"checkpoint without boundary": {
			Mode: inferenceStopBeforeNextCall, CheckpointPath: "/private/checkpoint.json",
		},
		"checkpoint without path": {
			Mode:         inferenceStopBeforeNextCall,
			StopBoundary: inferenceBoundary{Kind: inferenceBoundaryActions, Count: 1},
		},
		"resume without verification": {
			Mode: inferenceRunToTerminal, ResumeCheckpointPath: "/private/source.json",
			ResumeBoundary: inferenceBoundary{Kind: inferenceBoundaryActions, Count: 1},
		},
		"chained boundary does not advance": {
			Mode:           inferenceStopBeforeNextCall,
			StopBoundary:   inferenceBoundary{Kind: inferenceBoundaryActions, Count: 1},
			CheckpointPath: "/private/next.json", ResumeCheckpointPath: "/private/source.json",
			ResumeVerificationPath: "/private/verify.json",
			ResumeBoundary:         inferenceBoundary{Kind: inferenceBoundaryActions, Count: 1},
		},
		"baseline without directory": {Mode: inferenceRecordResumeBaseline},
		"ordinary with baseline directory": {
			Mode: inferenceRunToTerminal, BaselineDirectory: "/private/baseline",
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
