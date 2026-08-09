package modelgauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func EvaluateCapabilityRelation(report CapabilityRelationReport, labels []CapabilityRelationLabel) (CapabilityRelationEvaluation, error) {
	if report.Schema != CapabilityRelationReportSchemaV1 {
		return CapabilityRelationEvaluation{}, fmt.Errorf("capability relation report schema must be %q", CapabilityRelationReportSchemaV1)
	}
	caseIDs := make(map[string]struct{}, len(report.Cases))
	inputs := make(map[string]assemblyline.CapabilityRelationInput, len(report.Cases))
	for _, fixture := range report.Cases {
		if _, exists := caseIDs[fixture.ID]; exists {
			return CapabilityRelationEvaluation{}, fmt.Errorf("report case %q is duplicated", fixture.ID)
		}
		caseIDs[fixture.ID] = struct{}{}
		inputs[fixture.ID] = fixture.Input
	}
	byID := make(map[string]assemblyline.CapabilityRelation, len(labels))
	for _, label := range labels {
		input, exists := inputs[label.CaseID]
		if !exists {
			return CapabilityRelationEvaluation{}, fmt.Errorf("label references unknown case %q", label.CaseID)
		}
		if _, exists := byID[label.CaseID]; exists {
			return CapabilityRelationEvaluation{}, fmt.Errorf("label for case %q is duplicated", label.CaseID)
		}
		decision := assemblyline.CapabilityRelationDecision{Schema: assemblyline.CapabilityRelationSchemaV1, Relation: label.Relation}
		if err := decision.ValidateFor(input); err != nil {
			return CapabilityRelationEvaluation{}, fmt.Errorf("label for case %q is invalid: %w", label.CaseID, err)
		}
		byID[label.CaseID] = label.Relation
	}
	for caseID := range caseIDs {
		if _, exists := byID[caseID]; !exists {
			return CapabilityRelationEvaluation{}, fmt.Errorf("missing label for case %q", caseID)
		}
	}

	scores := map[Variant]VariantScore{
		VariantDirect:      {Total: len(report.Cases)},
		VariantDeliberated: {Total: len(report.Cases)},
	}
	seen := make(map[string]struct{}, len(report.Predictions))
	for _, prediction := range report.Predictions {
		if prediction.Variant != VariantDirect && prediction.Variant != VariantDeliberated {
			return CapabilityRelationEvaluation{}, fmt.Errorf("prediction variant %q is unsupported", prediction.Variant)
		}
		if _, exists := caseIDs[prediction.CaseID]; !exists {
			return CapabilityRelationEvaluation{}, fmt.Errorf("prediction references unknown case %q", prediction.CaseID)
		}
		key := prediction.CaseID + "\x00" + string(prediction.Variant)
		if _, exists := seen[key]; exists {
			return CapabilityRelationEvaluation{}, fmt.Errorf("prediction for case %q variant %q is duplicated", prediction.CaseID, prediction.Variant)
		}
		seen[key] = struct{}{}
		score := scores[prediction.Variant]
		if prediction.Valid {
			score.Valid++
			if prediction.Relation == byID[prediction.CaseID] {
				score.Correct++
			}
		}
		scores[prediction.Variant] = score
	}
	for caseID := range caseIDs {
		for _, variant := range []Variant{VariantDirect, VariantDeliberated} {
			if _, exists := seen[caseID+"\x00"+string(variant)]; !exists {
				return CapabilityRelationEvaluation{}, fmt.Errorf("missing prediction for case %q variant %q", caseID, variant)
			}
		}
	}
	metrics, err := aggregateVariantMetrics(report.Calls, caseIDs)
	if err != nil {
		return CapabilityRelationEvaluation{}, err
	}
	return CapabilityRelationEvaluation{ReportSchema: report.Schema, Scores: scores, Metrics: metrics}, nil
}

func aggregateVariantMetrics(calls []CallEvidence, caseIDs map[string]struct{}) (map[Variant]VariantMetrics, error) {
	metrics := map[Variant]VariantMetrics{
		VariantDirect: {}, VariantDeliberated: {},
	}
	for _, call := range calls {
		if _, exists := caseIDs[call.Request.CaseID]; !exists {
			return nil, fmt.Errorf("call references unknown case %q", call.Request.CaseID)
		}
		if call.Request.Variant == VariantDirect && call.Request.Stage != StageDirect {
			return nil, fmt.Errorf("direct call uses unsupported stage %q", call.Request.Stage)
		}
		if call.Request.Variant == VariantDeliberated {
			switch call.Request.Stage {
			case StageBriefing, StageDeliberation, StageSynthesis:
			default:
				return nil, fmt.Errorf("deliberated call uses unsupported stage %q", call.Request.Stage)
			}
		} else if call.Request.Variant != VariantDirect {
			return nil, fmt.Errorf("call variant %q is unsupported", call.Request.Variant)
		}
		metric := metrics[call.Request.Variant]
		metric.Calls++
		metric.TotalDuration += call.Response.TotalDuration
		metric.LoadDuration += call.Response.LoadDuration
		metric.PromptTokens += call.Response.PromptEvalCount
		metric.EvalTokens += call.Response.EvalCount
		if call.Response.AllocatedBytes > metric.MaxAllocatedBytes {
			metric.MaxAllocatedBytes = call.Response.AllocatedBytes
		}
		if call.Response.VRAMBytes > metric.MaxVRAMBytes {
			metric.MaxVRAMBytes = call.Response.VRAMBytes
		}
		metrics[call.Request.Variant] = metric
	}
	return metrics, nil
}
