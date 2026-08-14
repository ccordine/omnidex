package worker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
	"github.com/gryph/omnidex/internal/station"
)

const (
	objectiveAdvisorySourceID         = "objective-advisory-primary"
	objectiveAdvisoryMaxInputBytes    = 28 * 1024
	objectiveAdvisoryMaxOutputBytes   = 8 * 1024
	objectiveAdvisoryMaxOutputTokens  = 2048
	objectiveAdvisoryMinimumRelevance = 0.35
)

func (runtime *nativeRuntimeV3) newObjectiveAdvisoryRunner() (objectiveAdvisoryRunner, error) {
	if runtime == nil || runtime.svc == nil {
		return nil, fmt.Errorf("objective advisory requires an active worker runtime")
	}
	mode := runtime.svc.objectiveAdvisoryMode
	if mode == "" || mode == objectiveadvisory.ModeOff {
		return nil, nil
	}
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	providerID := strings.TrimSpace(runtime.svc.objectiveAdvisoryProvider)
	if providerID != llm.ExactPreparedProviderBackend {
		return nil, fmt.Errorf(
			"objective advisory supports only exact provider %q, received %q",
			llm.ExactPreparedProviderBackend, providerID,
		)
	}
	if nilWorkerTransport(runtime.svc.stationClient) ||
		nilWorkerTransport(runtime.svc.embeddings) {
		return nil, fmt.Errorf("enabled objective advisory requires exact generation and embedding transports")
	}
	modelName, err := runtime.svc.requiredStationModel(runtime.routing, station.ObjectiveAdvisory)
	if err != nil {
		return nil, err
	}
	config := objectiveadvisory.Config{
		Mode: mode,
		Sources: []objectiveadvisory.SourceConfig{{
			ID: objectiveAdvisorySourceID, Provider: providerID, Model: modelName,
			Sampling: objectiveadvisory.SamplingConfig{Temperature: 0},
			Budget: objectiveadvisory.Budget{
				MaxInputBytes:   objectiveAdvisoryMaxInputBytes,
				MaxOutputBytes:  objectiveAdvisoryMaxOutputBytes,
				MaxOutputTokens: objectiveAdvisoryMaxOutputTokens,
			},
		}},
		MinimumRelevance:    objectiveAdvisoryMinimumRelevance,
		MaxSelectedCapsules: 1,
	}
	advisoryRuntime, err := objectiveadvisory.New(config, exactObjectiveAdvisoryProvider{
		client: runtime.svc.stationClient, contextTokens: runtime.svc.inferenceContextTokens,
	}, runtime.svc.embeddings, time.Now)
	if err != nil {
		return nil, fmt.Errorf("configure objective advisory runtime: %w", err)
	}
	if runtime.svc.logger == nil {
		return nil, fmt.Errorf("enabled objective advisory requires a server logger")
	}
	return loggingObjectiveAdvisoryRunner{
		next: advisoryRuntime, config: config, logger: runtime.svc.logger,
	}, nil
}

type loggingObjectiveAdvisoryRunner struct {
	next   objectiveadvisory.Runner
	config objectiveadvisory.Config
	logger *log.Logger
}

func (runner loggingObjectiveAdvisoryRunner) Configuration() objectiveadvisory.Config {
	return runner.config
}

func (runner loggingObjectiveAdvisoryRunner) Run(
	ctx context.Context,
	input objectiveadvisory.ProjectionInput,
	gap objectiveadvisory.SemanticGap,
) (objectiveadvisory.Report, error) {
	started := time.Now()
	report, err := runner.next.Run(ctx, input, gap)
	runner.log(input, report, err, time.Since(started))
	return report, err
}

func (runner loggingObjectiveAdvisoryRunner) log(
	input objectiveadvisory.ProjectionInput,
	report objectiveadvisory.Report,
	runErr error,
	measured time.Duration,
) {
	statuses := make([]string, 0, len(report.Artifacts))
	artifactFailures := make([]string, 0, len(report.Artifacts))
	requestedProviders := make([]string, 0, len(report.Artifacts))
	requestedModels := make([]string, 0, len(report.Artifacts))
	effectiveProviders := make([]string, 0, len(report.Artifacts))
	effectiveModels := make([]string, 0, len(report.Artifacts))
	providerLatency := time.Duration(0)
	for _, artifact := range report.Artifacts {
		statuses = append(statuses, string(artifact.Status))
		artifactFailures = append(artifactFailures, artifact.Failure)
		requestedProviders = append(requestedProviders, artifact.Provider)
		requestedModels = append(requestedModels, artifact.RequestedModel)
		effectiveProviders = append(effectiveProviders, artifact.EffectiveProvider)
		effectiveModels = append(effectiveModels, artifact.EffectiveModel)
		providerLatency += artifact.Duration
	}
	chunkBytes := 0
	for _, chunk := range report.Chunks {
		chunkBytes += chunk.ByteCost
	}
	wall := report.Metrics.WallTime
	if wall <= 0 {
		wall = measured
	}
	triggerID := report.TriggerID
	triggerVersion := report.TriggerVersion
	if triggerID == "" {
		triggerID = objectiveadvisory.TriggerPostGroundingObjective
	}
	if triggerVersion == "" {
		triggerVersion = objectiveadvisory.TriggerVersionV1
	}
	runner.logger.Printf(
		"objective_advisory_run objective_id=%q generation=%d mode=%q trigger=%q trigger_version=%q artifact_statuses=%q artifact_failures=%q reduction_status=%q requested_providers=%q requested_models=%q effective_providers=%q effective_models=%q raw_bytes=%d chunks=%d chunk_bytes=%d candidates=%d selected=%d potential_capsule_content_bytes=%d potential_capsule_content_tokens=%d selected_capsule_content_bytes=%d selected_capsule_content_tokens=%d advisory_calls=%d embedding_calls=%d prompt_tokens=%d output_tokens=%d provider_latency_ms=%d wall_latency_ms=%d reduction_error=%q run_error=%q",
		boundedObjectiveAdvisoryLogValue(input.ObjectiveID), input.Generation, runner.config.Mode,
		triggerID, triggerVersion, boundedObjectiveAdvisoryLogValue(strings.Join(statuses, ",")),
		boundedObjectiveAdvisoryLogValue(strings.Join(artifactFailures, ",")),
		report.ReductionStatus,
		boundedObjectiveAdvisoryLogValue(strings.Join(requestedProviders, ",")),
		boundedObjectiveAdvisoryLogValue(strings.Join(requestedModels, ",")),
		boundedObjectiveAdvisoryLogValue(strings.Join(effectiveProviders, ",")),
		boundedObjectiveAdvisoryLogValue(strings.Join(effectiveModels, ",")),
		report.Metrics.RawBytes, report.Metrics.ChunksProduced, chunkBytes,
		report.Metrics.CandidateCapsules, report.Metrics.SelectedCapsules,
		report.Metrics.PotentialCapsuleContentBytes, report.Metrics.PotentialCapsuleContentTokens,
		report.Metrics.SelectedCapsuleContentBytes, report.Metrics.SelectedCapsuleContentTokens,
		report.Metrics.AdvisoryCalls, report.Metrics.EmbeddingCalls,
		report.Metrics.PromptTokens, report.Metrics.OutputTokens,
		providerLatency.Milliseconds(), wall.Milliseconds(),
		boundedObjectiveAdvisoryLogValue(report.ReductionError),
		boundedObjectiveAdvisoryLogError(runErr),
	)
}

func boundedObjectiveAdvisoryLogError(err error) string {
	if err == nil {
		return ""
	}
	return boundedObjectiveAdvisoryLogValue(err.Error())
}

func boundedObjectiveAdvisoryLogValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 512 {
		return value
	}
	value = value[:512]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
