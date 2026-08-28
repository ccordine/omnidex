package worker

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

type webModelIdentityGuard struct {
	mu         sync.Mutex
	generation *llm.ProviderIdentityExpectation
}

func requireIndependentWebReviewRoutes(routing ModelRouting) error {
	synthesis, err := stationModel(routing, station.WebGroundedSynthesis)
	if err != nil {
		return err
	}
	review, err := stationModel(routing, station.WebClaimEvidenceReview)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(synthesis), strings.TrimSpace(review)) {
		return fmt.Errorf("web claim-evidence review must use a model route distinct from grounded synthesis")
	}
	correction, err := stationModel(routing, station.WebGroundedSynthesisCorrection)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(correction), strings.TrimSpace(review)) {
		return fmt.Errorf("web claim-evidence review must use a model route distinct from synthesis correction")
	}
	return nil
}

func (guard *webModelIdentityGuard) validate(
	job assemblyline.PortableJob,
	execution exactStationExecution,
) error {
	if guard == nil {
		return fmt.Errorf("web model identity guard is unavailable")
	}
	identity := execution.ProviderIdentity
	if err := identity.Validate(); err != nil {
		return fmt.Errorf("web station provider identity is invalid: %w", err)
	}
	guard.mu.Lock()
	defer guard.mu.Unlock()
	switch job.Kind {
	case assemblyline.WorkWebSynthesisParagraphCoverage,
		assemblyline.WorkWebSynthesisParagraph,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebGroundedSynthesisCorrection:
		owned := identity
		guard.generation = &owned
		return nil
	case assemblyline.WorkWebReviewClaimCoverage,
		assemblyline.WorkWebReviewClaim,
		assemblyline.WorkWebReviewClaimVerdict,
		assemblyline.WorkWebReviewIssueEvidenceRelation,
		assemblyline.WorkWebReviewIssueDetail:
		if guard.generation == nil {
			return fmt.Errorf("web claim-evidence review ran before grounded synthesis identity was proven")
		}
		if sameExactWebModelIdentity(*guard.generation, identity) {
			return fmt.Errorf("web claim-evidence review resolved to its synthesis generation model identity")
		}
		return nil
	default:
		return nil
	}
}

func sameExactWebModelIdentity(left, right llm.ProviderIdentityExpectation) bool {
	return left.Backend == right.Backend &&
		left.Digest == right.Digest &&
		left.Quantization == right.Quantization &&
		left.NativeContextLimit == right.NativeContextLimit &&
		left.TokenizerProfile == right.TokenizerProfile
}
