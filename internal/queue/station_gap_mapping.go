package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func stationForPortableJob(job assemblyline.PortableJob) (station.ID, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	if job.Kind == assemblyline.WorkResponseCorrection {
		var input assemblyline.ResponseCorrectionInput
		if err := decodePortableGapPayload(job.Payload, &input); err != nil {
			return "", err
		}
		return stationForPortableJob(input.Original)
	}
	return stationForPortableWorkKind(job.Kind)
}

// StationForPortableJob returns the sole registered station that owns a
// validated PortableJob. The worker uses this same mapping when it opens the
// durable gap, so dispatch cannot maintain a second work-kind router.
func StationForPortableJob(job assemblyline.PortableJob) (station.ID, error) {
	return stationForPortableJob(job)
}

func stationForPortableWorkKind(kind assemblyline.WorkKind) (station.ID, error) {
	switch kind {
	case assemblyline.WorkApplicationContextNeeds,
		assemblyline.WorkApplicationIntent:
		return station.CodingRequirements, nil
	case assemblyline.WorkApplicationClassify:
		return station.CodingSurface, nil
	case assemblyline.WorkRepositoryRequirements:
		return station.CodingRequirements, nil
	case assemblyline.WorkApplicationJobSpecification:
		return station.CodingWorkload, nil
	case assemblyline.WorkApplicationTargetTree:
		return station.CodingTargetTree, nil
	case assemblyline.WorkApplicationFileContent:
		return station.CodingWorkload, nil
	case assemblyline.WorkApplicationAcceptanceGroundingReview:
		return station.CodingWorkloadReview, nil
	case assemblyline.WorkRepositorySearchTerm:
		return station.CodingRepositorySearchTerm, nil
	case assemblyline.WorkRepositoryChangeSurface:
		return station.CodingRepositoryChange, nil
	case assemblyline.WorkConversationContextSelection:
		return station.ConversationContextSelection, nil
	case assemblyline.WorkMemoryContextSelection:
		return station.MemoryContextSelection, nil
	case assemblyline.WorkConversationObjectiveKind:
		return station.ConversationObjectiveKind, nil
	case assemblyline.WorkConversationResponse:
		return station.ConversationResponse, nil
	case assemblyline.WorkGroundedAnswer:
		return station.GroundedAnswer, nil
	case assemblyline.WorkRepositoryEvidenceRelevance:
		return station.RepositoryEvidenceRelevance, nil
	case assemblyline.WorkRepositoryGroundedReview:
		return station.RepositoryGroundedReview, nil
	case assemblyline.WorkRepositoryGroundedCorrection:
		return station.RepositoryGroundedCorrection, nil
	case assemblyline.WorkWebSearchTerms:
		return station.WebSearchTerms, nil
	case assemblyline.WorkWebRelevance:
		return station.WebRelevance, nil
	case assemblyline.WorkWebGroundedSynthesis:
		return station.WebGroundedSynthesis, nil
	case assemblyline.WorkWebGroundedSynthesisCorrection:
		return station.WebGroundedSynthesisCorrection, nil
	case assemblyline.WorkWebClaimEvidenceReview:
		return station.WebClaimEvidenceReview, nil
	case assemblyline.WorkArtifactHandling:
		return station.CodingArtifactHandling, nil
	case assemblyline.WorkKnownArtifactTruth:
		return station.CodingKnownArtifactTruth, nil
	case assemblyline.WorkDeclarationArtifactBoundary:
		return station.CodingDeclarationArtifactBoundary, nil
	case assemblyline.WorkArtifactCandidateSelection:
		return station.CodingArtifactCandidateSelection, nil
	case assemblyline.WorkCapabilityRelation:
		return station.CodingCapabilityRelation, nil
	case assemblyline.WorkSkillSelection:
		return station.CodingSkillSelection, nil
	case assemblyline.WorkTypeScriptRepairGuidance:
		return station.CodingFragmentRepairGuidance, nil
	case assemblyline.WorkFragmentGeneration, assemblyline.WorkFragmentModification:
		return station.CodingFragment, nil
	case assemblyline.WorkFragmentCorrection:
		return station.CodingFragmentCorrection, nil
	default:
		return "", fmt.Errorf("portable work kind %q is not a production semantic station", kind)
	}
}
