package worker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

func parseStrictV3Plan(raw string) (artifacts.PlanArtifact, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var plan artifacts.PlanArtifact
	if err := decoder.Decode(&plan); err != nil {
		return artifacts.PlanArtifact{}, fmt.Errorf("decode v3 plan: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return artifacts.PlanArtifact{}, fmt.Errorf("decode v3 plan: %w", err)
	}
	if err := validateV3PlanShape(plan); err != nil {
		return artifacts.PlanArtifact{}, err
	}
	return plan, nil
}

func validateV3Plan(plan artifacts.PlanArtifact, intent artifacts.IntentArtifact, audit artifacts.CapabilityAuditArtifact) error {
	violations := validateV3PlanShapeViolations(plan)
	if strings.TrimSpace(plan.Goal) != strings.TrimSpace(intent.UserGoal) {
		violations = append(violations, "plan goal must equal the authoritative intent goal")
	}
	objectiveByID := map[string]artifacts.Objective{}
	for _, objective := range intent.Objectives {
		objectiveByID[strings.TrimSpace(objective.ID)] = objective
	}
	covered := map[string]struct{}{}
	coveredCapabilities := map[string]map[string]struct{}{}
	coveredCriteria := map[string]map[string]struct{}{}
	coveredConstraints := map[string]struct{}{}
	executionDelegated := map[string]bool{}
	available := stringSet(audit.AvailableCapabilities)
	if required, _ := plan.Constraints["needs_external_info"].(bool); required && !containsString(intent.RequiredCapabilities, capabilityWebSearch) {
		violations = append(violations, "plan requires external research that is absent from the authoritative intent")
	}
	for index, subtask := range plan.Subtasks {
		prefix := fmt.Sprintf("subtasks[%d]", index)
		objective, ok := objectiveByID[strings.TrimSpace(subtask.ObjectiveID)]
		if !ok {
			violations = append(violations, prefix+" references an unknown objective_id")
		} else {
			if strings.TrimSpace(subtask.Objective) != strings.TrimSpace(objective.Description) {
				violations = append(violations, prefix+" objective must equal its authoritative objective description")
			}
			covered[objective.ID] = struct{}{}
			if coveredCapabilities[objective.ID] == nil {
				coveredCapabilities[objective.ID] = map[string]struct{}{}
			}
			if coveredCriteria[objective.ID] == nil {
				coveredCriteria[objective.ID] = map[string]struct{}{}
			}
			for _, criterion := range cleanOrderedStrings(subtask.SuccessCriteria) {
				coveredCriteria[objective.ID][criterion] = struct{}{}
			}
			if subtask.Priority != objective.Priority {
				violations = append(violations, prefix+" priority differs from its authoritative objective")
			}
			if subtask.Kind == artifacts.SubtaskKindExecute {
				executionDelegated[objective.ID] = true
				if !objective.RequiresAction {
					violations = append(violations, prefix+" delegates execution for a non-action objective")
				}
			}
		}
		if !v3RoleOwnsSubtaskKind(subtask.RoleID, subtask.Kind) {
			violations = append(violations, fmt.Sprintf("%s role %q cannot own kind %q", prefix, subtask.RoleID, subtask.Kind))
		}
		for _, capability := range uniqueStrings(subtask.RequiredCapabilities) {
			if _, ok := available[capability]; !ok {
				violations = append(violations, fmt.Sprintf("%s requires unavailable capability %q", prefix, capability))
			}
			if ok {
				if !containsString(objective.RequiredCapabilities, capability) {
					violations = append(violations, fmt.Sprintf("%s adds capability %q outside objective %q", prefix, capability, objective.ID))
				} else {
					coveredCapabilities[objective.ID][capability] = struct{}{}
				}
			}
		}
		for _, constraint := range cleanOrderedStrings(subtask.Constraints) {
			if !containsStringExact(intent.Constraints, constraint) {
				violations = append(violations, fmt.Sprintf("%s adds constraint %q outside the authoritative intent", prefix, constraint))
			} else {
				coveredConstraints[constraint] = struct{}{}
			}
		}
		switch subtask.RoleID {
		case "workspace_researcher":
			if !containsString(subtask.RequiredCapabilities, capabilityWorkspaceRead) {
				violations = append(violations, prefix+" assigns workspace_researcher without workspace.read")
			}
		case "web_researcher":
			if !containsString(subtask.RequiredCapabilities, capabilityWebSearch) {
				violations = append(violations, prefix+" assigns web_researcher without web.search")
			}
		}
	}
	for _, constraint := range cleanOrderedStrings(intent.Constraints) {
		if _, assigned := coveredConstraints[constraint]; !assigned {
			violations = append(violations, fmt.Sprintf("authoritative constraint %q is not assigned", constraint))
		}
	}
	for _, objective := range intent.Objectives {
		if _, ok := covered[objective.ID]; !ok {
			violations = append(violations, fmt.Sprintf("objective %q has no delegated subtask", objective.ID))
		}
		for _, capability := range uniqueStrings(objective.RequiredCapabilities) {
			if _, ok := coveredCapabilities[objective.ID][capability]; !ok {
				violations = append(violations, fmt.Sprintf("objective %q capability %q is not assigned", objective.ID, capability))
			}
		}
		for _, criterion := range cleanOrderedStrings(objective.AcceptanceCriteria) {
			if _, ok := coveredCriteria[objective.ID][criterion]; !ok {
				violations = append(violations, fmt.Sprintf("objective %q acceptance criterion %q is not assigned", objective.ID, criterion))
			}
		}
		if objective.RequiresAction && !executionDelegated[objective.ID] {
			violations = append(violations, fmt.Sprintf("action objective %q has no execute subtask", objective.ID))
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("v3 plan rejected: %s", strings.Join(violations, "; "))
	}
	return nil
}

func validateV3PlanShape(plan artifacts.PlanArtifact) error {
	violations := validateV3PlanShapeViolations(plan)
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("v3 plan rejected: %s", strings.Join(violations, "; "))
}

func validateV3PlanShapeViolations(plan artifacts.PlanArtifact) []string {
	violations := make([]string, 0, 8)
	if strings.TrimSpace(plan.Goal) == "" {
		violations = append(violations, "goal is required")
	}
	if len(plan.Subtasks) == 0 {
		violations = append(violations, "at least one subtask is required")
	}
	seen := map[string]struct{}{}
	for index, subtask := range plan.Subtasks {
		prefix := fmt.Sprintf("subtasks[%d]", index)
		id := strings.TrimSpace(subtask.ID)
		if id == "" {
			violations = append(violations, prefix+".id is required")
		} else if _, exists := seen[id]; exists {
			violations = append(violations, fmt.Sprintf("subtask id %q is duplicated", id))
		} else {
			seen[id] = struct{}{}
		}
		if strings.TrimSpace(subtask.Objective) == "" {
			violations = append(violations, prefix+".objective is required")
		}
		if strings.TrimSpace(subtask.RoleID) == "" {
			violations = append(violations, prefix+".role_id is required")
		}
		if subtask.Priority < 1 || subtask.Priority > 100 {
			violations = append(violations, prefix+".priority must be between 1 and 100")
		}
		if len(cleanOrderedStrings(subtask.SuccessCriteria)) == 0 {
			violations = append(violations, prefix+".success_criteria is required")
		}
		switch strings.TrimSpace(subtask.Kind) {
		case artifacts.SubtaskKindResearch, artifacts.SubtaskKindAnalyze, artifacts.SubtaskKindExecute:
		default:
			violations = append(violations, fmt.Sprintf("%s.kind %q is invalid", prefix, subtask.Kind))
		}
	}
	return violations
}

func v3RoleOwnsSubtaskKind(roleID, kind string) bool {
	roleID = strings.TrimSpace(roleID)
	switch strings.TrimSpace(kind) {
	case artifacts.SubtaskKindResearch:
		return roleID == "workspace_researcher" || roleID == "web_researcher" || roleID == "subtask_executor"
	case artifacts.SubtaskKindAnalyze:
		return roleID == "subtask_executor"
	case artifacts.SubtaskKindExecute:
		return roleID == "subtask_executor"
	default:
		return false
	}
}
