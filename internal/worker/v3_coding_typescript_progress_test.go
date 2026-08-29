package worker

import (
	"fmt"
	"strings"
	"testing"
)

func TestTypeScriptCorrectionProgressUsesDistinctDiagnosticAndVerificationStage(t *testing.T) {
	t.Parallel()
	progress := newDirectCodingTypeScriptCorrectionProgress()
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	states := []struct {
		blockID           string
		verificationStage string
		diagnostic        string
	}{
		{
			blockID:           "inventory.filter",
			verificationStage: "run typecheck", diagnostic: "error TS2322: wrong result type",
		},
		{
			blockID:           "inventory.filter",
			verificationStage: "run test", diagnostic: "expected 1 to be 2",
		},
		{
			blockID:           "inventory.filter",
			verificationStage: "run test", diagnostic: "expected visible count to be 2",
		},
	}
	for index, state := range states {
		if err := progress.observeSemantic(
			state.blockID, state.verificationStage, state.diagnostic,
		); err != nil {
			t.Fatalf("changing state %d was rejected: %v", index, err)
		}
	}
	repeated := states[1]
	if err := progress.observeSemantic(
		repeated.blockID, repeated.verificationStage, repeated.diagnostic,
	); err == nil || !strings.Contains(err.Error(), "repeated compiler diagnostic") {
		t.Fatalf("repeated exact progress state error=%v", err)
	}
}

func TestTypeScriptCorrectionProgressRejectsSameDiagnosticAfterSourceTransition(t *testing.T) {
	t.Parallel()
	progress := newDirectCodingTypeScriptCorrectionProgress()
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	if err := progress.observeSemantic(
		"inventory.filter", "run test", "error TS2304: Cannot find name 'missing'.",
	); err != nil {
		t.Fatal(err)
	}
	if err := progress.observeSemantic(
		"inventory.filter", "run typecheck", "error TS2304: Cannot find name 'missing'.",
	); err == nil || !strings.Contains(err.Error(), "no distinct verified failure") {
		t.Fatalf("repeated diagnostic error=%v", err)
	}
}

func TestTypeScriptCorrectionProgressHasCodeOwnedPerBlockSemanticLimit(t *testing.T) {
	t.Parallel()
	progress := newDirectCodingTypeScriptCorrectionProgress()
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	states := []struct {
		blockID           string
		verificationStage string
		diagnostic        string
	}{
		{blockID: "feature.001", verificationStage: "run typecheck", diagnostic: "error TS2322: first"},
		{blockID: "feature.001", verificationStage: "run typecheck", diagnostic: "error TS2322: second"},
		{blockID: "feature.001", verificationStage: "run test", diagnostic: "expected three"},
	}
	if len(states) != maxDirectCodingTypeScriptStageSemanticCorrections {
		t.Fatalf("test fixture has %d states for a %d-state limit", len(states), maxDirectCodingTypeScriptStageSemanticCorrections)
	}
	for index, state := range states {
		if err := progress.observeSemantic(
			state.blockID, state.verificationStage, state.diagnostic,
		); err != nil {
			t.Fatalf("correction %d: %v", index+1, err)
		}
	}
	err := progress.observeSemantic(
		"feature.001", "run build", "error TS2322: exhausted distinct state",
	)
	if err == nil || !strings.Contains(err.Error(), "code-owned limit") {
		t.Fatalf("correction exhaustion error=%v", err)
	}
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	if err := progress.observeSemantic(
		"feature.002", "run typecheck", "error TS2322: next stage",
	); err != nil {
		t.Fatalf("next stage did not receive a fresh diagnostic-bound repair limit: %v", err)
	}
}

func TestTypeScriptCorrectionProgressKeepsDeterministicClosureIndependentPerBlock(t *testing.T) {
	t.Parallel()
	progress := newDirectCodingTypeScriptCorrectionProgress()
	if err := progress.beginStage(); err != nil {
		t.Fatal(err)
	}
	for index := range maxDirectCodingTypeScriptStageDeterministicCorrections {
		if err := progress.observeDeterministic(
			"catalog.summary", "run typecheck",
			fmt.Sprintf("error TS2322: deterministic catalog mismatch %d", index+1),
		); err != nil {
			t.Fatalf("deterministic correction %d: %v", index+1, err)
		}
	}
	for index := range maxDirectCodingTypeScriptStageSemanticCorrections {
		if err := progress.observeSemantic(
			"catalog.summary", "run test",
			fmt.Sprintf("expected catalog observation %d", index+1),
		); err != nil {
			t.Fatalf("independent semantic correction %d: %v", index+1, err)
		}
	}
	if err := progress.observeDeterministic(
		"catalog.summary", "run typecheck", "error TS2322: deterministic catalog overflow",
	); err == nil || !strings.Contains(err.Error(), "deterministic correction") {
		t.Fatalf("deterministic exhaustion error=%v", err)
	}
	if err := progress.observeDeterministic(
		"schedule.timeline", "run typecheck", "error TS2322: independent schedule mismatch",
	); err != nil {
		t.Fatalf("one block consumed another block's deterministic budget: %v", err)
	}
}
