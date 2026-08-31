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
	if value := cfg.Get("database_join_path_selection_model"); value != "" {
		out.Stations[station.DatabaseJoinPathSelection] = value
	}
	if value := cfg.Get("repository_evidence_relevance_model"); value != "" {
		out.Stations[station.RepositoryEvidenceRelevance] = value
	}
	if value := cfg.Get("web_relevance_model"); value != "" {
		out.Stations[station.WebRelevance] = value
	}
	if value := cfg.Get("web_grounded_synthesis_model"); value != "" {
		out.Stations[station.WebGroundedSynthesis] = value
	}
	if value := cfg.Get("coding_surface_model"); value != "" {
		out.Stations[station.CodingSurface] = value
	}
	if value := cfg.Get("coding_requirements_model"); value != "" {
		out.Stations[station.CodingRequirements] = value
		out.Stations[station.CodingProjectStackConstraint] = value
	}
	if value := cfg.Get("coding_workload_model"); value != "" {
		out.Stations[station.CodingTargetTree] = value
	}
	if value := cfg.Get("coding_artifact_handling_model"); value != "" {
		out.Stations[station.CodingArtifactHandling] = value
	}
	if value := cfg.Get("coding_capability_relation_model"); value != "" {
		out.Stations[station.CodingCapabilityRelation] = value
		out.Stations[station.CodingRuntimeCapabilityNecessity] = value
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
	return out
}
