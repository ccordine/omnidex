package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const AblationEvidenceExpectationSchemaV1 = "omnidex.ablation-evidence-expectation.v1"

// AblationEvidenceExpectation is frozen from the preregistered public run
// authority and the cold-loaded episode. It never derives coordinates from the
// evidence artifact it is about to verify.
type AblationEvidenceExpectation struct {
	Schema                string              `json:"schema"`
	PublicRunAuthority    PublicRunAuthority  `json:"public_run_authority"`
	EpisodeID             cognition.EpisodeID `json:"episode_id"`
	EpisodeSealSHA256     string              `json:"episode_seal_sha256"`
	ExpectedEvidenceClass AblationReplayClass `json:"expected_evidence_class"`
}

func NewAblationEvidenceExpectation(
	authority PublicRunAuthority,
	sealed SealedEpisode,
) (AblationEvidenceExpectation, error) {
	class, _, err := ablationReplayClass(authority.Variant)
	if err != nil {
		return AblationEvidenceExpectation{}, err
	}
	value := AblationEvidenceExpectation{
		Schema:             AblationEvidenceExpectationSchemaV1,
		PublicRunAuthority: authority, EpisodeID: sealed.Manifest.EpisodeID,
		EpisodeSealSHA256: sealed.SealSHA256, ExpectedEvidenceClass: class,
	}
	if err := value.Validate(); err != nil {
		return AblationEvidenceExpectation{}, err
	}
	if err := validateExpectedAblationEpisode(value, sealed); err != nil {
		return AblationEvidenceExpectation{}, err
	}
	return value, nil
}

func (value AblationEvidenceExpectation) Validate() error {
	class, _, err := ablationReplayClass(value.PublicRunAuthority.Variant)
	episode, episodeErr := PublicVariantEpisodeRef(value.PublicRunAuthority)
	if value.Schema != AblationEvidenceExpectationSchemaV1 || err != nil || episodeErr != nil ||
		value.PublicRunAuthority.Validate() != nil || episode.ID != value.EpisodeID ||
		!validDigest(value.EpisodeSealSHA256) || value.ExpectedEvidenceClass != class {
		return fmt.Errorf("ablation evidence expectation is invalid")
	}
	return nil
}

// VerifiedAblationEvidence can be constructed only by the cold product
// verifier. It is exact evidence for a later semantic replay exporter; it is
// intentionally not itself a serious-execution qualification.
type VerifiedAblationEvidence struct {
	authority   AblationEvidenceAuthority
	class       AblationReplayClass
	expectation AblationEvidenceExpectation
}

func (verified VerifiedAblationEvidence) SHA256() string {
	return verified.authority.SHA256
}

func (verified VerifiedAblationEvidence) Class() AblationReplayClass {
	return verified.class
}

func (verified VerifiedAblationEvidence) Authority() AblationEvidenceAuthority {
	return verified.authority
}

func VerifyAblationEvidenceFor(
	path string,
	sealed SealedEpisode,
	expected AblationEvidenceExpectation,
) (VerifiedAblationEvidence, error) {
	if err := expected.Validate(); err != nil {
		return VerifiedAblationEvidence{}, err
	}
	if err := validateExpectedAblationEpisode(expected, sealed); err != nil {
		return VerifiedAblationEvidence{}, err
	}
	authority, err := ablationEvidenceAuthorityFromEpisode(sealed)
	if err != nil {
		return VerifiedAblationEvidence{}, err
	}
	artifact, err := loadAblationEvidence(path, authority)
	if err != nil {
		return VerifiedAblationEvidence{}, err
	}
	root := artifact.Root
	publicSHA, err := expected.PublicRunAuthority.SHA256()
	if err != nil || root.PublicRunAuthority != expected.PublicRunAuthority ||
		root.PublicRunAuthoritySHA256 != publicSHA || root.EpisodeID != expected.EpisodeID ||
		root.Variant != expected.PublicRunAuthority.Variant ||
		root.Class != expected.ExpectedEvidenceClass ||
		root.Terminal.Revision != sealed.Manifest.FinalRevision ||
		root.Terminal.PublicOutcome != sealed.Manifest.Outcome.PublicOutcome ||
		root.Terminal.GoalSatisfied != sealed.Manifest.Outcome.GoalSatisfied ||
		root.Terminal.FailureCode != sealed.Manifest.Outcome.FailureCode {
		return VerifiedAblationEvidence{}, fmt.Errorf(
			"sealed episode differs from its ablation evidence root",
		)
	}
	return VerifiedAblationEvidence{
		authority: authority, class: root.Class, expectation: expected,
	}, nil
}

func validateExpectedAblationEpisode(
	expected AblationEvidenceExpectation,
	sealed SealedEpisode,
) error {
	publicSHA, err := expected.PublicRunAuthority.SHA256()
	if err != nil || sealed.Validate() != nil || sealed.Manifest.EpisodeID != expected.EpisodeID ||
		sealed.SealSHA256 != expected.EpisodeSealSHA256 ||
		sealed.Manifest.PublicRunAuthoritySHA256 != publicSHA ||
		sealed.Manifest.Variant != expected.PublicRunAuthority.Variant ||
		sealed.Manifest.Scenario != expected.PublicRunAuthority.Scenario {
		return fmt.Errorf("sealed episode differs from preregistered ablation authority")
	}
	return nil
}
