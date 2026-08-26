package worker

import (
	"strings"
	"testing"
)

func TestTypeScriptCorrectionProgressUsesCandidateDiagnosticAndVerificationStage(t *testing.T) {
	t.Parallel()
	progress := newDirectCodingTypeScriptCorrectionProgress()
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	states := []directCodingTypeScriptCorrectionState{
		{
			blockID: "inventory.filter", candidate: "function filter(): number { return 0; }",
			verificationStage: "run typecheck", diagnostic: "error TS2322: wrong result type",
		},
		{
			blockID: "inventory.filter", candidate: "function filter(): number { return 0; }",
			verificationStage: "run test", diagnostic: "expected 1 to be 2",
		},
		{
			blockID: "inventory.filter", candidate: "function filter(): number { return 1; }",
			verificationStage: "run test", diagnostic: "expected visible count to be 2",
		},
	}
	for index, state := range states {
		if err := progress.observe(
			state.blockID, state.candidate, state.verificationStage, state.diagnostic,
		); err != nil {
			t.Fatalf("changing state %d was rejected: %v", index, err)
		}
	}
	repeated := states[1]
	if err := progress.observe(
		repeated.blockID, repeated.candidate, repeated.verificationStage, repeated.diagnostic,
	); err == nil || !strings.Contains(err.Error(), "repeated candidate/diagnostic correction state") {
		t.Fatalf("repeated exact progress state error=%v", err)
	}
}

func TestTypeScriptCorrectionProgressAllowsChangedSourceWithSameDiagnostic(t *testing.T) {
	t.Parallel()
	progress := newDirectCodingTypeScriptCorrectionProgress()
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	if err := progress.observe(
		"inventory.filter", "function filter(): number { return missing; }",
		"run typecheck", "error TS2304: Cannot find name 'missing'.",
	); err != nil {
		t.Fatal(err)
	}
	if err := progress.observe(
		"inventory.filter", "function filter(): number { return missing + 1; }",
		"run typecheck", "error TS2304: Cannot find name 'missing'.",
	); err != nil {
		t.Fatalf("changed source with same diagnostic was rejected: %v", err)
	}
}

func TestTypeScriptCorrectionProgressHasCodeOwnedPerStageLimit(t *testing.T) {
	t.Parallel()
	progress := newDirectCodingTypeScriptCorrectionProgress()
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	states := []directCodingTypeScriptCorrectionState{
		{blockID: "feature.001", candidate: "function feature(): number { return 1; }", verificationStage: "run typecheck", diagnostic: "error TS2322: first"},
		{blockID: "feature.001", candidate: "function feature(): number { return 2; }", verificationStage: "run typecheck", diagnostic: "error TS2322: second"},
		{blockID: "feature.001", candidate: "function feature(): number { return 3; }", verificationStage: "run test", diagnostic: "expected three"},
	}
	if len(states) != maxDirectCodingTypeScriptStageCorrections {
		t.Fatalf("test fixture has %d states for a %d-state limit", len(states), maxDirectCodingTypeScriptStageCorrections)
	}
	for index, state := range states {
		if err := progress.observe(
			state.blockID, state.candidate, state.verificationStage, state.diagnostic,
		); err != nil {
			t.Fatalf("correction %d: %v", index+1, err)
		}
	}
	err := progress.observe(
		"feature.001", "function feature(): number { return 4; }",
		"run build", "error TS2322: exhausted distinct state",
	)
	if err == nil || !strings.Contains(err.Error(), "code-owned limit") {
		t.Fatalf("correction exhaustion error=%v", err)
	}
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	if err := progress.observe(
		"feature.002", "function second(): number { return 1; }",
		"run typecheck", "error TS2322: next stage",
	); err != nil {
		t.Fatalf("next stage did not receive a fresh correction budget: %v", err)
	}
}
