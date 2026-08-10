package cognitiongauntlet

import "fmt"

type SuiteQualification struct {
	Suite               Suite   `json:"suite"`
	Variant             Variant `json:"variant"`
	Episodes            int     `json:"episodes"`
	GoalSuccesses       int     `json:"goal_successes"`
	ValidTerminals      int     `json:"valid_terminals"`
	CausalAdmissions    int     `json:"causal_admissions"`
	CleanDeskAdmissions int     `json:"clean_desk_admissions"`
}

type RogueAdmissionInput struct {
	InitialSuites              []SuiteQualification `json:"initial_suites"`
	ScaleVariant               Variant              `json:"scale_variant"`
	TransferVariant            Variant              `json:"transfer_variant"`
	ScaleCausalAdmission       bool                 `json:"scale_causal_admission"`
	TransferCausalAdmission    bool                 `json:"transfer_causal_admission"`
	ScaleCleanDeskAdmission    bool                 `json:"scale_clean_desk_admission"`
	TransferCleanDeskAdmission bool                 `json:"transfer_clean_desk_admission"`
	Absolute                   AbsoluteGateInput    `json:"absolute"`
	Continuity                 ContinuityGateInput  `json:"continuity"`
	Scale                      ScaleGateInput       `json:"scale"`
	Transfer                   TransferGateInput    `json:"transfer"`
}

func EvaluateRogueAdmission(input RogueAdmissionInput) GateResult {
	reasons := qualifyInitialSuites(input.InitialSuites)
	if input.ScaleVariant != VariantFullCognition {
		reasons = append(reasons, "Rogue scale evidence is not from the full cognition variant")
	}
	if input.TransferVariant != VariantFullCognition {
		reasons = append(reasons, "Rogue transfer evidence is not from the full cognition variant")
	}
	if !input.ScaleCausalAdmission {
		reasons = append(reasons, "Rogue scale evidence lacks causal acquisition admission")
	}
	if !input.TransferCausalAdmission {
		reasons = append(reasons, "Rogue transfer evidence lacks causal acquisition admission")
	}
	if !input.ScaleCleanDeskAdmission {
		reasons = append(reasons, "Rogue scale evidence lacks clean-desk admission")
	}
	if !input.TransferCleanDeskAdmission {
		reasons = append(reasons, "Rogue transfer evidence lacks clean-desk admission")
	}
	checks := []struct {
		label string
		gate  GateResult
	}{
		{"absolute architecture", EvaluateAbsoluteGate(input.Absolute)},
		{"continuity", EvaluateContinuityGate(input.Continuity)},
		{"scale", EvaluateScaleGate(input.Scale)},
		{"transfer", EvaluateTransferGate(input.Transfer)},
	}
	for _, check := range checks {
		label, gate := check.label, check.gate
		if !gate.Passed {
			for _, reason := range gate.Reasons {
				reasons = append(reasons, fmt.Sprintf("%s gate: %s", label, reason))
			}
		}
	}
	return GateResult{Passed: len(reasons) == 0, Reasons: reasons}
}

func qualifyInitialSuites(results []SuiteQualification) []string {
	reasons := []string{}
	required := []Suite{SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined}
	seen := make(map[Suite]struct{}, len(results))
	for _, result := range results {
		if !containsSuite(required, result.Suite) {
			reasons = append(reasons, fmt.Sprintf("Rogue admission contains non-initial suite %q", result.Suite))
			continue
		}
		if _, duplicate := seen[result.Suite]; duplicate {
			reasons = append(reasons, fmt.Sprintf("initial suite %q is duplicated", result.Suite))
			continue
		}
		seen[result.Suite] = struct{}{}
		if result.Variant != VariantFullCognition {
			reasons = append(reasons, fmt.Sprintf(
				"initial suite %q is not qualified by the full cognition variant", result.Suite,
			))
		}
		if result.Episodes <= 0 || result.GoalSuccesses != result.Episodes ||
			result.ValidTerminals != result.Episodes || result.CausalAdmissions != result.Episodes ||
			result.CleanDeskAdmissions != result.Episodes {
			reasons = append(reasons, fmt.Sprintf(
				"initial suite %q has not passed every sealed causal episode", result.Suite,
			))
		}
	}
	for _, suite := range required {
		if _, exists := seen[suite]; !exists {
			reasons = append(reasons, fmt.Sprintf("initial suite %q has no qualification evidence", suite))
		}
	}
	return reasons
}

func containsSuite(values []Suite, expected Suite) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
