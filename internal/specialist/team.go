package specialist

import (
	"fmt"
	"sort"
	"strings"
)

const (
	MemoryActionRead         = "read"
	MemoryActionCreate       = "create"
	MemoryActionUpdate       = "update"
	MemoryActionDeprioritize = "deprioritize"
)

type MemoryPolicy struct {
	CanRead                   bool     `json:"can_read"`
	CanCreate                 bool     `json:"can_create"`
	CanUpdate                 bool     `json:"can_update"`
	CanDeprioritize           bool     `json:"can_deprioritize"`
	WritesRequireEvidence     bool     `json:"writes_require_evidence"`
	AllowedKinds              []string `json:"allowed_kinds,omitempty"`
	StalenessResponsibilities []string `json:"staleness_responsibilities,omitempty"`
}

type ToolGrant struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose,omitempty"`
}

type TeamProfile struct {
	Role                Role         `json:"role"`
	Authority           string       `json:"authority"`
	AllowedTools        []ToolGrant  `json:"allowed_tools"`
	Memory              MemoryPolicy `json:"memory"`
	CanDelegateTo       []string     `json:"can_delegate_to,omitempty"`
	ContextContribution string       `json:"context_contribution"`
}

func DefaultTeam() []TeamProfile {
	return []TeamProfile{
		profile(RoleManagerSpecialist, "coordinate specialist work and enforce task ownership", allMemoryKinds(),
			tools("planner.delegate", "worker.dispatch", "summary.request", "memory.search", "memory.create"),
			[]string{RolePlannerSpecialist, RoleShellExecutionSpecialist, RoleWebResearchSpecialist, RoleDocumentationSpecialist, RoleMemorySpecialist, RoleResearchSpecialist, RoleCodeSpecialist, RoleWorkerSpecialist, RoleSummarySpecialist, RoleCorrectionSpecialist, RoleExpectationSpecialist},
			"task graph, ownership, blockers, and completion state"),
		profile(RolePlannerSpecialist, "translate user intent into delegated, verifiable steps", allMemoryKinds(),
			tools("planner.delegate", "memory.search", "memory.create"),
			[]string{RoleShellExecutionSpecialist, RoleWebResearchSpecialist, RoleDocumentationSpecialist, RoleMemorySpecialist, RoleResearchSpecialist, RoleCodeSpecialist, RoleWorkerSpecialist, RoleExpectationSpecialist},
			"plan, required evidence, specialist assignments, and done criteria"),
		profile(RoleShellExecutionSpecialist, "inspect and modify local systems through shell commands under policy", allMemoryKinds(),
			tools("bash", "cat", "sed", "grep", "rg", "awk", "find", "ls", "pwd", "jq", "curl", "git", "go", "npm", "python3", "memory.create", "research.enqueue"),
			[]string{RoleResearchSpecialist, RoleMemorySpecialist, RoleCorrectionSpecialist},
			"command output, filesystem evidence, changed paths, and shell-derived memories"),
		profile(RoleWebResearchSpecialist, "gather fresh external evidence from public sources", allMemoryKinds(),
			tools("web.search", "web.fetch", "curl", "memory.search", "memory.create"),
			[]string{RoleDocumentationSpecialist, RoleMemorySpecialist, RoleSummarySpecialist, RoleCorrectionSpecialist},
			"sourced web findings, citations, freshness notes, and research memories"),
		profile(RoleDocumentationSpecialist, "act as coding documentation authority for any language, SDK, API, framework, library, or toolchain", allMemoryKinds(),
			tools("web.search", "web.fetch", "curl", "memory.search", "memory.create", "pgsql.query"),
			[]string{RoleWebResearchSpecialist, RoleMemorySpecialist, RoleSummarySpecialist, RoleCorrectionSpecialist},
			"authoritative documentation briefs covering setup, conventions, locations, APIs, examples, risks, citations, and reusable doc memories"),
		profile(RoleMemorySpecialist, "retrieve, rank, create, update, and deprioritize memories", allMemoryKinds(),
			tools("memory.search", "memory.create", "memory.update", "memory.deprioritize", "pgsql.query"),
			[]string{RoleCorrectionSpecialist, RoleSummarySpecialist},
			"ranked memory context, provenance, staleness flags, and memory mutations"),
		profile(RoleCorrectionSpecialist, "audit outputs and memories for stale or false capability claims", allMemoryKinds(),
			tools("memory.search", "memory.create", "memory.update", "memory.deprioritize", "verification.run"),
			[]string{RoleMemorySpecialist, RoleShellExecutionSpecialist, RoleWebResearchSpecialist},
			"corrections, rejected claims, updated memory priority, and retry guidance"),
		profile(RoleExpectationSpecialist, "maintain user intent, acceptance criteria, and quality bar", allMemoryKinds(),
			tools("memory.search", "memory.create", "verification.run"),
			[]string{RolePlannerSpecialist, RoleSummarySpecialist, RoleCorrectionSpecialist},
			"acceptance criteria, user preferences, and expectation deltas"),
		profile(RoleResearchSpecialist, "study a codebase, directory, documentation set, or topic into durable memory", allMemoryKinds(),
			tools("rg", "cat", "sed", "find", "web.search", "web.fetch", "memory.search", "memory.create", "pgsql.query"),
			[]string{RoleShellExecutionSpecialist, RoleWebResearchSpecialist, RoleDocumentationSpecialist, RoleMemorySpecialist, RoleSummarySpecialist},
			"research notes, indexed facts, file references, source summaries, and memory chunks"),
		profile(RoleCodeSpecialist, "change code and validate behavior with focused tests", allMemoryKinds(),
			tools("bash", "cat", "sed", "grep", "rg", "git", "go", "npm", "python3", "memory.search", "memory.create", "verification.run"),
			[]string{RoleShellExecutionSpecialist, RoleResearchSpecialist, RoleCorrectionSpecialist, RoleSummarySpecialist},
			"patch plan, changed files, test results, and implementation memories"),
		profile(RoleWorkerSpecialist, "execute bounded subtasks assigned by manager or planner", allMemoryKinds(),
			tools("bash", "cat", "sed", "grep", "rg", "git", "memory.search", "memory.create", "verification.run"),
			[]string{RoleShellExecutionSpecialist, RoleMemorySpecialist, RoleCorrectionSpecialist},
			"subtask result, local evidence, changed paths, and worker notes"),
		profile(RoleSummarySpecialist, "compress multi-specialist evidence into concise context", allMemoryKinds(),
			tools("memory.search", "memory.create", "memory.update"),
			[]string{RoleMemorySpecialist, RoleCorrectionSpecialist},
			"high-signal summaries, handoff context, and final-response context"),
	}
}

func ProfileForRole(roleID string) (TeamProfile, bool) {
	roleID = normalize(roleID)
	for _, candidate := range DefaultTeam() {
		if normalize(candidate.Role.ID) == roleID {
			return candidate, true
		}
	}
	return TeamProfile{}, false
}

func ToolAllowed(roleID, toolName string) bool {
	profile, ok := ProfileForRole(roleID)
	if !ok {
		return false
	}
	toolName = normalize(toolName)
	for _, grant := range profile.AllowedTools {
		if normalize(grant.Name) == toolName {
			return true
		}
	}
	return false
}

func MemoryActionAllowed(roleID, action string) bool {
	profile, ok := ProfileForRole(roleID)
	if !ok {
		return false
	}
	switch normalize(action) {
	case MemoryActionRead:
		return profile.Memory.CanRead
	case MemoryActionCreate:
		return profile.Memory.CanCreate
	case MemoryActionUpdate:
		return profile.Memory.CanUpdate
	case MemoryActionDeprioritize:
		return profile.Memory.CanDeprioritize
	default:
		return false
	}
}

func ValidateTeam(profiles []TeamProfile) error {
	if len(profiles) == 0 {
		return fmt.Errorf("specialist team is empty")
	}
	seen := map[string]struct{}{}
	for _, profile := range profiles {
		roleID := normalize(profile.Role.ID)
		if roleID == "" {
			return fmt.Errorf("specialist profile missing role id")
		}
		if _, exists := seen[roleID]; exists {
			return fmt.Errorf("duplicate specialist role %q", profile.Role.ID)
		}
		seen[roleID] = struct{}{}
		if strings.TrimSpace(profile.Authority) == "" {
			return fmt.Errorf("specialist %s missing authority", roleID)
		}
		if strings.TrimSpace(profile.ContextContribution) == "" {
			return fmt.Errorf("specialist %s missing context contribution", roleID)
		}
		if !profile.Memory.CanCreate || !profile.Memory.WritesRequireEvidence {
			return fmt.Errorf("specialist %s must be able to create evidence-backed memory", roleID)
		}
	}
	return nil
}

func RoleIDs(profiles []TeamProfile) []string {
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		roleID := strings.TrimSpace(profile.Role.ID)
		if roleID != "" {
			out = append(out, roleID)
		}
	}
	sort.Strings(out)
	return out
}

func profile(roleID, authority string, memoryKinds []string, allowedTools []ToolGrant, delegates []string, contribution string) TeamProfile {
	return TeamProfile{
		Role:         roleByID(roleID),
		Authority:    strings.TrimSpace(authority),
		AllowedTools: allowedTools,
		Memory: MemoryPolicy{
			CanRead:               true,
			CanCreate:             true,
			CanUpdate:             roleID == RoleMemorySpecialist || roleID == RoleCorrectionSpecialist || roleID == RoleManagerSpecialist || roleID == RoleSummarySpecialist,
			CanDeprioritize:       roleID == RoleMemorySpecialist || roleID == RoleCorrectionSpecialist || roleID == RoleManagerSpecialist,
			WritesRequireEvidence: true,
			AllowedKinds:          memoryKinds,
			StalenessResponsibilities: []string{
				"mark memory as stale when newer command, web, file, or database evidence supersedes it",
				"prefer deprioritizing stale memory over deleting provenance",
			},
		},
		CanDelegateTo:       cleanRoleIDs(delegates),
		ContextContribution: strings.TrimSpace(contribution),
	}
}

func roleByID(roleID string) Role {
	switch normalize(roleID) {
	case RoleManagerSpecialist:
		return Role{ID: RoleManagerSpecialist, Name: "Manager Specialist", Scope: "coordinate specialists into one coherent execution system"}
	case RoleMemorySpecialist:
		return Role{ID: RoleMemorySpecialist, Name: "Memory Specialist", Scope: "manage durable and working context across specialists"}
	case RoleCorrectionSpecialist:
		return Role{ID: RoleCorrectionSpecialist, Name: "Correction Specialist", Scope: "correct stale context, false claims, and failed assumptions"}
	case RoleExpectationSpecialist:
		return Role{ID: RoleExpectationSpecialist, Name: "Expectation Specialist", Scope: "track user expectations, acceptance criteria, and quality bar"}
	case RoleResearchSpecialist:
		return Role{ID: RoleResearchSpecialist, Name: "Research Specialist", Scope: "study topics, directories, and documentation into reusable memory"}
	case RoleDocumentationSpecialist:
		return Role{ID: RoleDocumentationSpecialist, Name: "Documentation Specialist", Scope: "serve as coding documentation authority for any language, SDK, API, framework, library, or toolchain"}
	case RoleCodeSpecialist:
		return Role{ID: RoleCodeSpecialist, Name: "Code Specialist", Scope: "implement code changes and validate them"}
	case RoleWorkerSpecialist:
		return Role{ID: RoleWorkerSpecialist, Name: "Worker Specialist", Scope: "execute bounded delegated work packages"}
	case RoleSummarySpecialist:
		return Role{ID: RoleSummarySpecialist, Name: "Summary Specialist", Scope: "compress specialist outputs into high-signal context"}
	default:
		switch normalize(roleID) {
		case RolePlannerSpecialist:
			return ForPipelineAction("plan")
		case RoleShellExecutionSpecialist:
			return ForLocalCapability("local_shell")
		case RoleWebResearchSpecialist:
			return ForPipelineAction("web_search")
		case RoleDocumentationSpecialist:
			return ForPipelineAction("documentation")
		default:
			return Role{ID: normalize(roleID), Name: "Specialist", Scope: "specialized execution"}
		}
	}
}

func tools(names ...string) []ToolGrant {
	out := make([]ToolGrant, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, ToolGrant{Name: name})
	}
	return out
}

func allMemoryKinds() []string {
	return []string{"capability", "correction", "documentation_research", "episodic", "expectation", "project", "research", "summary", "tool_evidence"}
}

func cleanRoleIDs(roleIDs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		clean := normalize(roleID)
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}
