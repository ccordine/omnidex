package cognitiongauntlet

import (
	"fmt"
)

func (fixture MicrogauntletCase) PairedAuthority(
	surface Surface,
	generation RatGeneration,
	repetition int,
	runtime RuntimeFingerprint,
) (PairedRunAuthority, error) {
	if err := fixture.spec.Validate(); err != nil {
		return PairedRunAuthority{}, err
	}
	if err := fixture.generated.Validate(); err != nil {
		return PairedRunAuthority{}, err
	}
	if err := generation.Validate(); err != nil {
		return PairedRunAuthority{}, err
	}
	budget, err := NewExecutableRunBudgetV2(
		fixture.spec.Budget, generation.Fixed.Brain.Sampling,
	)
	if err != nil {
		return PairedRunAuthority{}, err
	}
	public, err := fixture.PublicManifest(surface)
	if err != nil {
		return PairedRunAuthority{}, err
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return PairedRunAuthority{}, err
	}
	authority := PairedRunAuthority{
		Schema: PairedRunAuthoritySchemaV1, CaseID: fixture.spec.CaseID,
		Suite: public.Suite, FixtureVersion: fixture.spec.FixtureVersion,
		GeneratorVersion: fixture.spec.Generator.GeneratorVersion,
		Seed:             fixture.spec.Generator.Seed, Scenario: public.Scenario,
		OracleSHA256: oracle.OracleSHA256, SurfaceVersion: public.SurfaceVersion,
		ActionCatalogVersion: public.ActionCatalogVersion,
		ActionCatalogSHA256:  public.ActionCatalogSHA256,
		RatGeneration:        generation, Budget: budget, Runtime: runtime,
		Repetition: repetition,
	}
	return authority, authority.Validate()
}

func ValidateVariantEpisode(result VariantResult, episode SealedEpisode) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if err := episode.Validate(); err != nil {
		return err
	}
	if result.EpisodeSealSHA256 != episode.SealSHA256 {
		return fmt.Errorf("paired cognition result does not bind the supplied episode")
	}
	authority := result.Authority
	manifest := episode.Manifest
	authoritySHA, err := publicRunAuthoritySHA256(authority, result.Variant)
	if err != nil {
		return fmt.Errorf("hash paired cognition authority: %w", err)
	}
	if manifest.Scenario != authority.Scenario ||
		manifest.PublicRunAuthoritySHA256 != authoritySHA || manifest.Variant != result.Variant ||
		manifest.StationBudget != authority.Budget.Station ||
		manifest.RatGeneration.FixedSHA256 != authority.RatGeneration.FixedSHA256 ||
		manifest.RatGeneration.Runtime != authority.RatGeneration.Runtime {
		return fmt.Errorf("paired cognition episode changed scenario, brain, or runtime authority")
	}
	if manifest.Resources.PeakContextBytes > int64(authority.Budget.ContextBytes) ||
		manifest.Resources.PeakWorkingSetBytes > int64(authority.Budget.WorkingSetBytes) ||
		manifest.Resources.PolicyCallsConsumed > authority.Budget.ModelCalls ||
		manifest.Resources.ModelCalls > authority.Budget.ModelCalls ||
		manifest.Resources.EnvironmentActions > authority.Budget.EnvironmentActions ||
		manifest.Resources.ToolOperations > authority.Budget.ToolOperations {
		return fmt.Errorf("paired cognition episode exceeded its frozen run budget")
	}
	return nil
}

func BindVariantResult(
	authority PairedRunAuthority,
	variant Variant,
	episode SealedEpisode,
	evaluation Evaluation,
) (VariantResult, error) {
	if err := authority.Validate(); err != nil {
		return VariantResult{}, err
	}
	if !validVariant(variant) {
		return VariantResult{}, fmt.Errorf("cognition variant is not registered")
	}
	if err := evaluation.Validate(); err != nil {
		return VariantResult{}, err
	}
	if evaluation.EpisodeSealSHA256 != episode.SealSHA256 ||
		evaluation.OracleSHA256 != authority.OracleSHA256 || evaluation.Seed != authority.Seed {
		return VariantResult{}, fmt.Errorf("variant evaluation does not bind its episode and oracle")
	}
	result := VariantResult{
		Authority: authority, Variant: variant, EpisodeSealSHA256: episode.SealSHA256,
	}
	if err := ValidateVariantEpisode(result, episode); err != nil {
		return VariantResult{}, err
	}
	return result, nil
}
