package omni

import (
	"strings"
	"testing"
)

func TestProgressionGateForcesRecoveryForExhaustedCommand(t *testing.T) {
	command := "npm install @hotwired/stimulus recyclr tailwindcss webpack webpack-cli --save-dev"
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt: "finish calculator app",
		ObjectiveLedger: []StructuredObjective{
			{ID: "implement_calculator_ui", Status: "pending"},
		},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: command, ExitCode: 1, Stderr: "install failed"},
			{Step: 2, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=2"},
		},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	for _, want := range []string{
		"Recovery required.",
		"Blocked command(s): " + command,
		"Forbidden command(s): " + command,
		"Active objective(s): implement_calculator_ui",
		"inspect existing files",
	} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("recovery task missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}

func TestProgressionGateFailsCleanlyWhenRecoveryIsExhausted(t *testing.T) {
	command := "npm install @hotwired/stimulus recyclr tailwindcss webpack webpack-cli --save-dev"
	gate := ProgressionGate{MaxRecoveryAttempts: 1}
	decision := gate.ReviewStep(ProgressionInput{
		ObjectiveLedger: []StructuredObjective{{ID: "implement_calculator_ui", Status: "pending"}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: command, ExitCode: 1, Stderr: "install failed"},
			{Step: 2, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=2"},
			{Step: 2, ExitCode: 1, Stderr: "progression_gate: forced recovery required; repeated command exhausted; deterministic recovery required"},
		},
	})

	if decision.Action != ProgressFailWithEvidence {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressFailWithEvidence)
	}
	if !strings.Contains(decision.Reason, "recovery exhausted") {
		t.Fatalf("reason = %q", decision.Reason)
	}
}

func TestProgressionGateAllowsDifferentFailureFingerprint(t *testing.T) {
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		ObjectiveLedger: []StructuredObjective{{ID: "verify_ui_and_logic", Status: "pending"}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "go test ./internal/omni -run TestFoo", ExitCode: 1, Stderr: "expected 1 got 0"},
			{Step: 2, Command: "go test ./internal/omni -run TestFoo", ExitCode: 1, Stderr: "expected 2 got 1"},
		},
	})

	if decision.Action != ProgressAllow {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressAllow)
	}
}

func TestProgressionGateUsesCompletedEvidenceForRepeatedSuccess(t *testing.T) {
	command := "ls -la /tmp/demo"
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt: "connect calculator UI to logic",
		ObjectiveLedger: []StructuredObjective{
			{ID: "create_calculator_ui", Status: "pending"},
			{ID: "connect_ui_to_logic", Status: "pending"},
		},
		Observations: []StructuredCommandObservation{
			{Step: 2, Command: command, ExitCode: 0, Stdout: "package.json\nsrc\n"},
			{Step: 4, Command: "SKIPPED_REPEAT_SUCCESS: " + command, RejectedCommand: command, ExitCode: 0, Stdout: "already_completed"},
		},
	})

	if decision.Action != ProgressUseCompletedEvidence {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressUseCompletedEvidence)
	}
	if decision.PreviousResult == nil || !strings.Contains(decision.PreviousResult.Stdout, "package.json") {
		t.Fatalf("previous result missing stdout: %#v", decision.PreviousResult)
	}
	for _, want := range []string{"Use the previous command output", "package.json", "src", "Do not return done=true"} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("recovery task missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}

func TestProgressionGateBuildsMissingFileRecovery(t *testing.T) {
	gate := ProgressionGate{}
	decision := gate.ReviewStep(ProgressionInput{
		Prompt:          "connect calculator UI to logic",
		ObjectiveLedger: []StructuredObjective{{ID: "create_calculator_ui", Status: "pending"}},
		Observations: []StructuredCommandObservation{{
			Step:     1,
			Command:  "cat /tmp/demo/index.html",
			ExitCode: 1,
			Stderr:   "cat: /tmp/demo/index.html: No such file or directory",
		}},
	})

	if decision.Action != ProgressForceRecovery {
		t.Fatalf("action = %s, want %s", decision.Action, ProgressForceRecovery)
	}
	for _, want := range []string{"target path does not exist", "ls -la /tmp/demo", "find /tmp/demo -maxdepth 3 -type f", "Do not retry the invalid path"} {
		if !strings.Contains(decision.RecoveryToolTask, want) {
			t.Fatalf("missing-file recovery missing %q: %s", want, decision.RecoveryToolTask)
		}
	}
}
