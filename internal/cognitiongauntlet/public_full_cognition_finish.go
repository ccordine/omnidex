package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func sealPublicFullCognition(
	ctx context.Context,
	request PublicFullCognitionRunRequest,
	bundle PublicInferenceBundle,
	episode cognition.EpisodeRef,
	components fullRuntimeComponents,
	run cognitionruntime.RunResult,
) (PublicFullCognitionRunResult, error) {
	trace, err := readProductionTrace(ctx, components.repository, episode.ID)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	template, err := publicCognitionEpisodeTemplate(bundle, episode, request)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	metrics, err := appendProductionTrace(recorder, trace, RecoveryMetrics{}, nil)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	if metrics.Resources.ModelCalls != int(run.PolicyCalls) ||
		metrics.Resources.EnvironmentActions != int(run.EnvironmentActions) {
		return PublicFullCognitionRunResult{}, fmt.Errorf(
			"public runtime counters differ from its sealed production trace",
		)
	}
	sealed, err := recorder.Seal(
		request.EpisodeSealPath, trace.Header.Seal.FinalRevision, metrics.Outcome,
		metrics.Resources, metrics.Memory, metrics.Planning, metrics.Recovery,
	)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	result := PublicFullCognitionRunResult{Authority: bundle.Authority, Episode: sealed}
	return result, result.Validate()
}

func publicCognitionEpisodeTemplate(
	bundle PublicInferenceBundle,
	episode cognition.EpisodeRef,
	request PublicFullCognitionRunRequest,
) (EpisodeManifest, error) {
	authoritySHA, err := bundle.Authority.SHA256()
	if err != nil {
		return EpisodeManifest{}, err
	}
	brain := bundle.Authority.RatGeneration.Fixed.Brain
	return EpisodeManifest{
		Schema: EpisodeManifestSchemaV1, EpisodeID: episode.ID,
		Scenario: bundle.Authority.Scenario, PublicRunAuthoritySHA256: authoritySHA,
		Variant: VariantFullCognition, OmnidexCommit: request.OmnidexCommit,
		RuntimeVersion:          bundle.Authority.RatGeneration.Runtime.Version,
		LedgerSchemaVersion:     request.LedgerSchemaVersion,
		WorkingSetPolicyVersion: request.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: request.ProjectionPolicyVersion,
		RatGeneration:           bundle.Authority.RatGeneration,
		StationBudget:           bundle.Authority.Budget.Station,
		Model: ModelRecord{
			Name: brain.Model, Digest: brain.Digest, Quantization: brain.Quantization,
			SamplingSHA256: brain.SamplingSHA256, ContextLimit: brain.NativeContextLimit,
			Hardware: brain.Hardware, HardwareAuthoritySource: brain.HardwareAuthoritySource,
			Backend: brain.Backend, BackendVersion: brain.BackendVersion,
		},
	}, nil
}
