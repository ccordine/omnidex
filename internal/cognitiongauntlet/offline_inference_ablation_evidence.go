package cognitiongauntlet

import "fmt"

func verifyOfflineInferenceAblationEvidence(
	bundle PublicInferenceBundle,
	sealed SealedEpisode,
	evidencePath string,
) (VerifiedAblationEvidence, error) {
	if err := bundle.Validate(); err != nil {
		return VerifiedAblationEvidence{}, err
	}
	if bundle.Authority.Variant == VariantFullCognition ||
		!executableAblation(bundle.Authority.Variant) {
		return VerifiedAblationEvidence{}, fmt.Errorf(
			"offline inference ablation evidence variant is invalid",
		)
	}
	expected, err := NewAblationEvidenceExpectation(bundle.Authority, sealed)
	if err != nil {
		return VerifiedAblationEvidence{}, err
	}
	return VerifyAblationEvidenceFor(evidencePath, sealed, expected)
}
