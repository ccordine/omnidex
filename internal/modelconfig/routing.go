package modelconfig

import (
	"github.com/gryph/omnidex/internal/specialist"
)

type Routing struct {
	Default    string
	Fast       string
	Glue       string
	Reasoning  string
	Tagging    string
	Plan       string
	Analyze    string
	Response   string
	Search     string
	Memory     string
	Specialist map[string]string
}

func Apply(base Routing, cfg Config) Routing {
	out := base
	if out.Specialist == nil {
		out.Specialist = map[string]string{}
	} else {
		clone := map[string]string{}
		for key, value := range out.Specialist {
			clone[key] = value
		}
		out.Specialist = clone
	}
	if value := cfg.Get("default_model"); value != "" {
		out.Default = value
		if out.Response == base.Response || out.Response == "" {
			out.Response = value
		}
		if out.Fast == base.Fast || out.Fast == "" {
			out.Fast = value
		}
	}
	if value := cfg.Get("fast_model"); value != "" {
		out.Fast = value
	}
	if value := cfg.Get("glue_model"); value != "" {
		out.Glue = value
	}
	if value := cfg.Get("reasoning_model"); value != "" {
		out.Reasoning = value
	}
	if value := cfg.Get("planner_model"); value != "" {
		out.Plan = value
	}
	if value := cfg.Get("analyzer_model"); value != "" {
		out.Analyze = value
	}
	if value := cfg.Get("responder_model"); value != "" {
		out.Response = value
	}
	if value := cfg.Get("tagger_model"); value != "" {
		out.Tagging = value
	}
	if value := cfg.Get("search_model"); value != "" {
		out.Search = value
	}
	if value := cfg.Get("memory_model"); value != "" {
		out.Memory = value
	}
	if value := cfg.Get("executor_model"); value != "" {
		out.Specialist[specialist.RoleSubtaskExecutorSpecialist] = value
	}
	if value := cfg.Get("coding_surface_model"); value != "" {
		out.Specialist[specialist.RoleCodingSurfaceStation] = value
	}
	if value := cfg.Get("coding_product_identity_model"); value != "" {
		out.Specialist[specialist.RoleCodingProductIdentityStation] = value
	}
	if value := cfg.Get("coding_requirement_partition_model"); value != "" {
		out.Specialist[specialist.RoleCodingRequirementPartitionStation] = value
	}
	if value := cfg.Get("coding_requirement_adviser_model"); value != "" {
		out.Specialist[specialist.RoleCodingRequirementAdviserStation] = value
	}
	if value := cfg.Get("coding_requirement_split_model"); value != "" {
		out.Specialist[specialist.RoleCodingRequirementSplitStation] = value
	}
	if value := cfg.Get("coding_artifact_handling_model"); value != "" {
		out.Specialist[specialist.RoleCodingArtifactHandlingStation] = value
	}
	if value := cfg.Get("coding_capability_relation_model"); value != "" {
		out.Specialist[specialist.RoleCodingCapabilityRelationStation] = value
	}
	if value := cfg.Get("coding_skill_selection_model"); value != "" {
		out.Specialist[specialist.RoleCodingSkillSelectionStation] = value
	}
	if value := cfg.Get("coding_skill_procedure_model"); value != "" {
		out.Specialist[specialist.RoleCodingSkillProcedureStation] = value
	}
	if value := cfg.Get("coding_fragment_model"); value != "" {
		out.Specialist[specialist.RoleCodingFragmentStation] = value
	}
	if value := cfg.Get("coding_fragment_correction_model"); value != "" {
		out.Specialist[specialist.RoleCodingFragmentCorrectionStation] = value
	}
	if value := cfg.Get("shell_specialist_model"); value != "" {
		out.Specialist[specialist.RoleShellExecutionSpecialist] = value
	}
	return out
}

func Resolve(base Routing, env Config, project Config, card Config) Config {
	return Merge(env, project, card)
}

func ResolveRouting(base Routing, env Config, project Config, card Config) Routing {
	return Apply(base, Resolve(base, env, project, card))
}
