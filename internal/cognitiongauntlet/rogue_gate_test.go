package cognitiongauntlet

import "testing"

func TestRogueAdmissionRequiresEveryEarlierProofRail(t *testing.T) {
	input := validRogueAdmission()
	if gate := EvaluateRogueAdmission(input); !gate.Passed {
		t.Fatalf("Rogue admission=%+v", gate)
	}
	input.InitialSuites[1].GoalSuccesses--
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue was admitted after an isolated microgauntlet failure")
	}
	input = validRogueAdmission()
	input.Absolute.HiddenOracleAccesses = 1
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue was admitted after hidden oracle access")
	}
	input = validRogueAdmission()
	input.Transfer.SuccessfulSurfaces = input.Transfer.SuccessfulSurfaces[:1]
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue was admitted before transfer passed")
	}
	input = validRogueAdmission()
	input.InitialSuites[0].Variant = VariantDeterministicOracle
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue treated world-oracle validation as cognition competence")
	}
	input = validRogueAdmission()
	input.ScaleVariant = VariantDeterministicOracle
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue treated world-oracle scale validation as cognition scale evidence")
	}
	input = validRogueAdmission()
	input.InitialSuites[0].CausalAdmissions--
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue treated terminal success without causal evidence acquisition as competence")
	}
	input = validRogueAdmission()
	input.InitialSuites[0].CleanDeskAdmissions--
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue rewarded a projection that omitted critical evidence")
	}
	input = validRogueAdmission()
	input.TransferCausalAdmission = false
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue accepted a transfer rail without causal acquisition")
	}
	input = validRogueAdmission()
	input.ScaleCleanDeskAdmission = false
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue accepted a scale rail that omitted critical projected evidence")
	}
}

func TestRogueAdmissionRejectsMissingOrDuplicateInitialSuite(t *testing.T) {
	input := validRogueAdmission()
	input.InitialSuites = input.InitialSuites[:4]
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue was admitted without Combined qualification")
	}
	input = validRogueAdmission()
	input.InitialSuites[4].Suite = SuiteRetrieve
	if EvaluateRogueAdmission(input).Passed {
		t.Fatal("Rogue was admitted with duplicate Retrieve evidence")
	}
}

func validRogueAdmission() RogueAdmissionInput {
	suites := []Suite{SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined}
	qualified := make([]SuiteQualification, len(suites))
	for index, suite := range suites {
		qualified[index] = SuiteQualification{
			Suite: suite, Variant: VariantFullCognition,
			Episodes: 20, GoalSuccesses: 20, ValidTerminals: 20,
			CausalAdmissions: 20, CleanDeskAdmissions: 20,
		}
	}
	return RogueAdmissionInput{
		InitialSuites: qualified,
		ScaleVariant:  VariantFullCognition, TransferVariant: VariantFullCognition,
		ScaleCausalAdmission: true, TransferCausalAdmission: true,
		ScaleCleanDeskAdmission: true, TransferCleanDeskAdmission: true,
		Continuity: ContinuityGateInput{
			Episodes: 20, CorrectWorld: 20, CorrectLedger: 20,
			CorrectWorkingSet: 20, IdenticalProjection: 20,
		},
		Scale: ScaleGateInput{
			WorldMultiplier: 100, ContextGrowth: 1.25,
			DecisionGrowth: 1.20, SuccessLossPoints: 5,
		},
		Transfer: TransferGateInput{
			HeldOutSurfaces:    []string{"filesystem.v1", "record-surface.v1"},
			SuccessfulSurfaces: []string{"filesystem.v1", "record-surface.v1"},
		},
	}
}
