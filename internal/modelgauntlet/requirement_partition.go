package modelgauntlet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func RunRequirementPartition(
	ctx context.Context,
	config RequirementPartitionConfig,
	cases []RequirementPartitionCase,
	generator Generator,
) (RequirementPartitionReport, error) {
	report := RequirementPartitionReport{
		Schema: RequirementPartitionReportSchemaV1, StartedAt: time.Now().UTC(),
		Cases: append([]RequirementPartitionCase(nil), cases...),
		Config: RequirementPartitionConfigEvidence{
			StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
			ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
			PromptRenderer: RequirementPartitionPromptRendererV2,
		},
	}
	specs, err := requirementPartitionAdvisoryCases(config, cases, generator)
	if err != nil {
		return report, err
	}
	protocolReport, err := runStructuredAdvisoryProtocol(ctx, advisoryProtocolConfig{
		StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
		ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
	}, specs, generator)
	report.Calls = append(report.Calls, protocolReport.Calls...)
	report.Predictions = requirementPartitionPredictions(cases, protocolReport.Outcomes)
	report.FinishedAt = time.Now().UTC()
	if err != nil {
		return report, err
	}
	return report, nil
}

func requirementPartitionAdvisoryCases(
	config RequirementPartitionConfig,
	cases []RequirementPartitionCase,
	generator Generator,
) ([]structuredAdvisoryCase, error) {
	if generator == nil {
		return nil, fmt.Errorf("requirement partition gauntlet requires a generator")
	}
	if err := validateAdvisoryConfig(advisoryProtocolConfig{
		StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
		ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
	}); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("requirement partition gauntlet requires at least one case")
	}
	seen := make(map[string]struct{}, len(cases))
	specs := make([]structuredAdvisoryCase, 0, len(cases))
	for _, fixture := range cases {
		if strings.TrimSpace(fixture.ID) == "" || fixture.ID != strings.TrimSpace(fixture.ID) {
			return nil, fmt.Errorf("requirement partition case requires one trimmed ID")
		}
		if _, exists := seen[fixture.ID]; exists {
			return nil, fmt.Errorf("requirement partition case ID %q is duplicated", fixture.ID)
		}
		seen[fixture.ID] = struct{}{}
		job, err := assemblyline.NewRequirementPartitionJob(fixture.Input)
		if err != nil {
			return nil, fmt.Errorf("requirement partition case %q is invalid: %w", fixture.ID, err)
		}
		specs = append(specs, structuredAdvisoryCase{
			ID: fixture.ID, Job: job,
			Station: requirementPartitionAdvisoryStation{input: fixture.Input},
		})
	}
	return specs, nil
}

func requirementPartitionPredictions(
	cases []RequirementPartitionCase,
	outcomes []advisoryOutcome,
) []RequirementPartitionPrediction {
	inputs := make(map[string]assemblyline.RequirementPartitionInput, len(cases))
	for _, fixture := range cases {
		inputs[fixture.ID] = fixture.Input
	}
	predictions := make([]RequirementPartitionPrediction, 0, len(outcomes))
	for _, outcome := range outcomes {
		prediction := RequirementPartitionPrediction{
			CaseID: outcome.CaseID, Variant: outcome.Variant, Error: outcome.Error,
		}
		if outcome.Valid {
			quotes, err := decodeRequirementPartition(outcome.Content, inputs[outcome.CaseID])
			if err != nil {
				prediction.Error = err.Error()
			} else {
				prediction.Valid = true
				prediction.FeatureQuotes = quotes
				prediction.Error = ""
			}
		}
		predictions = append(predictions, prediction)
	}
	return predictions
}

func decodeRequirementPartition(
	raw string,
	input assemblyline.RequirementPartitionInput,
) ([]string, error) {
	var decision assemblyline.RequirementPartitionDecision
	if err := decodeExactJSON(raw, &decision); err != nil {
		return nil, fmt.Errorf("invalid requirement partition: %w", err)
	}
	if err := decision.ValidateFor(input); err != nil {
		return nil, err
	}
	return append([]string(nil), decision.FeatureQuotes...), nil
}
