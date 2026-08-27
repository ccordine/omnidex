package modelconfig

import (
	"github.com/gryph/omnidex/internal/station"
)

type Routing struct {
	Stations              map[station.ID]string
	RoleplaySemanticModel string
}

func Apply(base Routing, cfg Config) Routing {
	out := base
	if out.Stations == nil {
		out.Stations = map[station.ID]string{}
	} else {
		clone := map[station.ID]string{}
		for key, value := range out.Stations {
			clone[key] = value
		}
		out.Stations = clone
	}
	if value := cfg.Get("context_search_terms_model"); value != "" {
		out.Stations[station.ContextSearchTerms] = value
	}
	if value := cfg.Get("context_relevance_model"); value != "" {
		out.Stations[station.ContextRelevance] = value
	}
	if value := cfg.Get("context_minification_model"); value != "" {
		out.Stations[station.ContextMinification] = value
	}
	if value := cfg.Get("conversation_objective_kind_model"); value != "" {
		out.Stations[station.ConversationObjectiveKind] = value
	}
	if value := cfg.Get("conversation_response_model"); value != "" {
		out.Stations[station.ConversationResponse] = value
	}
	if value := cfg.Get("roleplay_semantic_model"); value != "" {
		out.RoleplaySemanticModel = value
	}
	if value := cfg.Get("grounded_answer_model"); value != "" {
		out.Stations[station.GroundedAnswer] = value
	}
	if value := cfg.Get("database_schema_selection_model"); value != "" {
		out.Stations[station.DatabaseSchemaSelection] = value
	}
	if value := cfg.Get("database_query_intent_model"); value != "" {
		out.Stations[station.DatabaseQueryIntent] = value
	}
	if value := cfg.Get("database_evidence_gap_model"); value != "" {
		out.Stations[station.DatabaseEvidenceGap] = value
	}
	if value := cfg.Get("database_join_path_selection_model"); value != "" {
		out.Stations[station.DatabaseJoinPathSelection] = value
	}
	if value := cfg.Get("repository_evidence_relevance_model"); value != "" {
		out.Stations[station.RepositoryEvidenceRelevance] = value
	}
	if value := cfg.Get("repository_grounded_review_model"); value != "" {
		out.Stations[station.RepositoryGroundedReview] = value
	}
	if value := cfg.Get("repository_grounded_correction_model"); value != "" {
		out.Stations[station.RepositoryGroundedCorrection] = value
	}
	if value := cfg.Get("web_search_terms_model"); value != "" {
		out.Stations[station.WebSearchTerms] = value
	}
	if value := cfg.Get("web_relevance_model"); value != "" {
		out.Stations[station.WebRelevance] = value
	}
	if value := cfg.Get("web_grounded_synthesis_model"); value != "" {
		out.Stations[station.WebGroundedSynthesis] = value
	}
	if value := cfg.Get("web_grounded_synthesis_correction_model"); value != "" {
		out.Stations[station.WebGroundedSynthesisCorrection] = value
	}
	if value := cfg.Get("web_claim_evidence_review_model"); value != "" {
		out.Stations[station.WebClaimEvidenceReview] = value
	}
	if value := cfg.Get("coding_surface_model"); value != "" {
		out.Stations[station.CodingSurface] = value
	}
	if value := cfg.Get("coding_requirements_model"); value != "" {
		out.Stations[station.CodingRequirements] = value
		out.Stations[station.CodingProjectStackConstraint] = value
		out.Stations[station.CodingServiceDeploymentIntent] = value
	}
	if value := cfg.Get("coding_workload_model"); value != "" {
		out.Stations[station.CodingWorkload] = value
		out.Stations[station.CodingTargetTree] = value
		out.Stations[station.CodingServiceStateLifetime] = value
		out.Stations[station.CodingServiceStateInterface] = value
		out.Stations[station.CodingServiceEndpointRequirement] = value
		out.Stations[station.CodingServiceEndpointExposure] = value
		out.Stations[station.CodingServiceEndpointMethod] = value
		out.Stations[station.CodingServiceEndpointRouteTemplate] = value
		out.Stations[station.CodingServiceEndpointRequestMedia] = value
		out.Stations[station.CodingServiceEndpointResponseMedia] = value
		out.Stations[station.CodingServiceEndpointSuccessStatus] = value
	}
	if value := cfg.Get("coding_artifact_handling_model"); value != "" {
		out.Stations[station.CodingArtifactHandling] = value
		out.Stations[station.CodingKnownArtifactTruth] = value
		out.Stations[station.CodingDeclarationArtifactBoundary] = value
		out.Stations[station.CodingArtifactCandidateSelection] = value
	}
	if value := cfg.Get("coding_capability_relation_model"); value != "" {
		out.Stations[station.CodingCapabilityRelation] = value
	}
	if value := cfg.Get("coding_skill_selection_model"); value != "" {
		out.Stations[station.CodingSkillSelection] = value
	}
	if value := cfg.Get("coding_fragment_model"); value != "" {
		out.Stations[station.CodingFragment] = value
	}
	if value := cfg.Get("coding_fragment_repair_guidance_model"); value != "" {
		out.Stations[station.CodingFragmentRepairGuidance] = value
	}
	if value := cfg.Get("coding_fragment_correction_model"); value != "" {
		out.Stations[station.CodingFragmentCorrection] = value
	}
	if value := cfg.Get("coding_repository_search_term_model"); value != "" {
		out.Stations[station.CodingRepositorySearchTerm] = value
	}
	if value := cfg.Get("coding_repository_change_surface_model"); value != "" {
		out.Stations[station.CodingRepositoryChange] = value
	}
	return out
}
