package modelgauntlet

import (
	"fmt"
	"strings"
)

func evaluateCompleteRequirementPromotion(
	report CompleteRequirementReport,
	evaluation CompleteRequirementEvaluation,
) CompleteRequirementPromotion {
	reasons := make([]string, 0)
	if len(report.Cases) < minimumCompleteRequirementCases {
		reasons = append(reasons, fmt.Sprintf("frozen corpus has %d cases; promotion requires at least %d", len(report.Cases), minimumCompleteRequirementCases))
	}
	if report.Config.Repetitions < minimumCompleteRequirementRepeats {
		reasons = append(reasons, fmt.Sprintf("experiment has %d repetition; promotion requires at least %d", report.Config.Repetitions, minimumCompleteRequirementRepeats))
	}
	direct := evaluation.Scores[VariantDirect]
	final := evaluation.Scores[VariantFinalPassAdvisory]
	transitions := evaluation.Transitions[VariantFinalPassAdvisory]
	if final.Valid != final.Total {
		reasons = append(reasons, fmt.Sprintf("final-pass validity is %d/%d", final.Valid, final.Total))
	}
	if final.Correct <= direct.Correct {
		reasons = append(reasons, fmt.Sprintf("final-pass correctness %d/%d does not exceed direct %d/%d", final.Correct, final.Total, direct.Correct, direct.Total))
	}
	if transitions.DirectPassAssistedFail > 0 {
		reasons = append(reasons, fmt.Sprintf("final-pass introduced %d paired regression(s)", transitions.DirectPassAssistedFail))
	}
	if transitions.DirectFailAssistedPass == 0 {
		reasons = append(reasons, "final-pass produced no paired fixes")
	}
	directStability := evaluation.Stability[VariantDirect]
	finalStability := evaluation.Stability[VariantFinalPassAdvisory]
	if finalStability.Stable < directStability.Stable {
		reasons = append(reasons, fmt.Sprintf("final-pass stable cases %d/%d are below direct %d/%d", finalStability.Stable, finalStability.Cases, directStability.Stable, directStability.Cases))
	}
	reasons = append(reasons, completeModelEvidenceFailures(report)...)
	return CompleteRequirementPromotion{Eligible: len(reasons) == 0, Reasons: reasons}
}

func completeModelEvidenceFailures(report CompleteRequirementReport) []string {
	type identity struct {
		digest       string
		quantization string
	}
	identities := make(map[string]identity)
	reasons := make([]string, 0)
	for _, call := range report.Calls {
		if call.Error != "" {
			continue
		}
		if strings.TrimSpace(call.Response.Model) == "" {
			reasons = append(reasons, fmt.Sprintf("successful call for model %q has no resolved model identity", call.Request.Model))
			continue
		}
		if err := validateSHA256("model", call.Response.ModelDigest); err != nil {
			reasons = append(reasons, fmt.Sprintf("successful call for model %q has invalid digest", call.Request.Model))
			continue
		}
		quantization := strings.TrimSpace(call.Response.Quantization)
		if quantization == "" {
			reasons = append(reasons, fmt.Sprintf("successful call for model %q has no quantization evidence", call.Request.Model))
			continue
		}
		current := identity{digest: call.Response.ModelDigest, quantization: quantization}
		if prior, exists := identities[call.Request.Model]; exists && prior != current {
			reasons = append(reasons, fmt.Sprintf("model %q changed digest or quantization during the experiment", call.Request.Model))
			continue
		}
		identities[call.Request.Model] = current
	}
	for _, model := range []string{report.Config.StableModel, report.Config.ReasoningModel} {
		if _, exists := identities[model]; !exists {
			reasons = append(reasons, fmt.Sprintf("model %q has no complete runtime identity evidence", model))
		}
	}
	return uniqueStrings(reasons)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
