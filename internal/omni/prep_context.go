package omni

import (
	"encoding/json"
	"fmt"
	"strings"
)

const defaultPrepContextBudgetLimit = 12000

type PrepContextBundle struct {
	TaskID               string              `json:"task_id,omitempty"`
	WorkspacePath        string              `json:"workspace_path"`
	WorksiteSurvey       WorksiteSurvey      `json:"worksite_survey"`
	CodebaseRoute        TaskRoute           `json:"codebase_route,omitempty"`
	MemoryBriefs         []PrepBrief         `json:"memory_briefs,omitempty"`
	DocumentationBriefs  []PrepBrief         `json:"documentation_briefs,omitempty"`
	WebResearchBriefs    []PrepBrief         `json:"web_research_briefs,omitempty"`
	WorkspaceBriefs      []PrepBrief         `json:"workspace_briefs,omitempty"`
	PlannerHints         []string            `json:"planner_hints,omitempty"`
	SpecialistHints      map[string][]string `json:"specialist_hints,omitempty"`
	Evidence             []PrepEvidence      `json:"evidence,omitempty"`
	OpenQuestions        []string            `json:"open_questions,omitempty"`
	Constraints          []string            `json:"constraints,omitempty"`
	Risks                []string            `json:"risks,omitempty"`
	ContextBudgetUsed    int                 `json:"context_budget_used"`
	ContextBudgetLimit   int                 `json:"context_budget_limit"`
	Compressed           bool                `json:"compressed,omitempty"`
	MemoryChecked        bool                `json:"memory_checked"`
	DocumentationChecked bool                `json:"documentation_checked,omitempty"`
	WebResearchChecked   bool                `json:"web_research_checked,omitempty"`
	CodebaseRouteChecked bool                `json:"codebase_route_checked,omitempty"`
}

type PrepBrief struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags,omitempty"`
	UsedBy      []string `json:"used_by,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type PrepEvidence struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Source    string   `json:"source"`
	Summary   string   `json:"summary"`
	Path      string   `json:"path,omitempty"`
	URL       string   `json:"url,omitempty"`
	MemoryID  string   `json:"memory_id,omitempty"`
	Hash      string   `json:"hash,omitempty"`
	Freshness string   `json:"freshness"`
	UsedBy    []string `json:"used_by,omitempty"`
}

type PrepValidation struct {
	Valid    bool     `json:"valid"`
	Failures []string `json:"failures,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func NewPrepContextBundle(taskID, workspace string, survey WorksiteSurvey, plan ContextToolPlan, route TaskRoute, memories []SessionMemory) PrepContextBundle {
	bundle := PrepContextBundle{
		TaskID:             strings.TrimSpace(taskID),
		WorkspacePath:      strings.TrimSpace(workspace),
		WorksiteSurvey:     survey,
		CodebaseRoute:      route,
		ContextBudgetLimit: defaultPrepContextBudgetLimit,
		MemoryChecked:      true,
		SpecialistHints: map[string][]string{
			"planner": {
				"Use prep briefs only as advisory routing context.",
				"Do not add objectives from memory or research unless workspace evidence proves a prerequisite.",
			},
			"shell_specialist": {
				"Prefer route likely files, verification commands, and bounded inspection commands.",
			},
			"evaluator": {
				"Check scope boundaries, provenance, and evidence requirements against prep context.",
			},
			"completion_checker": {
				"Completion still requires objective evidence, not prep claims.",
			},
		},
		Constraints: []string{
			"Prep context is advisory and budgeted.",
			"Memory is advisory and cannot create objectives.",
			"Web research is used only when required by the context plan.",
		},
	}
	if strings.TrimSpace(bundle.WorkspacePath) == "" {
		bundle.WorkspacePath = strings.TrimSpace(survey.WorkspacePath)
	}
	if strings.TrimSpace(bundle.WorkspacePath) == "" {
		bundle.WorkspacePath = workspacePathOrCurrentDir()
	}
	if strings.TrimSpace(survey.WorkspacePath) != "" {
		evidence := PrepEvidence{
			ID:        "prep-evidence-worksite-survey",
			Kind:      "worksite_survey",
			Source:    "worksite_survey",
			Summary:   strings.TrimSpace(strings.Join(survey.Evidence, "; ")),
			Path:      survey.WorkspacePath,
			Freshness: "current_turn",
			UsedBy:    []string{"planner", "evaluator", "completion_checker"},
		}
		if evidence.Summary == "" {
			evidence.Summary = fmt.Sprintf("project_state=%s package_manager=%s", survey.ProjectState, survey.PackageManager)
		}
		bundle.Evidence = append(bundle.Evidence, evidence)
		bundle.WorkspaceBriefs = append(bundle.WorkspaceBriefs, PrepBrief{
			ID:          "prep-brief-worksite-survey",
			Kind:        "worksite_survey",
			Content:     fmt.Sprintf("WORKSITE_SURVEY\nworkspace: %s\nproject_state: %s\npackage_manager: %s\nframeworks: %s", survey.WorkspacePath, survey.ProjectState, survey.PackageManager, strings.Join(survey.Frameworks, ", ")),
			UsedBy:      []string{"planner", "evaluator", "completion_checker"},
			EvidenceIDs: []string{evidence.ID},
		})
	}
	if len(route.LikelyFiles) > 0 || len(route.VerificationCommands) > 0 || route.Confidence > 0 {
		bundle.CodebaseRouteChecked = true
		evidenceID := "prep-evidence-codebase-route"
		bundle.Evidence = append(bundle.Evidence, PrepEvidence{
			ID:        evidenceID,
			Kind:      "codebase_route",
			Source:    "workspace_index",
			Summary:   strings.TrimSpace(strings.Join(route.Reasons, "; ")),
			Path:      DefaultCodebaseMapPath(bundle.WorkspacePath),
			Hash:      workspaceHash(bundle.WorkspacePath),
			Freshness: "current_workspace_hash",
			UsedBy:    []string{"planner", "shell_specialist", "evaluator"},
		})
		bundle.WorkspaceBriefs = append(bundle.WorkspaceBriefs, PrepBrief{
			ID:          "prep-brief-codebase-route",
			Kind:        "codebase_route_brief",
			Content:     formatCodebaseRouteBrief(route),
			UsedBy:      []string{"planner", "shell_specialist", "evaluator"},
			EvidenceIDs: []string{evidenceID},
		})
	}
	for index, memory := range memories {
		brief := prepBriefFromSessionMemory(index, memory)
		if brief.ID == "" {
			continue
		}
		bundle.Evidence = append(bundle.Evidence, PrepEvidence{
			ID:        brief.EvidenceIDs[0],
			Kind:      brief.Kind,
			Source:    prepEvidenceSourceForKind(brief.Kind),
			Summary:   truncateStructuredTimelineValue(memory.Content),
			MemoryID:  fmt.Sprintf("session:%s:%d", memory.Kind, index),
			Freshness: prepFreshness(memory.CreatedAt),
			UsedBy:    brief.UsedBy,
		})
		switch brief.Kind {
		case "documentation_brief":
			bundle.DocumentationChecked = true
			bundle.DocumentationBriefs = append(bundle.DocumentationBriefs, brief)
		case "web_research_brief":
			bundle.WebResearchChecked = true
			bundle.WebResearchBriefs = append(bundle.WebResearchBriefs, brief)
		case "codebase_route_brief", "worksite_survey":
			bundle.CodebaseRouteChecked = true
			if !prepBundleHasBrief(bundle.WorkspaceBriefs, brief.Kind) {
				bundle.WorkspaceBriefs = append(bundle.WorkspaceBriefs, brief)
			}
		default:
			bundle.MemoryBriefs = append(bundle.MemoryBriefs, brief)
		}
	}
	bundle.PlannerHints = prepPlannerHints(plan, bundle)
	bundle.Risks = prepRisks(plan, bundle)
	bundle.ContextBudgetUsed = prepContextBudget(bundle)
	return bundle
}

func ValidatePrepContextBundle(bundle PrepContextBundle, plan ContextToolPlan) PrepValidation {
	validation := PrepValidation{Valid: true}
	addFailure := func(code string) {
		validation.Valid = false
		validation.Failures = append(validation.Failures, code)
	}
	if strings.TrimSpace(bundle.WorkspacePath) == "" || strings.TrimSpace(bundle.WorksiteSurvey.WorkspacePath) == "" {
		addFailure("prep_missing_worksite_survey")
	}
	if !bundle.MemoryChecked {
		addFailure("prep_missing_memory_check")
	}
	if plan.NeedsDocuments && !bundle.DocumentationChecked {
		addFailure("prep_relevant_docs_not_checked")
	}
	if !plan.NeedsWebResearch && bundle.WebResearchChecked {
		addFailure("prep_web_research_unneeded")
	}
	if plan.NeedsShell && !bundle.CodebaseRouteChecked && strings.TrimSpace(bundle.WorksiteSurvey.ProjectState) != projectStateEmptyDirectory {
		validation.Warnings = append(validation.Warnings, "prep_route_missing_for_code_task")
	}
	if bundle.ContextBudgetLimit > 0 && bundle.ContextBudgetUsed > bundle.ContextBudgetLimit {
		addFailure("prep_context_too_large")
	}
	evidenceIDs := map[string]struct{}{}
	for _, evidence := range bundle.Evidence {
		if strings.TrimSpace(evidence.ID) != "" {
			evidenceIDs[evidence.ID] = struct{}{}
		}
		if strings.TrimSpace(evidence.Source) == "" || len(evidence.UsedBy) == 0 {
			addFailure("prep_brief_missing_provenance")
			break
		}
	}
	for _, brief := range allPrepBriefs(bundle) {
		if strings.TrimSpace(brief.Kind) == "" || strings.TrimSpace(brief.Content) == "" || len(brief.UsedBy) == 0 || len(brief.EvidenceIDs) == 0 {
			addFailure("prep_brief_missing_provenance")
			break
		}
		for _, evidenceID := range brief.EvidenceIDs {
			if _, ok := evidenceIDs[evidenceID]; !ok {
				addFailure("prep_brief_missing_provenance")
				break
			}
		}
	}
	validation.Failures = dedupeStrings(validation.Failures)
	validation.Warnings = dedupeStrings(validation.Warnings)
	return validation
}

func CompactPrepContextBundle(bundle PrepContextBundle, limit int) PrepContextBundle {
	if limit <= 0 {
		limit = defaultPrepContextBudgetLimit
	}
	bundle.ContextBudgetLimit = limit
	bundle.ContextBudgetUsed = prepContextBudget(bundle)
	if bundle.ContextBudgetUsed <= limit {
		return bundle
	}
	bundle.Compressed = true
	trimBriefs := func(briefs []PrepBrief) []PrepBrief {
		if len(briefs) > 4 {
			briefs = briefs[:4]
		}
		for i := range briefs {
			briefs[i].Content = truncatePrepText(briefs[i].Content, 700)
		}
		return briefs
	}
	bundle.MemoryBriefs = trimBriefs(bundle.MemoryBriefs)
	bundle.DocumentationBriefs = trimBriefs(bundle.DocumentationBriefs)
	bundle.WebResearchBriefs = trimBriefs(bundle.WebResearchBriefs)
	bundle.WorkspaceBriefs = trimBriefs(bundle.WorkspaceBriefs)
	for i := range bundle.Evidence {
		bundle.Evidence[i].Summary = truncatePrepText(bundle.Evidence[i].Summary, 240)
	}
	bundle.ContextBudgetUsed = prepContextBudget(bundle)
	return bundle
}

func prepBriefFromSessionMemory(index int, memory SessionMemory) PrepBrief {
	kind := strings.TrimSpace(memory.Kind)
	if kind == "" {
		return PrepBrief{}
	}
	usedBy := prepUsedByForKind(kind)
	evidenceID := fmt.Sprintf("prep-evidence-%s-%d", strings.ReplaceAll(kind, "_", "-"), index)
	return PrepBrief{
		ID:          fmt.Sprintf("prep-brief-%s-%d", strings.ReplaceAll(kind, "_", "-"), index),
		Kind:        kind,
		Content:     strings.TrimSpace(memory.Content),
		Tags:        cleanMemoryTags(memory.Tags),
		UsedBy:      usedBy,
		EvidenceIDs: []string{evidenceID},
	}
}

func prepUsedByForKind(kind string) []string {
	switch kind {
	case "codebase_route_brief":
		return []string{"planner", "shell_specialist", "evaluator"}
	case "documentation_brief":
		return []string{"planner", "documentation_specialist", "evaluator"}
	case "web_research_brief":
		return []string{"planner", "documentation_specialist", "evaluator"}
	case "capability":
		return []string{"planner", "evaluator"}
	default:
		return []string{"planner"}
	}
}

func prepEvidenceSourceForKind(kind string) string {
	switch kind {
	case "codebase_route_brief":
		return "workspace_index"
	case "documentation_brief":
		return "documentation_memory"
	case "web_research_brief":
		return "web_research"
	case "capability":
		return "session_memory"
	default:
		return "memory"
	}
}

func prepFreshness(createdAt string) string {
	if strings.TrimSpace(createdAt) == "" {
		return "current_turn"
	}
	return "recorded_at:" + strings.TrimSpace(createdAt)
}

func prepPlannerHints(plan ContextToolPlan, bundle PrepContextBundle) []string {
	hints := []string{"Use the prep bundle as compact, provenance-backed context."}
	if len(bundle.CodebaseRoute.LikelyFiles) > 0 {
		hints = append(hints, "Start with likely files from the codebase route before generic discovery.")
	}
	if plan.NeedsDocuments && len(bundle.DocumentationBriefs) > 0 {
		hints = append(hints, "Use documentation brief for conventions/API details.")
	}
	if plan.NeedsWebResearch && len(bundle.WebResearchBriefs) > 0 {
		hints = append(hints, "Use web research brief for current external facts only.")
	}
	return hints
}

func prepRisks(plan ContextToolPlan, bundle PrepContextBundle) []string {
	risks := []string{}
	if plan.NeedsDocuments && len(bundle.DocumentationBriefs) == 0 {
		risks = append(risks, "Documentation was requested by context plan but no reusable documentation brief was found.")
	}
	if plan.NeedsShell && len(bundle.CodebaseRoute.LikelyFiles) == 0 {
		risks = append(risks, "No likely files were found in the codebase route; use bounded discovery before patching.")
	}
	return risks
}

func prepBundleHasBrief(briefs []PrepBrief, kind string) bool {
	for _, brief := range briefs {
		if brief.Kind == kind {
			return true
		}
	}
	return false
}

func allPrepBriefs(bundle PrepContextBundle) []PrepBrief {
	total := []PrepBrief{}
	total = append(total, bundle.MemoryBriefs...)
	total = append(total, bundle.DocumentationBriefs...)
	total = append(total, bundle.WebResearchBriefs...)
	total = append(total, bundle.WorkspaceBriefs...)
	return total
}

func prepContextBudget(bundle PrepContextBundle) int {
	blob, err := json.Marshal(bundle)
	if err != nil {
		return 0
	}
	return len(blob)
}

func truncatePrepText(raw string, max int) string {
	trimmed := strings.TrimSpace(raw)
	if max <= 0 || len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "..."
}
