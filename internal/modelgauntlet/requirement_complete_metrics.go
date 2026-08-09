package modelgauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func aggregateCompleteRequirementMetrics(
	report CompleteRequirementReport,
) (map[Variant]VariantMetrics, error) {
	caseIDs := make(map[string]struct{}, len(report.Cases))
	for _, fixture := range report.Cases {
		caseIDs[fixture.ID] = struct{}{}
	}
	metrics := make(map[Variant]VariantMetrics, len(completeRequirementVariants()))
	for _, variant := range completeRequirementVariants() {
		metrics[variant] = VariantMetrics{}
	}
	for _, call := range report.Calls {
		if _, exists := caseIDs[call.Request.CaseID]; !exists {
			return nil, fmt.Errorf("call references unknown case %q", call.Request.CaseID)
		}
		if call.Request.Repetition < 1 || call.Request.Repetition > report.Config.Repetitions {
			return nil, fmt.Errorf("call for case %q has invalid repetition %d", call.Request.CaseID, call.Request.Repetition)
		}
		if call.Request.Operation == "" {
			return nil, fmt.Errorf("call for case %q has no operation identity", call.Request.CaseID)
		}
		if !isCompleteRequirementVariant(call.Request.Variant) {
			return nil, fmt.Errorf("call variant %q is unsupported", call.Request.Variant)
		}
		if err := validateCompleteRequirementCallStage(call.Request.Variant, call.Request.Stage); err != nil {
			return nil, err
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

func validateCompleteRequirementCallStage(variant Variant, stage CallStage) error {
	switch variant {
	case VariantDirect:
		if stage != StageDirect {
			return fmt.Errorf("direct complete-requirement call uses stage %q", stage)
		}
	case VariantPerSplitAdvisory:
		switch stage {
		case StageBriefing, StageDeliberation, StageSynthesis:
		default:
			return fmt.Errorf("per-split complete-requirement call uses stage %q", stage)
		}
	case VariantFinalPassAdvisory:
		switch stage {
		case StageDirect, StageDeliberation, StageSynthesis:
		default:
			return fmt.Errorf("final-pass complete-requirement call uses stage %q", stage)
		}
	default:
		return fmt.Errorf("complete-requirement call variant %q is unsupported", variant)
	}
	return nil
}

func completeCaseSources(cases []CompleteRequirementCase) (map[string]string, error) {
	result := make(map[string]string, len(cases))
	for _, fixture := range cases {
		if _, duplicate := result[fixture.ID]; duplicate {
			return nil, fmt.Errorf("complete requirement case %q is duplicated", fixture.ID)
		}
		result[fixture.ID] = fixture.SourceText
	}
	return result, nil
}

func validateCompleteRequirementLabels(
	caseSources map[string]string,
	labels []CompleteRequirementLabel,
) (map[string][]string, error) {
	result := make(map[string][]string, len(labels))
	for _, label := range labels {
		source, exists := caseSources[label.CaseID]
		if !exists {
			return nil, fmt.Errorf("complete requirement label references unknown case %q", label.CaseID)
		}
		if _, duplicate := result[label.CaseID]; duplicate {
			return nil, fmt.Errorf("complete requirement label for case %q is duplicated", label.CaseID)
		}
		decision := completeRequirementDecision(label.FeatureQuotes)
		if err := assemblyline.ValidateCompleteRequirementPartition(source, decision); err != nil {
			return nil, fmt.Errorf("complete requirement label for case %q: %w", label.CaseID, err)
		}
		result[label.CaseID] = append([]string(nil), label.FeatureQuotes...)
	}
	for caseID := range caseSources {
		if _, exists := result[caseID]; !exists {
			return nil, fmt.Errorf("complete requirement label for case %q is missing", caseID)
		}
	}
	return result, nil
}

func completeRequirementDecision(quotes []string) assemblyline.RequirementPartitionDecision {
	return assemblyline.RequirementPartitionDecision{
		Schema:        assemblyline.RequirementPartitionSchemaV1,
		FeatureQuotes: append([]string(nil), quotes...),
	}
}
