package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func fullCognitionEpisodeTemplate(
	fixture MicrogauntletCase,
	episode cognition.EpisodeRef,
	request FullCognitionRunRequest,
	authority PairedRunAuthority,
) (EpisodeManifest, error) {
	if err := episode.Validate(); err != nil {
		return EpisodeManifest{}, err
	}
	authoritySHA, err := publicRunAuthoritySHA256(authority, VariantFullCognition)
	if err != nil {
		return EpisodeManifest{}, fmt.Errorf("hash full cognition run authority: %w", err)
	}
	brain := request.RatGeneration.Fixed.Brain
	return EpisodeManifest{
		Schema: EpisodeManifestSchemaV1, EpisodeID: episode.ID,
		Scenario:                 fixture.generated.ExecutionScenario().Ref(),
		PublicRunAuthoritySHA256: authoritySHA, Variant: VariantFullCognition,
		OmnidexCommit:           request.OmnidexCommit,
		RuntimeVersion:          request.RatGeneration.Runtime.Version,
		LedgerSchemaVersion:     request.LedgerSchemaVersion,
		WorkingSetPolicyVersion: request.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: request.ProjectionPolicyVersion,
		RatGeneration:           request.RatGeneration, StationBudget: authority.Budget.Station,
		Model: ModelRecord{
			Name: brain.Model, Digest: brain.Digest, Quantization: brain.Quantization,
			SamplingSHA256: brain.SamplingSHA256, ContextLimit: brain.NativeContextLimit,
			Hardware: brain.Hardware, HardwareAuthoritySource: brain.HardwareAuthoritySource,
			Backend: brain.Backend, BackendVersion: brain.BackendVersion,
		},
	}, nil
}

func finishFullCognition(
	request FullCognitionRunRequest,
	authority PairedRunAuthority,
	fixture MicrogauntletCase,
	sealed SealedEpisode,
) (FullCognitionRunResult, error) {
	public, err := NewPublicRunAuthority(authority, VariantFullCognition)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	return EvaluatePublicFullCognition(
		request.EvaluationPath, fixture, authority, public, sealed,
	)
}
