package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func oracleEpisodeRef(authority PairedRunAuthority) (cognition.EpisodeRef, error) {
	return VariantEpisodeRef(authority, VariantDeterministicOracle)
}

func oracleEpisodeTemplate(
	fixture MicrogauntletCase,
	episode cognition.EpisodeRef,
	request OracleRunRequest,
	authority PairedRunAuthority,
) (EpisodeManifest, error) {
	brain := request.RatGeneration.Fixed.Brain
	authoritySHA, err := publicRunAuthoritySHA256(authority, VariantDeterministicOracle)
	if err != nil {
		return EpisodeManifest{}, fmt.Errorf("hash validated oracle run authority: %w", err)
	}
	return EpisodeManifest{
		Schema: EpisodeManifestSchemaV2, EpisodeID: episode.ID,
		Scenario:                 fixture.generated.ExecutionScenario().Ref(),
		PublicRunAuthoritySHA256: authoritySHA,
		Variant:                  VariantDeterministicOracle,
		OmnidexCommit:            request.OmnidexCommit,
		RuntimeVersion:           request.RatGeneration.Runtime.Version,
		LedgerSchemaVersion:      request.LedgerSchemaVersion,
		WorkingSetPolicyVersion:  request.WorkingSetPolicyVersion,
		ProjectionPolicyVersion:  request.ProjectionPolicyVersion,
		RatGeneration:            request.RatGeneration,
		StationBudget:            authority.Budget.Station,
		Model: ModelRecord{
			Name: brain.Model, Digest: brain.Digest, Quantization: brain.Quantization,
			SamplingSHA256: brain.SamplingSHA256, ContextLimit: brain.NativeContextLimit,
			Hardware: brain.Hardware, HardwareAuthoritySource: brain.HardwareAuthoritySource,
			Backend: brain.Backend, BackendVersion: brain.BackendVersion,
		},
	}, nil
}

func finishOracleBaseline(
	request OracleRunRequest,
	authority PairedRunAuthority,
	fixture MicrogauntletCase,
	sealed SealedEpisode,
) (OracleBaselineResult, error) {
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return OracleBaselineResult{}, err
	}
	causal, err := ValidateCausalAcquisitionTrace(fixture, sealed, authority.SurfaceVersion)
	if err != nil {
		return OracleBaselineResult{}, fmt.Errorf("admit oracle causal acquisition trace: %w", err)
	}
	evaluation, err := ScoreSealedEpisode(
		sealed, oracle,
		SymbolicEvaluationEvidence{
			GoalPredicateSatisfied: true, ValidTerminalState: true,
			ActualDecisionCost: oracle.WitnessCost,
		},
	)
	if err != nil {
		return OracleBaselineResult{}, err
	}
	evaluation.CausalAcquisition = &causal
	if err := SealEvaluation(request.EvaluationPath, evaluation, sealed, oracle); err != nil {
		return OracleBaselineResult{}, err
	}
	metric, err := evaluation.EfficiencyMetric()
	if err != nil {
		return OracleBaselineResult{}, err
	}
	variant, err := BindVariantResult(authority, VariantDeterministicOracle, sealed, evaluation)
	if err != nil {
		return OracleBaselineResult{}, err
	}
	result := OracleBaselineResult{
		Purpose: BaselineWorldValidation, Authority: authority, Variant: variant, Episode: sealed,
		Oracle: oracle, Evaluation: evaluation, Efficiency: metric, CausalAcquisition: causal,
	}
	if err := result.Validate(); err != nil {
		return OracleBaselineResult{}, err
	}
	return result, nil
}
