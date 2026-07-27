package worker

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

type v3SubtaskAssignment struct {
	ID                   string
	Kind                 string
	RoleID               string
	ObjectiveID          string
	Objective            string
	Priority             int
	RequiredCapabilities []string
	Constraints          []string
	SuccessCriteria      []string
}

func parseV3SubtaskAssignment(contexts map[string]string) (v3SubtaskAssignment, error) {
	priority, err := strconv.Atoi(strings.TrimSpace(contexts["subtask_priority"]))
	if err != nil || priority < 1 || priority > 100 {
		return v3SubtaskAssignment{}, fmt.Errorf("subtask priority is invalid: %q", contexts["subtask_priority"])
	}
	assignment := v3SubtaskAssignment{
		ID:                   strings.TrimSpace(contexts["subtask_id"]),
		Kind:                 strings.TrimSpace(contexts["subtask_kind"]),
		RoleID:               strings.TrimSpace(contexts["subtask_role_id"]),
		ObjectiveID:          strings.TrimSpace(contexts["subtask_objective_id"]),
		Objective:            strings.TrimSpace(contexts["subtask_objective"]),
		Priority:             priority,
		RequiredCapabilities: splitDelimitedContext(contexts["subtask_capabilities"], ","),
		Constraints:          splitDelimitedContext(contexts["subtask_constraints"], "|"),
		SuccessCriteria:      splitDelimitedContext(contexts["subtask_success"], "|"),
	}
	missing := make([]string, 0, 5)
	if assignment.ID == "" {
		missing = append(missing, "id")
	}
	if assignment.RoleID == "" {
		missing = append(missing, "role_id")
	}
	if assignment.ObjectiveID == "" {
		missing = append(missing, "objective_id")
	}
	if assignment.Objective == "" {
		missing = append(missing, "objective")
	}
	if len(assignment.SuccessCriteria) == 0 {
		missing = append(missing, "success_criteria")
	}
	if len(missing) > 0 {
		return v3SubtaskAssignment{}, fmt.Errorf("subtask assignment missing %s", strings.Join(missing, ", "))
	}
	if !v3RoleOwnsSubtaskKind(assignment.RoleID, assignment.Kind) {
		return v3SubtaskAssignment{}, fmt.Errorf("subtask role %q cannot own kind %q", assignment.RoleID, assignment.Kind)
	}
	return assignment, nil
}

func splitDelimitedContext(value, delimiter string) []string {
	parts := strings.Split(value, delimiter)
	return cleanOrderedStrings(parts)
}

func validateV3SubtaskResult(result artifacts.SubtaskResultArtifact) error {
	violations := make([]string, 0, 8)
	if strings.TrimSpace(result.SubtaskID) == "" {
		violations = append(violations, "subtask_id is required")
	}
	if strings.TrimSpace(result.Kind) == "" {
		violations = append(violations, "kind is required")
	}
	if strings.TrimSpace(result.RoleID) == "" {
		violations = append(violations, "role_id is required")
	}
	if strings.TrimSpace(result.ObjectiveID) == "" {
		violations = append(violations, "objective_id is required")
	}
	if strings.TrimSpace(result.Objective) == "" {
		violations = append(violations, "objective is required")
	}
	if result.Priority < 1 || result.Priority > 100 {
		violations = append(violations, "priority must be between 1 and 100")
	}
	if strings.TrimSpace(result.Summary) == "" {
		violations = append(violations, "summary is required")
	}
	if !v3RoleOwnsSubtaskKind(result.RoleID, result.Kind) {
		violations = append(violations, fmt.Sprintf("role %q cannot own kind %q", result.RoleID, result.Kind))
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("v3 subtask result rejected: %s", strings.Join(violations, "; "))
	}
	return nil
}

func validateV3SubtaskAssignment(assignment v3SubtaskAssignment, intent artifacts.IntentArtifact, audit artifacts.CapabilityAuditArtifact) (artifacts.Objective, error) {
	var authoritative artifacts.Objective
	found := false
	for _, objective := range intent.Objectives {
		if strings.TrimSpace(objective.ID) == assignment.ObjectiveID {
			authoritative = objective
			found = true
			break
		}
	}
	if !found {
		return artifacts.Objective{}, fmt.Errorf("subtask %q references unknown objective %q", assignment.ID, assignment.ObjectiveID)
	}
	violations := make([]string, 0, 6)
	if assignment.Objective != strings.TrimSpace(authoritative.Description) {
		violations = append(violations, "objective differs from the authoritative objective description")
	}
	if assignment.Priority != authoritative.Priority {
		violations = append(violations, "priority differs from the authoritative objective")
	}
	if assignment.Kind == artifacts.SubtaskKindExecute && !authoritative.RequiresAction {
		violations = append(violations, "execution is forbidden for a non-action objective")
	}
	if authoritative.RequiresAction && assignment.Kind != artifacts.SubtaskKindExecute && containsExecutionCapability(assignment.RequiredCapabilities) {
		violations = append(violations, "execution capability is assigned to a non-execute subtask")
	}
	for _, capability := range uniqueStrings(assignment.RequiredCapabilities) {
		if !containsString(authoritative.RequiredCapabilities, capability) {
			violations = append(violations, fmt.Sprintf("capability %q is outside the authoritative objective", capability))
		}
		if !containsString(audit.AvailableCapabilities, capability) {
			violations = append(violations, fmt.Sprintf("capability %q is unavailable", capability))
		}
	}
	for _, constraint := range cleanOrderedStrings(assignment.Constraints) {
		if !containsStringExact(intent.Constraints, constraint) {
			violations = append(violations, fmt.Sprintf("constraint %q is outside the authoritative intent", constraint))
		}
	}
	if authoritative.RequiresAction && containsString(authoritative.RequiredCapabilities, capabilityWorkspaceWrite) {
		for _, constraint := range cleanOrderedStrings(intent.Constraints) {
			if !containsStringExact(assignment.Constraints, constraint) {
				violations = append(violations, fmt.Sprintf("authoritative constraint %q is not assigned", constraint))
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return artifacts.Objective{}, fmt.Errorf("subtask %q assignment rejected: %s", assignment.ID, strings.Join(violations, "; "))
	}
	return authoritative, nil
}

func containsStringExact(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
