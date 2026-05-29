package modelconfig

import (
	"github.com/gryph/omnidex/internal/specialist"
)

type Routing struct {
	Default    string
	Fast       string
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
	if value := cfg.Get("planner_model"); value != "" {
		out.Plan = value
		out.Specialist[specialist.RolePlannerSpecialist] = value
		out.Specialist[specialist.RoleShellExecutionSpecialist] = value
	}
	if value := cfg.Get("thinking_model"); value != "" {
		out.Reasoning = value
	}
	if value := cfg.Get("evaluator_model"); value != "" {
		out.Analyze = value
		out.Specialist[specialist.RoleReviewVerificationSpecialist] = value
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
