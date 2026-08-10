package modelgauntlet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func RunRepositoryRetrieval(
	ctx context.Context,
	config RepositoryRetrievalConfig,
	cases []RepositoryRetrievalCase,
	generator Generator,
) (RepositoryRetrievalReport, error) {
	report := RepositoryRetrievalReport{
		Schema: RepositoryRetrievalReportSchemaV2, StartedAt: time.Now().UTC(),
		Cases: append([]RepositoryRetrievalCase(nil), cases...),
		Config: RepositoryRetrievalConfigEvidence{
			StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
			ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
			PromptRenderer: RepositoryRetrievalPromptRendererV2,
		},
	}
	specs, err := repositoryRetrievalAdvisoryCases(config, cases, generator)
	if err != nil {
		return report, err
	}
	protocol, err := runStructuredAdvisoryProtocol(ctx, advisoryProtocolConfig{
		StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
		ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
	}, specs, generator)
	report.Calls = append(report.Calls, protocol.Calls...)
	report.Predictions = repositoryRetrievalPredictions(cases, protocol.Outcomes)
	report.FinishedAt = time.Now().UTC()
	return report, err
}

func repositoryRetrievalAdvisoryCases(
	config RepositoryRetrievalConfig,
	cases []RepositoryRetrievalCase,
	generator Generator,
) ([]structuredAdvisoryCase, error) {
	if generator == nil {
		return nil, fmt.Errorf("repository retrieval gauntlet requires a generator")
	}
	if err := validateAdvisoryConfig(advisoryProtocolConfig{
		StableModel: config.StableModel, ReasoningModel: config.ReasoningModel,
		ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
	}); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("repository retrieval gauntlet requires at least one case")
	}
	seen := make(map[string]struct{}, len(cases))
	specs := make([]structuredAdvisoryCase, 0, len(cases))
	for _, fixture := range cases {
		if strings.TrimSpace(fixture.ID) == "" || fixture.ID != strings.TrimSpace(fixture.ID) {
			return nil, fmt.Errorf("repository retrieval case requires one trimmed ID")
		}
		if _, exists := seen[fixture.ID]; exists {
			return nil, fmt.Errorf("repository retrieval case ID %q is duplicated", fixture.ID)
		}
		seen[fixture.ID] = struct{}{}
		job, err := assemblyline.NewRepositoryRetrievalJob(fixture.Input)
		if err != nil {
			return nil, fmt.Errorf("repository retrieval case %q is invalid: %w", fixture.ID, err)
		}
		specs = append(specs, structuredAdvisoryCase{
			ID: fixture.ID, Job: job, Station: repositoryRetrievalAdvisoryStation{input: fixture.Input},
		})
	}
	return specs, nil
}

func repositoryRetrievalPredictions(
	cases []RepositoryRetrievalCase,
	outcomes []advisoryOutcome,
) []RepositoryRetrievalPrediction {
	inputs := make(map[string]assemblyline.RepositoryRetrievalInput, len(cases))
	for _, fixture := range cases {
		inputs[fixture.ID] = fixture.Input
	}
	predictions := make([]RepositoryRetrievalPrediction, 0, len(outcomes))
	for _, outcome := range outcomes {
		prediction := RepositoryRetrievalPrediction{
			CaseID: outcome.CaseID, Variant: outcome.Variant, Error: outcome.Error,
		}
		if outcome.Valid {
			decision, err := decodeRepositoryRetrieval(outcome.Content, inputs[outcome.CaseID])
			if err != nil {
				prediction.Error = err.Error()
			} else {
				prediction.Valid = true
				prediction.Operation = decision.Operation
				prediction.QueryQuote = decision.QueryQuote
				prediction.Error = ""
			}
		}
		predictions = append(predictions, prediction)
	}
	return predictions
}

func decodeRepositoryRetrieval(
	raw string,
	input assemblyline.RepositoryRetrievalInput,
) (assemblyline.RepositoryRetrievalDecision, error) {
	var decision assemblyline.RepositoryRetrievalDecision
	if err := decodeExactJSON(raw, &decision); err != nil {
		return decision, fmt.Errorf("invalid repository retrieval decision: %w", err)
	}
	if err := decision.ValidateFor(input); err != nil {
		return decision, err
	}
	return decision, nil
}
