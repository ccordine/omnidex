package config

import (
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/station"
)

func loadStationModels(_ Config) map[station.ID]string {
	keys := map[station.ID]string{
		station.ContextSearchTerms:                 "OMNI_CONTEXT_SEARCH_TERMS_MODEL",
		station.ContextRelevance:                   "OMNI_CONTEXT_RELEVANCE_MODEL",
		station.ContextMinification:                "OMNI_CONTEXT_MINIFICATION_MODEL",
		station.ConversationObjectiveKind:          "OMNI_CONVERSATION_OBJECTIVE_KIND_MODEL",
		station.ConversationResponse:               "OMNI_CONVERSATION_RESPONSE_MODEL",
		station.GroundedAnswer:                     "OMNI_GROUNDED_ANSWER_MODEL",
		station.DatabaseSchemaSelection:            "OMNI_DATABASE_SCHEMA_SELECTION_MODEL",
		station.DatabaseQueryIntent:                "OMNI_DATABASE_QUERY_INTENT_MODEL",
		station.DatabaseEvidenceGap:                "OMNI_DATABASE_EVIDENCE_GAP_MODEL",
		station.DatabaseJoinPathSelection:          "OMNI_DATABASE_JOIN_PATH_SELECTION_MODEL",
		station.RepositoryEvidenceRelevance:        "OMNI_REPOSITORY_EVIDENCE_RELEVANCE_MODEL",
		station.RepositoryGroundedReview:           "OMNI_REPOSITORY_GROUNDED_REVIEW_MODEL",
		station.RepositoryGroundedCorrection:       "OMNI_REPOSITORY_GROUNDED_CORRECTION_MODEL",
		station.WebSearchTerms:                     "OMNI_WEB_SEARCH_TERMS_MODEL",
		station.WebRelevance:                       "OMNI_WEB_RELEVANCE_MODEL",
		station.WebGroundedSynthesis:               "OMNI_WEB_GROUNDED_SYNTHESIS_MODEL",
		station.WebGroundedSynthesisCorrection:     "OMNI_WEB_GROUNDED_SYNTHESIS_CORRECTION_MODEL",
		station.WebClaimEvidenceReview:             "OMNI_WEB_CLAIM_EVIDENCE_REVIEW_MODEL",
		station.CodingSurface:                      "OMNI_CODING_SURFACE_MODEL",
		station.CodingRequirements:                 "OMNI_CODING_REQUIREMENTS_MODEL",
		station.CodingProjectStackConstraint:       "OMNI_CODING_REQUIREMENTS_MODEL",
		station.CodingServiceDeploymentIntent:      "OMNI_CODING_REQUIREMENTS_MODEL",
		station.CodingServiceStateLifetime:         "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingServiceStateInterface:        "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingServiceEndpointRequirement:   "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingServiceEndpointExposure:      "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingServiceEndpointMethod:        "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingServiceEndpointRouteTemplate: "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingServiceEndpointRequestMedia:  "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingServiceEndpointResponseMedia: "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingServiceEndpointSuccessStatus: "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingWorkload:                     "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingTargetTree:                   "OMNI_CODING_WORKLOAD_MODEL",
		station.CodingArtifactHandling:             "OMNI_CODING_ARTIFACT_HANDLING_MODEL",
		station.CodingKnownArtifactTruth:           "OMNI_CODING_ARTIFACT_HANDLING_MODEL",
		station.CodingDeclarationArtifactBoundary:  "OMNI_CODING_ARTIFACT_HANDLING_MODEL",
		station.CodingArtifactCandidateSelection:   "OMNI_CODING_ARTIFACT_HANDLING_MODEL",
		station.CodingCapabilityRelation:           "OMNI_CODING_CAPABILITY_RELATION_MODEL",
		station.CodingSkillSelection:               "OMNI_CODING_SKILL_SELECTION_MODEL",
		station.CodingFragment:                     "OMNI_CODING_FRAGMENT_MODEL",
		station.CodingFragmentRepairGuidance:       "OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL",
		station.CodingFragmentCorrection:           "OMNI_CODING_FRAGMENT_CORRECTION_MODEL",
		station.CodingRepositorySearchTerm:         "OMNI_CODING_REPOSITORY_SEARCH_TERM_MODEL",
		station.CodingRepositoryChange:             "OMNI_CODING_REPOSITORY_CHANGE_SURFACE_MODEL",
	}
	models := make(map[station.ID]string, len(keys))
	for id, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			models[id] = value
		}
	}
	return models
}
