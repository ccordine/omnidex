package worker

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

type repositoryGroundingModelIdentityGuard struct {
	mu         sync.Mutex
	generation *llm.ProviderIdentityExpectation
	review     *llm.ProviderIdentityExpectation
}

func requireIndependentRepositoryReviewRoutes(routing ModelRouting) error {
	answer, err := stationModel(routing, station.GroundedAnswer)
	if err != nil {
		return err
	}
	review, err := stationModel(routing, station.RepositoryGroundedReview)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(answer), strings.TrimSpace(review)) {
		return fmt.Errorf("repository grounded review must use a model route distinct from grounded answer")
	}
	correction, err := stationModel(routing, station.RepositoryGroundedCorrection)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(correction), strings.TrimSpace(review)) {
		return fmt.Errorf("repository grounded review must use a model route distinct from grounded correction")
	}
	return nil
}

func (guard *repositoryGroundingModelIdentityGuard) validate(
	job assemblyline.PortableJob,
	execution exactStationExecution,
) error {
	if guard == nil {
		return fmt.Errorf("repository grounding model identity guard is unavailable")
	}
	identity := execution.ProviderIdentity
	if err := identity.Validate(); err != nil {
		return fmt.Errorf("repository grounding provider identity is invalid: %w", err)
	}
	guard.mu.Lock()
	defer guard.mu.Unlock()
	kind, err := repositoryGroundingOriginalWorkKind(job)
	if err != nil {
		return err
	}
	switch kind {
	case assemblyline.WorkGroundedAnswer:
		owned := identity
		guard.generation = &owned
		guard.review = nil
		return nil
	case assemblyline.WorkRepositoryGroundedCorrection:
		if guard.review != nil && sameRepositoryGroundingIdentity(*guard.review, identity) {
			return fmt.Errorf("repository grounded correction resolved to its independent review model identity")
		}
		owned := identity
		guard.generation = &owned
		return nil
	case assemblyline.WorkRepositoryGroundedReview:
		if guard.generation == nil {
			return fmt.Errorf("repository grounded review ran before repository answer generation identity was proven")
		}
		if sameRepositoryGroundingIdentity(*guard.generation, identity) {
			return fmt.Errorf("repository grounded review resolved to its answer generation model identity")
		}
		owned := identity
		guard.review = &owned
		return nil
	default:
		return fmt.Errorf("repository grounding identity guard received unsupported work kind %q", kind)
	}
}

func repositoryGroundingOriginalWorkKind(job assemblyline.PortableJob) (assemblyline.WorkKind, error) {
	if job.Kind != assemblyline.WorkResponseCorrection {
		return job.Kind, nil
	}
	if err := job.Validate(); err != nil {
		return "", err
	}
	var correction assemblyline.ResponseCorrectionInput
	if err := json.Unmarshal(job.Payload, &correction); err != nil {
		return "", fmt.Errorf("decode repository grounding validation retry: %w", err)
	}
	if correction.Original.Kind == assemblyline.WorkResponseCorrection {
		return "", fmt.Errorf("repository grounding validation retry cannot wrap another correction")
	}
	return correction.Original.Kind, nil
}

func sameRepositoryGroundingIdentity(left, right llm.ProviderIdentityExpectation) bool {
	return left.Backend == right.Backend && left.Digest == right.Digest &&
		left.Quantization == right.Quantization &&
		left.NativeContextLimit == right.NativeContextLimit &&
		left.TokenizerProfile == right.TokenizerProfile
}
