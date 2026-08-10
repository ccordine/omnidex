package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/artifacts"
)

func buildV3CodingCoordinatorPlan(intent artifacts.IntentArtifact) (artifacts.PlanArtifact, bool) {
	if len(intent.Objectives) != 1 {
		return artifacts.PlanArtifact{}, false
	}
	objective := intent.Objectives[0]
	if !objective.RequiresAction ||
		!containsString(objective.RequiredCapabilities, capabilityWorkspaceWrite) ||
		!containsString(objective.RequiredCapabilities, capabilityCommandExecute) {
		return artifacts.PlanArtifact{}, false
	}
	criteria := cleanOrderedStrings(append(
		append([]string(nil), objective.AcceptanceCriteria...),
		intent.CompletionCriteria...,
	))
	return artifacts.PlanArtifact{
		Goal:        intent.UserGoal,
		Constraints: map[string]any{},
		Subtasks: []artifacts.Subtask{{
			ID:                   "coordinate_implementation",
			Kind:                 artifacts.SubtaskKindExecute,
			RoleID:               "subtask_executor",
			ObjectiveID:          objective.ID,
			Objective:            objective.Description,
			Priority:             objective.Priority,
			RequiredCapabilities: cleanOrderedStrings(objective.RequiredCapabilities),
			Constraints:          cleanOrderedStrings(intent.Constraints),
			SuccessCriteria:      criteria,
		}},
	}, true
}

func (r *nativeRuntimeV3) persistV3Plan(plan artifacts.PlanArtifact) error {
	if err := r.writeArtifact(artifacts.KindPlan, plan); err != nil {
		return err
	}
	count, err := r.svc.repo.CountStepsByAction(r.ctx, r.claim.Job.ID, "v3_subtask")
	if err != nil {
		return err
	}
	if count == 0 {
		filtered := filterDelegatedSubtasks(plan.Subtasks)
		if len(filtered) > 0 {
			if _, err := r.svc.repo.ExpandDelegatedSubtasks(r.ctx, r.claim.Authority, filtered); err != nil {
				return err
			}
		}
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("marshal v3 plan artifact: %w", err)
	}
	return r.complete("plan", string(planJSON), string(planJSON))
}
