package cognitiongauntlet

import (
	"fmt"
	"sort"
)

func (fixture MicrogauntletCase) TransferAuthority(
	surfaces []Surface,
	generation RatGeneration,
	variant Variant,
	repetition int,
	runtime RuntimeFingerprint,
) (TransferAuthority, error) {
	if err := fixture.spec.Validate(); err != nil {
		return TransferAuthority{}, err
	}
	versions, err := sortedSurfaceVersions(surfaces)
	if err != nil {
		return TransferAuthority{}, err
	}
	public, err := fixture.PublicManifest(SurfaceSymbolic)
	if err != nil {
		return TransferAuthority{}, err
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return TransferAuthority{}, err
	}
	budget, err := NewExecutableRunBudgetV2(
		fixture.spec.Budget, generation.Fixed.Brain.Sampling,
	)
	if err != nil {
		return TransferAuthority{}, err
	}
	authority := TransferAuthority{
		Schema: TransferAuthoritySchemaV1, CaseID: fixture.spec.CaseID,
		TaskSuite: public.Suite, FixtureVersion: fixture.spec.FixtureVersion,
		GeneratorVersion: fixture.spec.Generator.GeneratorVersion,
		Seed:             fixture.spec.Generator.Seed, Scenario: public.Scenario,
		OracleSHA256:         oracle.OracleSHA256,
		ActionCatalogVersion: public.ActionCatalogVersion,
		ActionCatalogSHA256:  public.ActionCatalogSHA256,
		SurfaceVersions:      versions, Variant: variant, Repetition: repetition,
		RatGeneration: generation, Budget: budget, Runtime: runtime,
	}
	return authority, authority.Validate()
}

func BindTransferEpisode(
	authority TransferAuthority,
	run OracleBaselineResult,
) (TransferEpisodeResult, error) {
	if err := run.Validate(); err != nil {
		return TransferEpisodeResult{}, err
	}
	return BindTransferVariant(
		authority, run.Variant, run.Episode, run.Evaluation, run.CausalAcquisition,
	)
}

func BindTransferVariant(
	authority TransferAuthority,
	variant VariantResult,
	episode SealedEpisode,
	evaluation Evaluation,
	causal CausalAcquisitionReport,
) (TransferEpisodeResult, error) {
	if err := authority.Validate(); err != nil {
		return TransferEpisodeResult{}, err
	}
	if err := ValidateVariantEpisode(variant, episode); err != nil {
		return TransferEpisodeResult{}, err
	}
	if err := evaluation.Validate(); err != nil {
		return TransferEpisodeResult{}, err
	}
	if evaluation.EpisodeSealSHA256 != episode.SealSHA256 ||
		evaluation.OracleSHA256 != authority.OracleSHA256 {
		return TransferEpisodeResult{}, fmt.Errorf("transfer evaluation does not bind its episode and oracle")
	}
	cleanDeskQualified := evaluation.CleanDesk != nil && evaluation.CleanDesk.ConcentrationQualified
	if authority.Variant == VariantFullCognition && !cleanDeskQualified {
		return TransferEpisodeResult{}, fmt.Errorf("full cognition transfer omitted critical projected evidence")
	}
	if err := causal.Validate(); err != nil {
		return TransferEpisodeResult{}, err
	}
	if evaluation.CausalAcquisition == nil {
		return TransferEpisodeResult{}, fmt.Errorf("transfer evaluation lacks causal acquisition authority")
	}
	evaluationCausalSHA, err := digestJSON(*evaluation.CausalAcquisition)
	if err != nil {
		return TransferEpisodeResult{}, err
	}
	causalSHA, err := digestJSON(causal)
	if err != nil || evaluationCausalSHA != causalSHA {
		return TransferEpisodeResult{}, fmt.Errorf("transfer evaluation changed its causal acquisition report")
	}
	paired := variant.Authority
	if paired.CaseID != authority.CaseID || paired.Suite != authority.TaskSuite ||
		paired.FixtureVersion != authority.FixtureVersion ||
		paired.GeneratorVersion != authority.GeneratorVersion || paired.Seed != authority.Seed ||
		paired.Scenario != authority.Scenario || paired.OracleSHA256 != authority.OracleSHA256 ||
		paired.ActionCatalogVersion != authority.ActionCatalogVersion ||
		paired.ActionCatalogSHA256 != authority.ActionCatalogSHA256 ||
		paired.Repetition != authority.Repetition || paired.RatGeneration.FixedSHA256 != authority.RatGeneration.FixedSHA256 ||
		paired.RatGeneration.Runtime != authority.RatGeneration.Runtime || paired.Budget != authority.Budget ||
		paired.Runtime != authority.Runtime {
		return TransferEpisodeResult{}, fmt.Errorf("transfer episode changed latent task or cognition authority")
	}
	if !containsString(authority.SurfaceVersions, paired.SurfaceVersion) {
		return TransferEpisodeResult{}, fmt.Errorf("transfer episode used an unregistered surface")
	}
	if variant.Variant != authority.Variant {
		return TransferEpisodeResult{}, fmt.Errorf("transfer episode changed its frozen variant")
	}
	if causal.EpisodeSealSHA256 != episode.SealSHA256 ||
		causal.OracleSHA256 != authority.OracleSHA256 ||
		causal.SurfaceVersion != paired.SurfaceVersion {
		return TransferEpisodeResult{}, fmt.Errorf("transfer causal acquisition changed episode, oracle, or surface")
	}
	authoritySHA, err := digestJSON(authority)
	if err != nil {
		return TransferEpisodeResult{}, err
	}
	result := TransferEpisodeResult{
		AuthoritySHA256: authoritySHA, SurfaceVersion: paired.SurfaceVersion,
		Variant: variant.Variant, EpisodeSealSHA256: episode.SealSHA256,
		GoalSuccess: evaluation.GoalSuccess, CleanDeskQualified: cleanDeskQualified,
		CausalAcquisition: causal,
	}
	return result, result.Validate()
}

func (result TransferEpisodeResult) Validate() error {
	if !validDigest(result.AuthoritySHA256) || !validDigest(result.EpisodeSealSHA256) ||
		!validVariant(result.Variant) {
		return fmt.Errorf("transfer episode identity is invalid")
	}
	if err := result.CausalAcquisition.Validate(); err != nil {
		return err
	}
	if result.CausalAcquisition.EpisodeSealSHA256 != result.EpisodeSealSHA256 ||
		result.CausalAcquisition.SurfaceVersion != result.SurfaceVersion {
		return fmt.Errorf("transfer episode causal acquisition is not bound")
	}
	if result.GoalSuccess &&
		result.CausalAcquisition.AcquiredEvidence != result.CausalAcquisition.RequiredEvidence {
		return fmt.Errorf("successful transfer episode lacks complete causal acquisition")
	}
	if result.Variant == VariantFullCognition && !result.CleanDeskQualified {
		return fmt.Errorf("full cognition transfer lacks clean-desk qualification")
	}
	return requireExact(result.SurfaceVersion, "transfer episode surface version", 256)
}

func EvaluateTransferRail(
	authority TransferAuthority,
	results []TransferEpisodeResult,
) (TransferRailReport, error) {
	if err := authority.Validate(); err != nil {
		return TransferRailReport{}, err
	}
	authoritySHA, err := digestJSON(authority)
	if err != nil {
		return TransferRailReport{}, err
	}
	bySurface := make(map[string]TransferEpisodeResult, len(results))
	for index, result := range results {
		if err := result.Validate(); err != nil {
			return TransferRailReport{}, fmt.Errorf("transfer result %d: %w", index, err)
		}
		if result.AuthoritySHA256 != authoritySHA ||
			result.CausalAcquisition.OracleSHA256 != authority.OracleSHA256 ||
			!containsString(authority.SurfaceVersions, result.SurfaceVersion) {
			return TransferRailReport{}, fmt.Errorf("transfer result %d changed its frozen authority", index)
		}
		if _, duplicate := bySurface[result.SurfaceVersion]; duplicate {
			return TransferRailReport{}, fmt.Errorf("transfer surface %q is duplicated", result.SurfaceVersion)
		}
		bySurface[result.SurfaceVersion] = result
	}
	successful := make([]string, 0, len(results))
	for _, version := range authority.SurfaceVersions {
		result, exists := bySurface[version]
		if !exists {
			continue
		}
		if result.GoalSuccess && (result.Variant != VariantFullCognition || result.CleanDeskQualified) &&
			result.CausalAcquisition.AcquiredEvidence ==
				result.CausalAcquisition.RequiredEvidence {
			successful = append(successful, version)
		}
	}
	gate := EvaluateTransferGate(TransferGateInput{
		HeldOutSurfaces:    append([]string(nil), authority.SurfaceVersions...),
		SuccessfulSurfaces: successful,
	})
	if authority.Variant != VariantFullCognition {
		gate.Passed = false
		gate.Reasons = append(gate.Reasons, "transfer promotion requires the full cognition variant")
	}
	ordered := append([]TransferEpisodeResult(nil), results...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].SurfaceVersion < ordered[right].SurfaceVersion
	})
	return TransferRailReport{
		Authority: authority, Episodes: ordered, Gate: gate,
	}, nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
