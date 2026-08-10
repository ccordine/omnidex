package cognitiongauntlet

import (
	"fmt"
	"reflect"
)

type offlineTransferArtifacts struct {
	surface      Surface
	bundle       PublicInferenceBundle
	promotion    OfflinePromotionReceipt
	promotionSHA string
	episode      SealedEpisode
	evaluation   Evaluation
}

func loadOfflineTransferArtifacts(
	config OfflineTransferConfig,
	registration OfflineTransferPreregistration,
	surface Surface,
) (offlineTransferArtifacts, error) {
	runConfig, err := config.derivedRunConfig(registration, surface)
	if err != nil {
		return offlineTransferArtifacts{}, err
	}
	paths := runConfig.Paths()
	bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
	if err != nil {
		return offlineTransferArtifacts{}, err
	}
	promotion, promotionSHA, err := loadOfflinePromotionReceiptArtifact(paths.Receipt)
	if err != nil {
		return offlineTransferArtifacts{}, err
	}
	evaluation, err := promotion.VerifyEvaluationArtifact(paths.Evaluation)
	if err != nil {
		return offlineTransferArtifacts{}, err
	}
	episode, err := LoadSealedEpisode(paths.Episode)
	if err != nil {
		return offlineTransferArtifacts{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return offlineTransferArtifacts{}, err
	}
	wantVersion, _ := surface.Version()
	if bundle.Authority.SurfaceVersion != wantVersion ||
		bundle.Authority.Variant != VariantFullCognition ||
		bundle.Authority.RatGeneration != registration.Fixed.RatGeneration ||
		bundle.Authority.Budget != registration.Fixed.Budget ||
		bundle.Authority.Runtime != registration.Fixed.RuntimeFingerprint ||
		bundle.Authority.Repetition != registration.Plan.Repetition ||
		promotion.PublicRunAuthoritySHA256 != publicSHA ||
		promotion.EpisodeSealSHA256 != episode.SealSHA256 ||
		evaluation.EpisodeSealSHA256 != episode.SealSHA256 ||
		evaluation.Seed != registration.Plan.Seed {
		return offlineTransferArtifacts{}, fmt.Errorf("offline Transfer surface artifacts changed authority")
	}
	if err := validatePublicInferenceEpisode(bundle, episode); err != nil {
		return offlineTransferArtifacts{}, err
	}
	return offlineTransferArtifacts{
		surface: surface, bundle: bundle, promotion: promotion,
		promotionSHA: promotionSHA, episode: episode, evaluation: evaluation,
	}, nil
}

func deriveOfflineTransferAuthority(
	registration OfflineTransferPreregistration,
	artifacts []offlineTransferArtifacts,
) (TransferAuthority, error) {
	if len(artifacts) != registration.RunCount {
		return TransferAuthority{}, fmt.Errorf("offline Transfer artifacts are incomplete")
	}
	first := artifacts[0]
	fixtureVersion, generatorVersion, err := offlineScenarioVersions(registration.Workload)
	if err != nil {
		return TransferAuthority{}, err
	}
	versions, err := sortedSurfaceVersions(registration.Plan.Surfaces)
	if err != nil {
		return TransferAuthority{}, err
	}
	authority := TransferAuthority{
		Schema: TransferAuthoritySchemaV1, CaseID: registration.Workload.CaseID(),
		TaskSuite: registration.Plan.Suite, FixtureVersion: fixtureVersion,
		GeneratorVersion: generatorVersion, Seed: registration.Plan.Seed,
		Scenario:             first.bundle.Authority.Scenario,
		OracleSHA256:         first.evaluation.OracleSHA256,
		ActionCatalogVersion: first.bundle.Authority.ActionCatalogVersion,
		ActionCatalogSHA256:  first.bundle.Authority.ActionCatalogSHA256,
		SurfaceVersions:      versions, Variant: VariantFullCognition,
		Repetition:    registration.Plan.Repetition,
		RatGeneration: registration.Fixed.RatGeneration, Budget: registration.Fixed.Budget,
		Runtime: registration.Fixed.RuntimeFingerprint,
	}
	if err := validateOfflineTransferAuthority(authority, registration); err != nil {
		return TransferAuthority{}, err
	}
	for index, artifact := range artifacts {
		if artifact.bundle.Authority.Scenario != authority.Scenario ||
			artifact.bundle.Authority.ActionCatalogVersion != authority.ActionCatalogVersion ||
			artifact.bundle.Authority.ActionCatalogSHA256 != authority.ActionCatalogSHA256 ||
			artifact.evaluation.OracleSHA256 != authority.OracleSHA256 ||
			artifact.evaluation.TaskArchetype != offlineScenarioTaskArchetype(registration.Plan.Suite) {
			return TransferAuthority{}, fmt.Errorf("offline Transfer surface %d changed the latent task", index+1)
		}
	}
	return authority, nil
}

func buildOfflineTransferRunReceipt(
	authority TransferAuthority,
	registration OfflineTransferPreregistration,
	artifact offlineTransferArtifacts,
) (OfflineTransferRunReceipt, error) {
	paired := PairedRunAuthority{
		Schema: PairedRunAuthoritySchemaV1, CaseID: authority.CaseID,
		Suite: authority.TaskSuite, FixtureVersion: authority.FixtureVersion,
		GeneratorVersion: authority.GeneratorVersion, Seed: authority.Seed,
		Scenario: authority.Scenario, OracleSHA256: authority.OracleSHA256,
		SurfaceVersion:       artifact.bundle.Authority.SurfaceVersion,
		ActionCatalogVersion: authority.ActionCatalogVersion,
		ActionCatalogSHA256:  authority.ActionCatalogSHA256,
		RatGeneration:        authority.RatGeneration, Budget: authority.Budget,
		Runtime: authority.Runtime, Repetition: authority.Repetition,
	}
	if err := ValidatePublicRunAuthorityProjection(paired, artifact.bundle.Authority); err != nil {
		return OfflineTransferRunReceipt{}, err
	}
	if artifact.evaluation.CausalAcquisition == nil {
		return OfflineTransferRunReceipt{}, fmt.Errorf("offline Transfer evaluation lacks causal acquisition")
	}
	variant := VariantResult{
		Authority: paired, Variant: VariantFullCognition,
		EpisodeSealSHA256: artifact.episode.SealSHA256,
	}
	result, err := BindTransferVariant(
		authority, variant, artifact.episode, artifact.evaluation,
		*artifact.evaluation.CausalAcquisition,
	)
	if err != nil {
		return OfflineTransferRunReceipt{}, err
	}
	run := OfflineTransferRunReceipt{
		SurfaceVersion:           artifact.bundle.Authority.SurfaceVersion,
		PromotionReceiptSHA256:   artifact.promotionSHA,
		PublicRunAuthoritySHA256: artifact.promotion.PublicRunAuthoritySHA256,
		EvaluationArtifactSHA256: artifact.promotion.EvaluationArtifactSHA256,
		Result:                   result, InferenceStartedAt: artifact.promotion.InferenceStartedAt,
		InferenceExitedAt:    artifact.promotion.InferenceExitedAt,
		EvaluatorStartedAt:   artifact.promotion.EvaluatorStartedAt,
		EvaluatorCompletedAt: artifact.promotion.CompletedAt,
	}
	authoritySHA, _ := digestJSON(authority)
	if err := run.validate(
		artifact.bundle.Authority.SurfaceVersion, authoritySHA, registration.RegisteredAt,
	); err != nil {
		return OfflineTransferRunReceipt{}, err
	}
	return run, nil
}

func offlineScenarioVersions(spec OfflineScenarioSpec) (string, string, error) {
	if spec.Initial != nil {
		return spec.Initial.FixtureVersion, spec.Initial.Generator.GeneratorVersion, nil
	}
	if spec.Extended != nil {
		return spec.Extended.FixtureVersion, spec.Extended.Generator.GeneratorVersion, nil
	}
	return "", "", fmt.Errorf("offline Transfer scenario has no registered kind")
}

func equalOfflineTransferReceipt(left, right OfflineTransferReceipt) bool {
	return reflect.DeepEqual(left, right)
}
