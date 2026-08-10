package cognitiongauntlet

import "fmt"

type offlineScaleArtifacts struct {
	current       OfflineScaleCase
	bundle        PublicInferenceBundle
	promotion     OfflinePromotionReceipt
	promotionSHA  string
	episode       SealedEpisode
	evaluation    Evaluation
	scaleEvidence OfflineScaleEvaluationEvidence
	scaleSHA      string
}

func loadOfflineScaleArtifacts(
	config OfflineScaleConfig,
	registration OfflineScalePreregistration,
	current OfflineScaleCase,
) (offlineScaleArtifacts, error) {
	if !containsOfflineScaleCase(registration.Cases, current) {
		return offlineScaleArtifacts{}, fmt.Errorf("offline Scale coordinate was not preregistered")
	}
	paths := config.runPaths(current)
	bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
	if err != nil {
		return offlineScaleArtifacts{}, err
	}
	promotion, promotionSHA, err := loadOfflinePromotionReceiptArtifact(paths.Receipt)
	if err != nil {
		return offlineScaleArtifacts{}, err
	}
	evaluation, err := promotion.VerifyEvaluationArtifact(paths.Evaluation)
	if err != nil {
		return offlineScaleArtifacts{}, err
	}
	episode, err := LoadSealedEpisode(paths.Episode)
	if err != nil {
		return offlineScaleArtifacts{}, err
	}
	evidence, scaleSHA, err := LoadOfflineScaleEvaluationEvidence(config.scaleEvidencePath(current))
	if err != nil {
		return offlineScaleArtifacts{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return offlineScaleArtifacts{}, err
	}
	if bundle.Authority.Variant != VariantFullCognition ||
		bundle.Authority.SurfaceVersion != "symbolic.v1" ||
		bundle.Authority.RatGeneration != registration.Fixed.RatGeneration ||
		bundle.Authority.Budget != registration.Fixed.Budget ||
		bundle.Authority.Runtime != registration.Fixed.RuntimeFingerprint ||
		bundle.Authority.Repetition != current.Repetition ||
		promotion.PublicRunAuthoritySHA256 != publicSHA ||
		promotion.EpisodeSealSHA256 != episode.SealSHA256 ||
		promotion.EvaluationArtifactSHA256 != evidence.EvaluationArtifactSHA256 ||
		evidence.Case != current || evidence.Scenario != bundle.Authority.Scenario ||
		evidence.OracleSHA256 != evaluation.OracleSHA256 ||
		evidence.EpisodeSealSHA256 != episode.SealSHA256 {
		return offlineScaleArtifacts{}, fmt.Errorf("offline Scale run artifacts changed authority")
	}
	if err := validatePublicInferenceEpisode(bundle, episode); err != nil {
		return offlineScaleArtifacts{}, err
	}
	return offlineScaleArtifacts{
		current: current, bundle: bundle, promotion: promotion, promotionSHA: promotionSHA,
		episode: episode, evaluation: evaluation, scaleEvidence: evidence, scaleSHA: scaleSHA,
	}, nil
}

func loadAllOfflineScaleArtifacts(
	config OfflineScaleConfig,
	registration OfflineScalePreregistration,
) ([]offlineScaleArtifacts, error) {
	artifacts := make([]offlineScaleArtifacts, len(registration.Cases))
	for index, current := range registration.Cases {
		artifact, err := loadOfflineScaleArtifacts(config, registration, current)
		if err != nil {
			return nil, fmt.Errorf("load Scale run %d: %w", index+1, err)
		}
		artifacts[index] = artifact
	}
	return artifacts, nil
}
