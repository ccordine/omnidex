package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

// VerifiedAblationSemanticReplay is issued only after product-owned typed
// rederivation and exact Matrix preregistration binding. Generic replay
// verification cannot construct this value.
type VerifiedAblationSemanticReplay struct {
	verified        cognitionreplay.VerifiedBase
	preregistration matrixReplayPreregistration
	class           AblationReplayClass
}

func (verified VerifiedAblationSemanticReplay) SHA256() string {
	return verified.verified.SHA256()
}

func (verified VerifiedAblationSemanticReplay) Class() AblationReplayClass {
	return verified.class
}

func (verified VerifiedAblationSemanticReplay) RequireSeriousExecution() error {
	if verified.verified.SHA256() == "" || verified.preregistration.validate() != nil ||
		verified.class != AblationReplaySerious ||
		verified.preregistration.variant == VariantRawShell ||
		verified.preregistration.variant == VariantOracleEvidence {
		return fmt.Errorf("ablation semantic replay is not verified serious execution")
	}
	return nil
}

func VerifyAblationSemanticReplayFor(
	raw []byte,
	preregistered matrixReplayPreregistration,
) (VerifiedAblationSemanticReplay, error) {
	if err := preregistered.validate(); err != nil {
		return VerifiedAblationSemanticReplay{}, err
	}
	wantClass, private, err := ablationReplayClass(preregistered.variant)
	if err != nil || private {
		return VerifiedAblationSemanticReplay{}, fmt.Errorf(
			"ablation semantic replay requires a public preregistered ablation coordinate",
		)
	}
	verified, err := cognitionreplay.VerifyBase(raw)
	if err != nil {
		return VerifiedAblationSemanticReplay{}, err
	}
	projection, err := verifyAblationSemanticProjection(verified)
	if err != nil {
		return VerifiedAblationSemanticReplay{}, err
	}
	if projection.class != wantClass || projection.bundle.Authority != preregistered.authority ||
		preregistered.binds(
			projection.bundle.Authority, projection.episode.Manifest.EpisodeID,
			projection.episode.Manifest.EpisodeStartedAt,
		) != nil || preregistered.bindsExecution(projection.episode.Manifest) != nil {
		return VerifiedAblationSemanticReplay{}, fmt.Errorf(
			"ablation semantic replay differs from preregistered authority",
		)
	}
	return VerifiedAblationSemanticReplay{
		verified: verified, preregistration: preregistered, class: wantClass,
	}, nil
}
