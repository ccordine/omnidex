package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func ablationEpisodeTemplate(
	fixture MicrogauntletCase,
	episode cognition.EpisodeRef,
	request AblationRunRequest,
	authority PairedRunAuthority,
) (EpisodeManifest, error) {
	public, err := NewPublicRunAuthority(authority, request.Variant)
	if err != nil {
		return EpisodeManifest{}, err
	}
	return newAblationEpisodeTemplate(
		public, episode, request.OmnidexCommit, request.LedgerSchemaVersion,
		request.WorkingSetPolicyVersion, request.ProjectionPolicyVersion,
	)
}

func newAblationEpisodeTemplate(
	authority PublicRunAuthority,
	episode cognition.EpisodeRef,
	omnidexCommit string,
	ledgerSchemaVersion string,
	workingSetPolicyVersion string,
	projectionPolicyVersion string,
) (EpisodeManifest, error) {
	authoritySHA, err := authority.SHA256()
	if err != nil {
		return EpisodeManifest{}, err
	}
	brain := authority.RatGeneration.Fixed.Brain
	return EpisodeManifest{
		Schema: EpisodeManifestSchemaV2, EpisodeID: episode.ID,
		Scenario: authority.Scenario, PublicRunAuthoritySHA256: authoritySHA,
		Variant: authority.Variant, OmnidexCommit: omnidexCommit,
		RuntimeVersion:          authority.RatGeneration.Runtime.Version,
		LedgerSchemaVersion:     ledgerSchemaVersion,
		WorkingSetPolicyVersion: workingSetPolicyVersion,
		ProjectionPolicyVersion: projectionPolicyVersion,
		RatGeneration:           authority.RatGeneration, StationBudget: authority.Budget.Station,
		Model: ModelRecord{
			Name: brain.Model, Digest: brain.Digest, Quantization: brain.Quantization,
			SamplingSHA256: brain.SamplingSHA256, ContextLimit: brain.NativeContextLimit,
			Hardware: brain.Hardware, HardwareAuthoritySource: brain.HardwareAuthoritySource,
			Backend: brain.Backend, BackendVersion: brain.BackendVersion,
		},
	}, nil
}

func finishAblation(
	request AblationRunRequest,
	authority PairedRunAuthority,
	fixture MicrogauntletCase,
	sealed SealedEpisode,
	evidence AblationEvidenceAuthority,
	failure FailureTrace,
) (AblationRunResult, error) {
	evaluation, causal, err := ScoreAndSealMicrogauntletEpisode(
		request.EvaluationPath, fixture, authority.SurfaceVersion, sealed,
		SymbolicEvaluationEvidence{
			GoalPredicateSatisfied: sealed.Manifest.Outcome.GoalSatisfied,
			ValidTerminalState:     sealed.Manifest.Outcome.Terminal,
			ActualDecisionCost:     int64(sealed.Manifest.Resources.ModelDecisions), Failure: failure,
		},
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return AblationRunResult{}, err
	}
	variant, err := BindVariantResult(authority, request.Variant, sealed, evaluation)
	if err != nil {
		return AblationRunResult{}, err
	}
	efficiency, err := evaluation.EfficiencyMetric()
	if err != nil {
		return AblationRunResult{}, err
	}
	evidenceClass := AblationDevelopmentEvidence
	switch request.Variant {
	case VariantOracleEvidence:
		evidenceClass = AblationOracleContaminated
	case VariantRawShell:
		evidenceClass = AblationBenchmarkOnly
	}
	result := AblationRunResult{
		EvidenceClass: evidenceClass, PromotionEligible: false,
		Authority: authority, Variant: variant, Episode: sealed, Evidence: evidence, Oracle: oracle,
		Evaluation: evaluation, Efficiency: efficiency, CausalAcquisition: causal,
	}
	if err := result.Validate(); err != nil {
		return AblationRunResult{}, fmt.Errorf("validate cognition ablation result: %w", err)
	}
	return result, nil
}
