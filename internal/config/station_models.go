package config

import (
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/station"
)

func loadStationModels(_ Config) map[station.ID]string {
	keys := map[station.ID]string{
		station.ContextRelevance:                              "OMNI_CONTEXT_RELEVANCE_MODEL",
		station.ContextMinification:                           "OMNI_CONTEXT_MINIFICATION_MODEL",
		station.ConversationObjectiveKind:                     "OMNI_CONVERSATION_OBJECTIVE_KIND_MODEL",
		station.ConversationResponse:                          "OMNI_CONVERSATION_RESPONSE_MODEL",
		station.GroundedAnswer:                                "OMNI_GROUNDED_ANSWER_MODEL",
		station.DatabaseSchemaSelection:                       "OMNI_DATABASE_SCHEMA_SELECTION_MODEL",
		station.DatabaseQueryIntent:                           "OMNI_DATABASE_QUERY_INTENT_MODEL",
		station.DatabaseJoinPathSelection:                     "OMNI_DATABASE_JOIN_PATH_SELECTION_MODEL",
		station.WebRelevance:                                  "OMNI_WEB_RELEVANCE_MODEL",
		station.WebGroundedSynthesis:                          "OMNI_WEB_GROUNDED_SYNTHESIS_MODEL",
		station.CodingSurface:                                 "OMNI_CODING_SURFACE_MODEL",
		station.CodingRequirements:                            "OMNI_CODING_REQUIREMENTS_MODEL",
		station.CodingProjectStackConstraint:                  "OMNI_CODING_REQUIREMENTS_MODEL",
		station.CodingArtifactHandling:                        "OMNI_CODING_ARTIFACT_HANDLING_MODEL",
		station.CodingCapabilityRelation:                      "OMNI_CODING_CAPABILITY_RELATION_MODEL",
		station.CodingRuntimeCapabilityNecessity:              "OMNI_CODING_CAPABILITY_RELATION_MODEL",
		station.CodingFragment:                                "OMNI_CODING_FRAGMENT_MODEL",
		station.CodingFragmentRepairGuidance:                  "OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL",
		station.CodingFragmentCorrection:                      "OMNI_CODING_FRAGMENT_CORRECTION_MODEL",
	}
	models := make(map[station.ID]string, len(keys))
	for id, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			models[id] = value
		}
	}
	return models
}
