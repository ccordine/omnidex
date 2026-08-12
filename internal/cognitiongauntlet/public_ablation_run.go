package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func RunPublicAblation(
	ctx context.Context,
	bundle PublicInferenceBundle,
	request PublicAblationRunRequest,
) (PublicAblationRunResult, error) {
	if ctx == nil {
		return PublicAblationRunResult{}, fmt.Errorf("public cognition ablation context is nil")
	}
	if err := request.validate(bundle); err != nil {
		return PublicAblationRunResult{}, err
	}
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	brain, err := productionBrain(
		bundle.Authority.RatGeneration, bundle.Authority.Budget.Station.MaxOutputTokens,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	bootstrap, err := attestAblationBrain(ctx, request.Client, brain)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	attested := bootstrap.AttestedBrain
	frozen, err := bundle.Authority.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil || !sameFrozenBrain(attested, frozen) {
		return PublicAblationRunResult{}, fmt.Errorf("live provider or host differs from frozen ablation authority")
	}
	activation, err := observeAblationProviderProcess(
		ctx, request.Client, attested, episode, request.Actor,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	activationAuthority, err := activation.Authority()
	if err != nil {
		return PublicAblationRunResult{}, fmt.Errorf("derive public ablation provider process activation authority: %w", err)
	}
	state, err := newAblationStateWithAuthority(
		bundle.Authority.Variant, episode, request.Actor, bundle.Goal,
		bundle.Completion, bundle.Catalog, bundle.Authority.Budget.WorkingSetBytes,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	journal := newAblationCallJournal()
	loader := &ablationProjectionLoader{}
	policy, err := cognitionpolicy.New(
		request.Client, attested, activationAuthority, loader, journal,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	template, err := newAblationEpisodeTemplate(
		bundle.Authority, episode, request.OmnidexCommit, request.LedgerSchemaVersion,
		request.WorkingSetPolicyVersion, request.ProjectionPolicyVersion,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	bootstrapArtifact, bootstrapAuthority, err := prepareRuntimeBrainBootstrapEvidence(
		request.EpisodeSealPath, bootstrap, bundle.Authority.RatGeneration.Fixed.Brain,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	if err := appendRuntimeBrainBootstrapTrace(recorder, bootstrapAuthority); err != nil {
		return PublicAblationRunResult{}, err
	}
	activationArtifact, activationEvidenceAuthority, err := prepareRuntimeProviderActivationEvidence(
		request.EpisodeSealPath, activation, bundle.Authority.RatGeneration.Fixed.Brain,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	if err := appendRuntimeProviderActivationTrace(recorder, activationEvidenceAuthority); err != nil {
		return PublicAblationRunResult{}, err
	}
	startedAt := time.Now().UTC()
	transition, err := request.Environment.Start(ctx, bundle.Authority.Scenario)
	if err != nil {
		return PublicAblationRunResult{}, fmt.Errorf("start public cognition ablation: %w", err)
	}
	if err := appendTransitionObservations(recorder, transition); err != nil {
		return PublicAblationRunResult{}, err
	}
	if err := state.recordTransition(transition); err != nil {
		return PublicAblationRunResult{}, err
	}
	packet := ContaminatedEvidencePacket{}
	if request.ContaminatedEvidence != nil {
		packet = *request.ContaminatedEvidence
	}
	execution, err := executeAblation(
		ctx, bundle.Authority.Budget,
		request.Environment, runtimeAblationCompletion{evaluator: request.Completion},
		recorder, state, loader, journal, policy, packet, transition, startedAt,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	if err := sealRuntimeBrainBootstrapEvidence(
		request.EpisodeSealPath, bootstrapArtifact, bootstrapAuthority,
		bundle.Authority.RatGeneration.Fixed.Brain,
	); err != nil {
		return PublicAblationRunResult{}, err
	}
	if err := sealRuntimeProviderActivationEvidence(
		request.EpisodeSealPath, activationArtifact, activationEvidenceAuthority,
		bundle.Authority.RatGeneration.Fixed.Brain,
	); err != nil {
		return PublicAblationRunResult{}, err
	}
	sealed, evidenceAuthority, err := finalizeAblationEpisode(
		request.EpisodeSealPath, request.EvidenceSealPath, startedAt,
		bundle.Authority, recorder, state, journal, execution,
		bootstrapAuthority, activationEvidenceAuthority,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	result := PublicAblationRunResult{
		Authority: bundle.Authority, Episode: sealed, Evidence: evidenceAuthority,
	}
	return result, result.Validate()
}
