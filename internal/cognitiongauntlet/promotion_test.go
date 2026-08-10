package cognitiongauntlet

import "testing"

func TestCompetenceGateKeepsSuccessAndEfficiencyPoliciesDistinct(t *testing.T) {
	success := EvaluateCompetenceGate(CompetenceGateInput{
		Policy: CompetenceSuccessSuperiority, Tasks: 100,
		PairedLiftLowerBoundPoints: 1.2, Rescues: 12, Regressions: 3,
	})
	if !success.Passed {
		t.Fatalf("success gate=%+v", success)
	}
	efficiency := EvaluateCompetenceGate(CompetenceGateInput{
		Policy: CompetenceEfficiencySuperiority, Tasks: 100, SuccessLossPoints: 2,
		ContextReduction: 0.45, RequiredContextReduction: 0.45,
		DuplicateAcquisitionDelta: -1, ToolOperationDelta: -2,
	})
	if !efficiency.Passed {
		t.Fatalf("efficiency gate=%+v", efficiency)
	}
	if EvaluateCompetenceGate(CompetenceGateInput{
		Policy: CompetenceEfficiencySuperiority, Tasks: 100, SuccessLossPoints: 2.1,
		ContextReduction: 0.60, RequiredContextReduction: 0.45,
		DuplicateAcquisitionDelta: -1, ToolOperationDelta: -2,
	}).Passed {
		t.Fatal("efficiency gate hid an excessive success loss")
	}
}

func TestTransferGateRequiresTwoUnchangedHeldOutSurfaces(t *testing.T) {
	passed := EvaluateTransferGate(TransferGateInput{
		HeldOutSurfaces:    []string{"filesystem.v1", "records.v1"},
		SuccessfulSurfaces: []string{"filesystem.v1", "records.v1"},
	})
	if !passed.Passed {
		t.Fatalf("transfer gate=%+v", passed)
	}
	changed := EvaluateTransferGate(TransferGateInput{
		HeldOutSurfaces:    []string{"filesystem.v1", "records.v1"},
		SuccessfulSurfaces: []string{"filesystem.v1", "records.v1"}, PromptChanges: 1,
	})
	if changed.Passed {
		t.Fatal("transfer gate accepted a prompt change between surfaces")
	}
}
