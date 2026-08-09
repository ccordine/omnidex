package modelgauntlet

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type completePredictionKey struct {
	CaseID     string
	Repetition int
	Variant    Variant
}

func EvaluateCompleteRequirementPartition(
	report CompleteRequirementReport,
	labels []CompleteRequirementLabel,
) (CompleteRequirementEvaluation, error) {
	if report.Schema != CompleteRequirementReportSchemaV1 {
		return CompleteRequirementEvaluation{}, fmt.Errorf("complete requirement report schema must be %q", CompleteRequirementReportSchemaV1)
	}
	if report.Config.PromptRenderer != CompleteRequirementRendererV3 {
		return CompleteRequirementEvaluation{}, fmt.Errorf("complete requirement prompt renderer must be %q", CompleteRequirementRendererV3)
	}
	if report.Config.StructuredMaxOutputTokens != maxStructuredTokens ||
		report.Config.PerSplitAdvisoryMaxOutputTokens != maxDeliberationTokens ||
		report.Config.FinalAdvisoryMaxOutputTokens != maxFinalRequirementDeliberationTokens {
		return CompleteRequirementEvaluation{}, fmt.Errorf("complete requirement report output budgets do not match the registered protocol")
	}
	if report.Config.Repetitions < 1 || report.Config.Repetitions > maxCompleteRequirementRepeats {
		return CompleteRequirementEvaluation{}, fmt.Errorf("complete requirement report repetitions are invalid")
	}
	caseSources, err := completeCaseSources(report.Cases)
	if err != nil {
		return CompleteRequirementEvaluation{}, err
	}
	labelQuotes, err := validateCompleteRequirementLabels(caseSources, labels)
	if err != nil {
		return CompleteRequirementEvaluation{}, err
	}
	predictions, scores, err := scoreCompletePredictions(report, caseSources, labelQuotes)
	if err != nil {
		return CompleteRequirementEvaluation{}, err
	}
	metrics, err := aggregateCompleteRequirementMetrics(report)
	if err != nil {
		return CompleteRequirementEvaluation{}, err
	}
	evaluation := CompleteRequirementEvaluation{
		ReportSchema: report.Schema, Scores: scores, Metrics: metrics,
		Transitions: completeTransitions(report, predictions, labelQuotes),
		Stability:   completeStability(report, predictions),
	}
	evaluation.Promotion = evaluateCompleteRequirementPromotion(report, evaluation)
	return evaluation, nil
}

func scoreCompletePredictions(
	report CompleteRequirementReport,
	caseSources map[string]string,
	labels map[string][]string,
) (map[completePredictionKey]CompleteRequirementPrediction, map[Variant]VariantScore, error) {
	total := len(report.Cases) * report.Config.Repetitions
	scores := make(map[Variant]VariantScore, len(completeRequirementVariants()))
	for _, variant := range completeRequirementVariants() {
		scores[variant] = VariantScore{Total: total}
	}
	seen := make(map[completePredictionKey]CompleteRequirementPrediction, total*len(completeRequirementVariants()))
	for _, prediction := range report.Predictions {
		source, exists := caseSources[prediction.CaseID]
		if !exists {
			return nil, nil, fmt.Errorf("prediction references unknown case %q", prediction.CaseID)
		}
		if prediction.Repetition < 1 || prediction.Repetition > report.Config.Repetitions {
			return nil, nil, fmt.Errorf("prediction for case %q has invalid repetition %d", prediction.CaseID, prediction.Repetition)
		}
		if !isCompleteRequirementVariant(prediction.Variant) {
			return nil, nil, fmt.Errorf("prediction variant %q is unsupported", prediction.Variant)
		}
		key := completePredictionKey{prediction.CaseID, prediction.Repetition, prediction.Variant}
		if _, duplicate := seen[key]; duplicate {
			return nil, nil, fmt.Errorf("prediction for case %q repetition %d variant %q is duplicated", prediction.CaseID, prediction.Repetition, prediction.Variant)
		}
		if prediction.Valid {
			decision := assemblyline.RequirementPartitionDecision{
				Schema:        assemblyline.RequirementPartitionSchemaV1,
				FeatureQuotes: append([]string(nil), prediction.FeatureQuotes...),
			}
			if err := assemblyline.ValidateCompleteRequirementPartition(source, decision); err != nil {
				return nil, nil, fmt.Errorf("valid prediction for case %q is invalid: %w", prediction.CaseID, err)
			}
			if strings.TrimSpace(prediction.Error) != "" {
				return nil, nil, fmt.Errorf("valid prediction for case %q carries an error", prediction.CaseID)
			}
		} else if strings.TrimSpace(prediction.Error) == "" {
			return nil, nil, fmt.Errorf("invalid prediction for case %q requires an explicit error", prediction.CaseID)
		}
		seen[key] = prediction
		score := scores[prediction.Variant]
		if prediction.Valid {
			score.Valid++
			if slices.Equal(prediction.FeatureQuotes, labels[prediction.CaseID]) {
				score.Correct++
			}
		}
		scores[prediction.Variant] = score
	}
	for caseID := range caseSources {
		for repetition := 1; repetition <= report.Config.Repetitions; repetition++ {
			for _, variant := range completeRequirementVariants() {
				key := completePredictionKey{caseID, repetition, variant}
				if _, exists := seen[key]; !exists {
					return nil, nil, fmt.Errorf("missing prediction for case %q repetition %d variant %q", caseID, repetition, variant)
				}
			}
		}
	}
	return seen, scores, nil
}

func completeTransitions(
	report CompleteRequirementReport,
	predictions map[completePredictionKey]CompleteRequirementPrediction,
	labels map[string][]string,
) map[Variant]PairedTransitions {
	result := map[Variant]PairedTransitions{
		VariantPerSplitAdvisory: {}, VariantFinalPassAdvisory: {},
	}
	for _, fixture := range report.Cases {
		for repetition := 1; repetition <= report.Config.Repetitions; repetition++ {
			direct := completePredictionPassed(predictions[completePredictionKey{fixture.ID, repetition, VariantDirect}], labels[fixture.ID])
			for _, variant := range []Variant{VariantPerSplitAdvisory, VariantFinalPassAdvisory} {
				assisted := completePredictionPassed(predictions[completePredictionKey{fixture.ID, repetition, variant}], labels[fixture.ID])
				transition := result[variant]
				switch {
				case direct && assisted:
					transition.DirectPassAssistedPass++
				case direct:
					transition.DirectPassAssistedFail++
				case assisted:
					transition.DirectFailAssistedPass++
				default:
					transition.DirectFailAssistedFail++
				}
				result[variant] = transition
			}
		}
	}
	return result
}

func completeStability(
	report CompleteRequirementReport,
	predictions map[completePredictionKey]CompleteRequirementPrediction,
) map[Variant]VariantStability {
	result := make(map[Variant]VariantStability, len(completeRequirementVariants()))
	for _, variant := range completeRequirementVariants() {
		stability := VariantStability{Cases: len(report.Cases)}
		for _, fixture := range report.Cases {
			first := predictions[completePredictionKey{fixture.ID, 1, variant}]
			stable := true
			for repetition := 2; repetition <= report.Config.Repetitions; repetition++ {
				candidate := predictions[completePredictionKey{fixture.ID, repetition, variant}]
				if candidate.Valid != first.Valid || candidate.Error != first.Error || !slices.Equal(candidate.FeatureQuotes, first.FeatureQuotes) {
					stable = false
					break
				}
			}
			if stable {
				stability.Stable++
			} else {
				stability.Unstable++
			}
		}
		result[variant] = stability
	}
	return result
}

func completePredictionPassed(prediction CompleteRequirementPrediction, label []string) bool {
	return prediction.Valid && slices.Equal(prediction.FeatureQuotes, label)
}

func isCompleteRequirementVariant(variant Variant) bool {
	return variant == VariantDirect || variant == VariantPerSplitAdvisory || variant == VariantFinalPassAdvisory
}
