package modelgauntlet

import (
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func EvaluateRequirementPartition(
	report RequirementPartitionReport,
	labels []RequirementPartitionLabel,
) (RequirementPartitionEvaluation, error) {
	if report.Schema != RequirementPartitionReportSchemaV1 {
		return RequirementPartitionEvaluation{}, fmt.Errorf("requirement partition report schema must be %q", RequirementPartitionReportSchemaV1)
	}
	caseIDs := make(map[string]struct{}, len(report.Cases))
	inputs := make(map[string]assemblyline.RequirementPartitionInput, len(report.Cases))
	for _, fixture := range report.Cases {
		if _, exists := caseIDs[fixture.ID]; exists {
			return RequirementPartitionEvaluation{}, fmt.Errorf("report case %q is duplicated", fixture.ID)
		}
		caseIDs[fixture.ID] = struct{}{}
		inputs[fixture.ID] = fixture.Input
	}
	byID, err := validateRequirementPartitionLabels(inputs, labels)
	if err != nil {
		return RequirementPartitionEvaluation{}, err
	}
	scores := map[Variant]VariantScore{
		VariantDirect: {Total: len(report.Cases)}, VariantDeliberated: {Total: len(report.Cases)},
	}
	seen := make(map[string]struct{}, len(report.Predictions))
	for _, prediction := range report.Predictions {
		if prediction.Variant != VariantDirect && prediction.Variant != VariantDeliberated {
			return RequirementPartitionEvaluation{}, fmt.Errorf("prediction variant %q is unsupported", prediction.Variant)
		}
		if _, exists := caseIDs[prediction.CaseID]; !exists {
			return RequirementPartitionEvaluation{}, fmt.Errorf("prediction references unknown case %q", prediction.CaseID)
		}
		key := prediction.CaseID + "\x00" + string(prediction.Variant)
		if _, exists := seen[key]; exists {
			return RequirementPartitionEvaluation{}, fmt.Errorf("prediction for case %q variant %q is duplicated", prediction.CaseID, prediction.Variant)
		}
		seen[key] = struct{}{}
		score := scores[prediction.Variant]
		if prediction.Valid {
			score.Valid++
			if slices.Equal(prediction.FeatureQuotes, byID[prediction.CaseID]) {
				score.Correct++
			}
		}
		scores[prediction.Variant] = score
	}
	for caseID := range caseIDs {
		for _, variant := range []Variant{VariantDirect, VariantDeliberated} {
			if _, exists := seen[caseID+"\x00"+string(variant)]; !exists {
				return RequirementPartitionEvaluation{}, fmt.Errorf("missing prediction for case %q variant %q", caseID, variant)
			}
		}
	}
	metrics, err := aggregateVariantMetrics(report.Calls, caseIDs)
	if err != nil {
		return RequirementPartitionEvaluation{}, err
	}
	return RequirementPartitionEvaluation{ReportSchema: report.Schema, Scores: scores, Metrics: metrics}, nil
}

func validateRequirementPartitionLabels(
	inputs map[string]assemblyline.RequirementPartitionInput,
	labels []RequirementPartitionLabel,
) (map[string][]string, error) {
	byID := make(map[string][]string, len(labels))
	for _, label := range labels {
		input, exists := inputs[label.CaseID]
		if !exists {
			return nil, fmt.Errorf("label references unknown case %q", label.CaseID)
		}
		if _, exists := byID[label.CaseID]; exists {
			return nil, fmt.Errorf("label for case %q is duplicated", label.CaseID)
		}
		decision := assemblyline.RequirementPartitionDecision{
			Schema:        assemblyline.RequirementPartitionSchemaV1,
			FeatureQuotes: append([]string(nil), label.FeatureQuotes...),
		}
		if err := decision.ValidateFor(input); err != nil {
			return nil, fmt.Errorf("label for case %q is invalid: %w", label.CaseID, err)
		}
		byID[label.CaseID] = append([]string(nil), label.FeatureQuotes...)
	}
	for caseID := range inputs {
		if _, exists := byID[caseID]; !exists {
			return nil, fmt.Errorf("missing label for case %q", caseID)
		}
	}
	return byID, nil
}
