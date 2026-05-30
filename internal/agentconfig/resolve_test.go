package agentconfig

import "testing"

func TestStackResolvePriority(t *testing.T) {
	stack := Stack{
		EnvFile:    Config{"agent_system": "omnidex", "agent_strict": "false"},
		ProcessEnv: Config{"agent_system": "codex"},
		Workspace:  Config{"agent_system": "cursor"},
		Project:    Config{"agent_strict": "true"},
		Card:       Config{"agent_system": "codex"},
		Instance:   Config{"agent_system": "cursor"},
	}
	resolved, source := stack.Resolve()
	if source != SourceInstance {
		t.Fatalf("source=%q want instance", source)
	}
	if resolved.System() != SystemCursor {
		t.Fatalf("system=%q want cursor", resolved.System())
	}
	if !resolved.IsStrict() {
		t.Fatal("expected strict from project layer")
	}
}

func TestStackResolveProjectOverGlobal(t *testing.T) {
	stack := Stack{
		EnvFile:   Config{"agent_system": "omnidex"},
		Workspace: Config{"agent_system": "cursor"},
		Project:   Config{"agent_system": "codex"},
	}
	resolved, source := stack.Resolve()
	if source != SourceProject {
		t.Fatalf("source=%q want project", source)
	}
	if resolved.System() != SystemCodex {
		t.Fatalf("system=%q want codex", resolved.System())
	}
}
