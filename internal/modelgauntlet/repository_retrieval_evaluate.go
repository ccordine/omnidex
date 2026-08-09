package modelgauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func EvaluateRepositoryRetrieval(
	report RepositoryRetrievalReport,
	labels []RepositoryRetrievalLabel,
) (RepositoryRetrievalEvaluation, error) {
	if report.Schema != RepositoryRetrievalReportSchemaV2 {
		return RepositoryRetrievalEvaluation{}, fmt.Errorf("repository retrieval report schema must be %q", RepositoryRetrievalReportSchemaV2)
	}
	caseIDs := make(map[string]struct{}, len(report.Cases))
	inputs := make(map[string]assemblyline.RepositoryRetrievalInput, len(report.Cases))
	for _, fixture := range report.Cases {
		if _, duplicate := caseIDs[fixture.ID]; duplicate {
			return RepositoryRetrievalEvaluation{}, fmt.Errorf("repository retrieval report case %q is duplicated", fixture.ID)
		}
		caseIDs[fixture.ID] = struct{}{}
		inputs[fixture.ID] = fixture.Input
	}
	byID, err := validateRepositoryRetrievalLabels(inputs, labels)
	if err != nil {
		return RepositoryRetrievalEvaluation{}, err
	}
	scores := map[Variant]VariantScore{
		VariantDirect: {Total: len(report.Cases)}, VariantDeliberated: {Total: len(report.Cases)},
	}
	seen := make(map[string]struct{}, len(report.Predictions))
	for _, prediction := range report.Predictions {
		if prediction.Variant != VariantDirect && prediction.Variant != VariantDeliberated {
			return RepositoryRetrievalEvaluation{}, fmt.Errorf("repository retrieval prediction variant %q is unsupported", prediction.Variant)
		}
		if _, exists := caseIDs[prediction.CaseID]; !exists {
			return RepositoryRetrievalEvaluation{}, fmt.Errorf("repository retrieval prediction references unknown case %q", prediction.CaseID)
		}
		key := prediction.CaseID + "\x00" + string(prediction.Variant)
		if _, duplicate := seen[key]; duplicate {
			return RepositoryRetrievalEvaluation{}, fmt.Errorf("repository retrieval prediction %q %q is duplicated", prediction.CaseID, prediction.Variant)
		}
		seen[key] = struct{}{}
		score := scores[prediction.Variant]
		if prediction.Valid {
			score.Valid++
			label := byID[prediction.CaseID]
			if prediction.Operation == label.Operation && prediction.QueryQuote == label.QueryQuote {
				score.Correct++
			}
		}
		scores[prediction.Variant] = score
	}
	for caseID := range caseIDs {
		for _, variant := range []Variant{VariantDirect, VariantDeliberated} {
			if _, exists := seen[caseID+"\x00"+string(variant)]; !exists {
				return RepositoryRetrievalEvaluation{}, fmt.Errorf("missing repository retrieval prediction for case %q variant %q", caseID, variant)
			}
		}
	}
	metrics, err := aggregateVariantMetrics(report.Calls, caseIDs)
	if err != nil {
		return RepositoryRetrievalEvaluation{}, err
	}
	return RepositoryRetrievalEvaluation{ReportSchema: report.Schema, Scores: scores, Metrics: metrics}, nil
}

func validateRepositoryRetrievalLabels(
	inputs map[string]assemblyline.RepositoryRetrievalInput,
	labels []RepositoryRetrievalLabel,
) (map[string]RepositoryRetrievalLabel, error) {
	byID := make(map[string]RepositoryRetrievalLabel, len(labels))
	for _, label := range labels {
		input, exists := inputs[label.CaseID]
		if !exists {
			return nil, fmt.Errorf("repository retrieval label references unknown case %q", label.CaseID)
		}
		if _, duplicate := byID[label.CaseID]; duplicate {
			return nil, fmt.Errorf("repository retrieval label for case %q is duplicated", label.CaseID)
		}
		decision := assemblyline.RepositoryRetrievalDecision{
			Schema:    assemblyline.RepositoryRetrievalSchemaV2,
			Operation: label.Operation, QueryQuote: label.QueryQuote,
		}
		if err := decision.ValidateFor(input); err != nil {
			return nil, fmt.Errorf("repository retrieval label for case %q is invalid: %w", label.CaseID, err)
		}
		byID[label.CaseID] = label
	}
	for caseID := range inputs {
		if _, exists := byID[caseID]; !exists {
			return nil, fmt.Errorf("missing repository retrieval label for case %q", caseID)
		}
	}
	return byID, nil
}
