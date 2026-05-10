package odn

import "sort"

type RoleDefinition struct {
	ID             string
	Version        string
	Purpose        string
	OutputContract string
	AllowedTools   []string
	Implemented    bool
}

type ToolDefinition struct {
	ID          string
	Version     string
	RoleID      string
	Purpose     string
	RiskTier    int
	SideEffect  bool
	Repeatable  bool
	Implemented bool
}

type Registry struct {
	Roles           map[string]RoleDefinition
	Tools           map[string]ToolDefinition
	MaxToolsPerStep int
}

func DefaultRegistry() Registry {
	tools := map[string]ToolDefinition{
		"scaffold_go_html_project": {
			ID:          "scaffold_go_html_project",
			Version:     "1.0",
			RoleID:      "coding_specialist",
			Purpose:     "Create a deterministic Go + HTML starter project in workspace.",
			RiskTier:    1,
			SideEffect:  true,
			Repeatable:  false,
			Implemented: true,
		},
		"linux_command": {
			ID:          "linux_command",
			Version:     "1.0",
			RoleID:      "linux_expert",
			Purpose:     "Generate and execute tightly constrained Linux commands.",
			RiskTier:    2,
			SideEffect:  true,
			Repeatable:  true,
			Implemented: true,
		},
		"verification_gate": {
			ID:          "verification_gate",
			Version:     "1.0",
			RoleID:      "verification_specialist",
			Purpose:     "Verify preceding tool outputs against deterministic checks.",
			RiskTier:    0,
			SideEffect:  false,
			Repeatable:  true,
			Implemented: true,
		},
		"pgsql_query": {
			ID:          "pgsql_query",
			Version:     "0.1",
			RoleID:      "pgsql_expert",
			Purpose:     "Produce a constrained SQL query for context and memory lookups.",
			RiskTier:    1,
			SideEffect:  false,
			Repeatable:  true,
			Implemented: false,
		},
		"web_research": {
			ID:          "web_research",
			Version:     "0.1",
			RoleID:      "web_researcher",
			Purpose:     "Run web research through approved fetch/scrape adapters.",
			RiskTier:    1,
			SideEffect:  true,
			Repeatable:  true,
			Implemented: false,
		},
		"memory_lookup": {
			ID:          "memory_lookup",
			Version:     "0.1",
			RoleID:      "retrieval_librarian",
			Purpose:     "Retrieve candidate memory/context packs.",
			RiskTier:    0,
			SideEffect:  false,
			Repeatable:  true,
			Implemented: false,
		},
		"memory_write": {
			ID:          "memory_write",
			Version:     "0.1",
			RoleID:      "memory_curator",
			Purpose:     "Persist approved memory entries with provenance.",
			RiskTier:    2,
			SideEffect:  true,
			Repeatable:  true,
			Implemented: false,
		},
		"migration_generate": {
			ID:          "migration_generate",
			Version:     "0.1",
			RoleID:      "migration_specialist",
			Purpose:     "Generate migration bundles with up/down SQL.",
			RiskTier:    1,
			SideEffect:  false,
			Repeatable:  true,
			Implemented: false,
		},
		"schema_guard": {
			ID:          "schema_guard",
			Version:     "0.1",
			RoleID:      "schema_governor",
			Purpose:     "Validate migration ordering, safety, and drift policies.",
			RiskTier:    0,
			SideEffect:  false,
			Repeatable:  true,
			Implemented: false,
		},
	}

	roles := map[string]RoleDefinition{
		"router_llm":              role("router_llm", "CSV-only tool routing output.", []string{"scaffold_go_html_project", "linux_command", "verification_gate", "pgsql_query", "web_research", "memory_lookup", "memory_write", "migration_generate", "schema_guard"}, true),
		"coding_specialist":       role("coding_specialist", "Perform bounded code-generation tasks.", []string{"scaffold_go_html_project"}, true),
		"linux_expert":            role("linux_expert", "Generate tightly scoped shell commands.", []string{"linux_command"}, true),
		"verification_specialist": role("verification_specialist", "Run post-tool verification logic.", []string{"verification_gate"}, true),
		"pgsql_expert":            role("pgsql_expert", "Compose deterministic SQL for retrieval and diagnostics.", []string{"pgsql_query"}, false),
		"web_researcher":          role("web_researcher", "Gather external information via approved fetch tools.", []string{"web_research"}, false),
		"doc_manager":             role("doc_manager", "Split large documents into worker scopes.", nil, false),
		"doc_worker":              role("doc_worker", "Extract claims and relevant facts from assigned scope.", nil, false),
		"vlc_specialist":          role("vlc_specialist", "Handle media workflows via constrained controls.", nil, false),
		"software_architect":      role("software_architect", "Evaluate architecture choices and tradeoffs.", nil, false),
		"planning_specialist":     role("planning_specialist", "Convert objectives into milestones.", nil, false),
		"strategy_specialist":     role("strategy_specialist", "Optimize sequencing and posture.", nil, false),
		"migration_specialist":    role("migration_specialist", "Generate migration bundle candidates.", []string{"migration_generate"}, false),
		"schema_governor":         role("schema_governor", "Guard migration order and drift.", []string{"schema_guard"}, false),
		"memory_curator":          role("memory_curator", "Approve memory writes with provenance.", []string{"memory_write"}, false),
		"retrieval_librarian":     role("retrieval_librarian", "Optimize retrieval plans.", []string{"memory_lookup"}, false),
		"security_specialist":     role("security_specialist", "Detect and prevent abuse paths.", nil, false),
		"test_specialist":         role("test_specialist", "Generate deterministic test plans.", nil, false),
		"incident_analyst":        role("incident_analyst", "Perform failure forensics.", nil, false),
		"cost_latency_specialist": role("cost_latency_specialist", "Tune cost/latency tradeoffs.", nil, false),
		"context_compressor":      role("context_compressor", "Compress large context into targeted packs.", nil, false),
	}

	return Registry{
		Roles:           roles,
		Tools:           tools,
		MaxToolsPerStep: 8,
	}
}

func role(id, purpose string, allowed []string, implemented bool) RoleDefinition {
	copyAllowed := append([]string(nil), allowed...)
	return RoleDefinition{
		ID:             id,
		Version:        "1.0",
		Purpose:        purpose,
		OutputContract: "strict_contract",
		AllowedTools:   copyAllowed,
		Implemented:    implemented,
	}
}

func (r Registry) ToolIDs(onlyImplemented bool) []string {
	ids := make([]string, 0, len(r.Tools))
	for id, tool := range r.Tools {
		if onlyImplemented && !tool.Implemented {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r Registry) GetTool(id string) (ToolDefinition, bool) {
	tool, ok := r.Tools[id]
	return tool, ok
}

func (r Registry) IsRepeatable(id string) bool {
	tool, ok := r.Tools[id]
	if !ok {
		return false
	}
	return tool.Repeatable
}
