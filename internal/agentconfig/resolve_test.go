package agentconfig

import "testing"

func TestStackResolvePriority(t *testing.T) {
	stack := Stack{
		EnvFile:    Config{"agent_system": "omnidex"},
		ProcessEnv: Config{"agent_system": "codex"},
		Workspace:  Config{"agent_system": "cursor"},
		Project:    Config{"cursor_model": "project-model"},
		Card:       Config{"agent_system": "codex"},
		Instance:   Config{"agent_system": "cursor"},
	}
	resolved, source, err := stack.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if source != SourceInstance {
		t.Fatalf("source=%q want instance", source)
	}
	if resolved.System() != SystemCursor {
		t.Fatalf("system=%q want cursor", resolved.System())
	}
	if resolved.CursorModel() != "project-model" {
		t.Fatal("expected project model to remain in merged config")
	}
}

func TestStackResolveProjectOverGlobal(t *testing.T) {
	stack := Stack{
		EnvFile:   Config{"agent_system": "omnidex"},
		Workspace: Config{"agent_system": "cursor"},
		Project:   Config{"agent_system": "codex"},
	}
	resolved, source, err := stack.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if source != SourceProject {
		t.Fatalf("source=%q want project", source)
	}
	if resolved.System() != SystemCodex {
		t.Fatalf("system=%q want codex", resolved.System())
	}
}

func TestStackResolveRejectsInvalidLayer(t *testing.T) {
	stack := Stack{Project: Config{"agent_system": "legacy-local"}}
	if _, _, err := stack.Resolve(); err == nil {
		t.Fatal("expected invalid project agent configuration to fail")
	}
}
