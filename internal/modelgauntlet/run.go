package modelgauntlet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func RunCapabilityRelation(
	ctx context.Context,
	config CapabilityRelationConfig,
	cases []CapabilityRelationCase,
	generator Generator,
) (CapabilityRelationReport, error) {
	report := CapabilityRelationReport{
		Schema: CapabilityRelationReportSchemaV1, StartedAt: time.Now().UTC(),
		Cases: append([]CapabilityRelationCase(nil), cases...),
		Config: CapabilityRelationConfigEvidence{
			StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
			ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
			PromptRenderer: CapabilityRelationPromptRendererV6,
		},
	}
	if err := validateRun(config, cases, generator); err != nil {
		return report, err
	}
	specs, err := capabilityRelationAdvisoryCases(cases)
	if err != nil {
		return report, err
	}
	protocolReport, err := runStructuredAdvisoryProtocol(ctx, advisoryProtocolConfig{
		StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
		ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
	}, specs, generator)
	report.Calls = append(report.Calls, protocolReport.Calls...)
	report.Predictions = capabilityRelationPredictions(cases, protocolReport.Outcomes)
	report.FinishedAt = time.Now().UTC()
	if err != nil {
		return report, err
	}
	return report, nil
}

func validateRun(config CapabilityRelationConfig, cases []CapabilityRelationCase, generator Generator) error {
	if generator == nil {
		return fmt.Errorf("capability relation gauntlet requires a generator")
	}
	if err := validateAdvisoryConfig(advisoryProtocolConfig{
		StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
		ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
	}); err != nil {
		return err
	}
	if len(cases) == 0 {
		return fmt.Errorf("capability relation gauntlet requires at least one case")
	}
	seen := make(map[string]struct{}, len(cases))
	for _, fixture := range cases {
		if fixture.ID == "" || fixture.ID != strings.TrimSpace(fixture.ID) {
			return fmt.Errorf("capability relation case requires one trimmed ID")
		}
		if _, exists := seen[fixture.ID]; exists {
			return fmt.Errorf("capability relation case ID %q is duplicated", fixture.ID)
		}
		seen[fixture.ID] = struct{}{}
		if _, err := assemblyline.NewCapabilityRelationJob(fixture.Input); err != nil {
			return fmt.Errorf("capability relation case %q is invalid: %w", fixture.ID, err)
		}
	}
	return nil
}

func capabilityRelationAdvisoryCases(cases []CapabilityRelationCase) ([]structuredAdvisoryCase, error) {
	specs := make([]structuredAdvisoryCase, 0, len(cases))
	for _, fixture := range cases {
		job, err := assemblyline.NewCapabilityRelationJob(fixture.Input)
		if err != nil {
			return nil, fmt.Errorf("create capability relation job for case %q: %w", fixture.ID, err)
		}
		specs = append(specs, structuredAdvisoryCase{
			ID: fixture.ID, Job: job,
			Station: capabilityRelationAdvisoryStation{input: fixture.Input},
		})
	}
	return specs, nil
}

func capabilityRelationPredictions(
	cases []CapabilityRelationCase,
	outcomes []advisoryOutcome,
) []CapabilityRelationPrediction {
	inputs := make(map[string]assemblyline.CapabilityRelationInput, len(cases))
	for _, fixture := range cases {
		inputs[fixture.ID] = fixture.Input
	}
	predictions := make([]CapabilityRelationPrediction, 0, len(outcomes))
	for _, outcome := range outcomes {
		prediction := CapabilityRelationPrediction{
			CaseID: outcome.CaseID, Variant: outcome.Variant, Error: outcome.Error,
		}
		if outcome.Valid {
			relation, err := decodeRelation(outcome.Content, inputs[outcome.CaseID])
			if err != nil {
				prediction.Error = err.Error()
			} else {
				prediction.Valid = true
				prediction.Relation = relation
				prediction.Error = ""
			}
		}
		predictions = append(predictions, prediction)
	}
	return predictions
}
