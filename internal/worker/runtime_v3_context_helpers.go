package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
)

func filterRuntimeAvailableV3Tools(service *Service, names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range uniqueStrings(names) {
		switch name {
		case "workspace.research", "workspace.write", "command.run":
			if service == nil || service.workspace == nil || !service.workspace.Enabled() {
				continue
			}
		case "web.search":
			if service == nil || service.webSearch == nil {
				continue
			}
		}
		out = append(out, name)
	}
	return out
}

func intentObjectiveSummary(intent artifacts.IntentArtifact) string {
	lines := []string{"Goal: " + strings.TrimSpace(intent.UserGoal)}
	for _, objective := range intent.Objectives {
		lines = append(lines, fmt.Sprintf("Objective %s (priority %d): %s", objective.ID, objective.Priority, objective.Description))
		for _, criterion := range objective.AcceptanceCriteria {
			lines = append(lines, "Acceptance: "+strings.TrimSpace(criterion))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func sessionTagForJob(job model.Job) string {
	return sessionTag(job)
}

func artifactRef(kind string, jobID int64) string {
	return fmt.Sprintf("%s:job-%d", strings.TrimSpace(kind), jobID)
}

func (r *nativeRuntimeV3) validatePlannedSpecialists(plan artifacts.PlanArtifact) error {
	violations := make([]string, 0, len(plan.Subtasks))
	availableTools := filterRuntimeAvailableV3Tools(r.svc, r.svc.v3Tools.Names())
	for _, subtask := range plan.Subtasks {
		spec, ok := r.svc.skillSpec(subtask.RoleID)
		if !ok {
			violations = append(violations, fmt.Sprintf("subtask %q references unregistered role %q", subtask.ID, subtask.RoleID))
			continue
		}
		roleTools := effectiveV3Tools(spec.AllowedTools, availableTools)
		for _, requiredTool := range toolsForV3Capabilities(subtask.RequiredCapabilities) {
			if !containsString(roleTools, requiredTool) {
				violations = append(violations, fmt.Sprintf("subtask %q role %q cannot use required tool %q", subtask.ID, subtask.RoleID, requiredTool))
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("v3 specialist routing rejected: %s", strings.Join(violations, "; "))
	}
	return nil
}
