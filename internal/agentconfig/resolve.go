package agentconfig

// Config source labels — highest wins, no blocking gates between layers.
const (
	SourceEnv       = "env"
	SourceWorkspace = "workspace"
	SourceProject   = "project"
	SourceCard      = "card"
	SourceInstance  = "instance"
)

// Stack holds agent-config layers from lowest to highest priority:
//
//	env (file + process) → workspace (global DB) → project → card → instance (single run)
//
// Each layer only overrides keys it sets; nothing at a lower layer can block a higher one.
type Stack struct {
	EnvFile    Config
	ProcessEnv Config
	Workspace  Config
	Project    Config
	Card       Config
	Instance   Config
}

func (s Stack) Resolve() (Config, string) {
	type layer struct {
		name string
		cfg  Config
	}
	layers := []layer{
		{SourceEnv, Merge(s.EnvFile, s.ProcessEnv)},
		{SourceWorkspace, s.Workspace},
		{SourceProject, s.Project},
		{SourceCard, s.Card},
		{SourceInstance, s.Instance},
	}
	out := Config{}
	source := SourceEnv
	for _, layer := range layers {
		if len(layer.cfg) == 0 {
			continue
		}
		out = Merge(out, layer.cfg)
		source = layer.name
	}
	if len(out) == 0 {
		return Config{"agent_system": SystemOmnidex}, SourceEnv
	}
	return out, source
}
