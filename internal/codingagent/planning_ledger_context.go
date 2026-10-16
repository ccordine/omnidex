package codingagent

import "github.com/gryph/omnidex/internal/model"

// PlanningLedgerContext is the current PostgreSQL projection supplied once to
// a planning call. It is never appended to the conversation transcript.
type PlanningLedgerContext struct {
	Revision int64                 `json:"revision"`
	Items    []PlanningContextItem `json:"items"`
}

type PlanningContextItem struct {
	ID                     model.LedgerItemID      `json:"id"`
	Kind                   string                  `json:"kind"`
	Text                   string                  `json:"text"`
	ReviewState            model.LedgerReviewState `json:"review_state,omitempty"`
	GroupID                model.LedgerItemID      `json:"group_id,omitempty"`
	RequirementIDs         []model.LedgerItemID    `json:"requirement_ids,omitempty"`
	ConstraintIDs          []model.LedgerItemID    `json:"constraint_ids,omitempty"`
	AcceptanceCriterionIDs []model.LedgerItemID    `json:"acceptance_criterion_ids,omitempty"`
	TaskIDs                []model.LedgerItemID    `json:"task_ids,omitempty"`
	DeliveryObligation     string                  `json:"delivery_obligation,omitempty"`
	KnownAffectedSystems   []string                `json:"known_affected_systems,omitempty"`
	ReadFiles              []string                `json:"read_files,omitempty"`
	WriteFiles             []string                `json:"write_files,omitempty"`
	DeleteFiles            []string                `json:"delete_files,omitempty"`
	Argv                   []string                `json:"argv,omitempty"`
	Required               bool                    `json:"required,omitempty"`
	TaskID                 model.LedgerItemID      `json:"task_id,omitempty"`
	PrerequisiteID         model.LedgerItemID      `json:"prerequisite_id,omitempty"`
}

func ActivePlanningLedgerContext(ledger *EngineeringLedger, completedTaskIDs ...string) *PlanningLedgerContext {
	if ledger == nil {
		return nil
	}
	context := &PlanningLedgerContext{Revision: ledger.Revision, Items: []PlanningContextItem{}}
	completed := make(map[model.LedgerItemID]bool, len(completedTaskIDs))
	for _, id := range completedTaskIDs {
		completed[model.LedgerItemID(id)] = true
	}
	appendItem := func(base model.LedgerItem, item PlanningContextItem) {
		if base.Disposition != model.LedgerDispositionActive {
			return
		}
		item.ID, item.ReviewState = base.ID, base.ReviewState
		context.Items = append(context.Items, item)
	}
	for _, item := range ledger.Objectives {
		appendItem(item.LedgerItem, PlanningContextItem{Kind: "objective", Text: item.Outcome})
	}
	for _, item := range ledger.Requirements {
		appendItem(item.LedgerItem, PlanningContextItem{Kind: "requirement", Text: item.Outcome})
	}
	for _, item := range ledger.Constraints {
		appendItem(item.LedgerItem, PlanningContextItem{Kind: "constraint", Text: item.Rule})
	}
	for _, item := range ledger.Groups {
		appendItem(item.LedgerItem, PlanningContextItem{Kind: "group", Text: item.Title, GroupID: item.ParentGroupID})
	}
	for _, item := range ledger.Tasks {
		if completed[item.ID] {
			continue
		}
		appendItem(item.LedgerItem, PlanningContextItem{
			Kind: "task", Text: item.Goal, GroupID: item.GroupID,
			RequirementIDs: item.RequirementIDs, ConstraintIDs: item.ConstraintIDs,
			AcceptanceCriterionIDs: item.AcceptanceCriterionIDs,
			DeliveryObligation:     item.DeliveryObligation, KnownAffectedSystems: item.KnownAffectedSystems,
			ReadFiles: item.ReadFiles, WriteFiles: item.WriteFiles, DeleteFiles: item.DeleteFiles,
		})
	}
	remainingCriteria := map[model.LedgerItemID]bool{}
	for _, item := range ledger.AcceptanceCriteria {
		taskIDs := []model.LedgerItemID{}
		for _, id := range item.TaskIDs {
			if !completed[id] {
				taskIDs = append(taskIDs, id)
			}
		}
		if len(item.TaskIDs) > 0 && len(taskIDs) == 0 {
			continue
		}
		remainingCriteria[item.ID] = true
		appendItem(item.LedgerItem, PlanningContextItem{Kind: "acceptance_criterion", Text: item.Outcome,
			RequirementIDs: item.RequirementIDs, TaskIDs: taskIDs, Required: item.Required})
	}
	for _, item := range ledger.Checks {
		criteria := []model.LedgerItemID{}
		for _, id := range item.AcceptanceCriterionIDs {
			if remainingCriteria[id] {
				criteria = append(criteria, id)
			}
		}
		if len(item.AcceptanceCriterionIDs) > 0 && len(criteria) == 0 {
			continue
		}
		appendItem(item.LedgerItem, PlanningContextItem{Kind: "check", Text: item.Observation,
			Argv: item.Argv, Required: item.Required, RequirementIDs: item.RequirementIDs,
			AcceptanceCriterionIDs: criteria})
	}
	for _, item := range ledger.Dependencies {
		if completed[item.TaskID] || completed[item.PrerequisiteID] {
			continue
		}
		appendItem(item.LedgerItem, PlanningContextItem{Kind: "dependency", Text: string(item.Kind),
			TaskID: item.TaskID, PrerequisiteID: item.PrerequisiteID})
	}
	return context
}
