package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	initialPlanningQuestion  = "initial-ledger-proposal"
	mutationPlanningQuestion = "owner-turn-ledger-mutation"
)

// PlanningTurn is the complete authority for one conversational edit. The
// observed revision comes from the persisted proposal, never model output or
// conversation memory.
type PlanningTurn struct {
	OperationID      string `json:"operation_id"`
	OwnerTurnID      string `json:"owner_turn_id"`
	Text             string `json:"text"`
	ObservedRevision int64  `json:"observed_revision"`
}

func (turn PlanningTurn) validate(initial bool) error {
	if strings.TrimSpace(turn.OperationID) == "" || strings.TrimSpace(turn.OwnerTurnID) == "" || strings.TrimSpace(turn.Text) == "" {
		return fmt.Errorf("planning turn requires operation, owner-turn, and exact text authority")
	}
	if initial {
		if turn.ObservedRevision != 0 {
			return fmt.Errorf("initial planning turn must observe revision zero")
		}
	} else if turn.ObservedRevision < 1 {
		return fmt.Errorf("planning mutation requires an observed proposal revision")
	}
	return nil
}

// InitialPlanningProposal contains the first parsed draft and code-bound turn
// provenance. Later parsing produces changes against the persisted draft.
type InitialPlanningProposal struct {
	Ledger               EngineeringLedger          `json:"ledger"`
	OwnerTurnDisposition model.OwnerTurnDisposition `json:"owner_turn_disposition"`
}

type DirectPlanningAction struct {
	Kind        model.LedgerMutationKind `json:"kind"`
	TargetID    model.LedgerItemID       `json:"target_id"`
	ReviewState model.LedgerReviewState  `json:"review_state,omitempty"`
	Rank        *int64                   `json:"rank,omitempty"`
	Rationale   string                   `json:"rationale"`
}

// DirectPlanningMutation converts an explicit UI action to a mutation without
// invoking a provider. It deliberately supports only actions whose meaning is
// completely expressed by the control itself.
func DirectPlanningMutation(turn PlanningTurn, action DirectPlanningAction) (LedgerMutation, error) {
	if err := turn.validate(false); err != nil {
		return LedgerMutation{}, err
	}
	switch action.Kind {
	case model.LedgerMutationRemove, model.LedgerMutationRestore,
		model.LedgerMutationReview, model.LedgerMutationReorder:
	default:
		return LedgerMutation{}, fmt.Errorf("direct planning action %q requires semantic interpretation", action.Kind)
	}
	operation := model.LedgerMutationOperation{
		Kind: action.Kind, TargetID: action.TargetID, ReviewState: action.ReviewState,
		Rank: action.Rank, Rationale: action.Rationale,
	}
	mutation := LedgerMutation{
		OperationID: turn.OperationID, BaseRevision: turn.ObservedRevision,
		OwnerTurnID: turn.OwnerTurnID,
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Statements: []model.OwnerStatementDisposition{{Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{action.TargetID}}},
		},
		Operations: []model.LedgerMutationOperation{operation},
	}
	if err := ValidateLedgerMutation(turn.ObservedRevision, mutation); err != nil {
		return LedgerMutation{}, err
	}
	return mutation, nil
}

// ProposePlanningLedger parses the ordinary planning reply once for the
// existing ledger repository to persist.
func (controller *Controller) ProposePlanningLedger(ctx context.Context, turn PlanningTurn) (InitialPlanningProposal, error) {
	if err := controller.validatePlanning(); err != nil {
		return InitialPlanningProposal{}, err
	}
	if err := turn.validate(true); err != nil {
		return InitialPlanningProposal{}, err
	}
	response, err := controller.planningResponse(ctx, turn, nil)
	if err != nil {
		return InitialPlanningProposal{}, err
	}
	call := controller.planningAssignment(PlanningInitial, initialPlanningQuestion,
		"Which planning ledger items are expressed in this conversation response?", turn, nil)
	call.Route = controller.Configuration.LedgerParser
	call.PlanningResponse = response
	call.ValidationNotes, err = controller.planningValidationNotes(ctx, turn.OperationID)
	if err != nil {
		return InitialPlanningProposal{}, err
	}
	result, err := controller.invoke(ctx, call, 1)
	if err != nil {
		return InitialPlanningProposal{}, err
	}
	var parsed struct {
		Ledger EngineeringLedger `json:"ledger"`
	}
	if err := decodeContract(result.Content, &parsed); err != nil {
		return InitialPlanningProposal{}, controller.rejectPlanningParse(ctx, turn, PlanningInitial, result.CallID, err)
	}
	ledger := parsed.Ledger
	ledger.ID, ledger.Revision = fmt.Sprintf("planning-%d", controller.JobID), 1
	requirePlanningVerification(&ledger)
	for _, item := range planningItemPointers(&ledger) {
		if item.Disposition == model.LedgerDispositionActive {
			item.ReviewState = model.LedgerReviewProposed
		}
	}
	if err := ledger.ValidateDraft(); err != nil {
		return InitialPlanningProposal{}, controller.rejectPlanningParse(ctx, turn, PlanningInitial, result.CallID, err)
	}
	return InitialPlanningProposal{Ledger: ledger, OwnerTurnDisposition: parsedPlanningDisposition(ledger, nil)}, nil
}

// PlanPlanningMutation continues the chat with the current ledger, then asks
// the separate parser only for ledger operations present in the reply.
func (controller *Controller) PlanPlanningMutation(ctx context.Context, ledger EngineeringLedger, turn PlanningTurn) (LedgerMutation, error) {
	if err := controller.validatePlanning(); err != nil {
		return LedgerMutation{}, err
	}
	if err := turn.validate(false); err != nil {
		return LedgerMutation{}, err
	}
	if ledger.Revision != turn.ObservedRevision {
		return LedgerMutation{}, fmt.Errorf("planning turn observed revision %d but database revision is %d", turn.ObservedRevision, ledger.Revision)
	}
	response, err := controller.planningResponse(ctx, turn, &ledger)
	if err != nil {
		return LedgerMutation{}, err
	}
	call := controller.planningAssignment(PlanningMutation, mutationPlanningQuestion,
		"Which planning ledger updates are expressed in this conversation response?", turn, &ledger)
	call.Route = controller.Configuration.LedgerParser
	call.PlanningResponse = response
	call.ValidationNotes, err = controller.planningValidationNotes(ctx, turn.OperationID)
	if err != nil {
		return LedgerMutation{}, err
	}
	result, err := controller.invoke(ctx, call, 1)
	if err != nil {
		return LedgerMutation{}, err
	}
	var parsed planningSieveMutationResult
	if err := decodeContract(result.Content, &parsed); err != nil {
		return LedgerMutation{}, controller.rejectPlanningParse(ctx, turn, PlanningMutation, result.CallID, err)
	}
	// The provider sieve already merged sparse patches with retained state.
	// Reinterpreting these full rows as model patches rejects code-owned fields.
	operations := parsed.Operations
	mutation := LedgerMutation{
		OperationID: turn.OperationID, OwnerTurnID: turn.OwnerTurnID,
		BaseRevision: turn.ObservedRevision, Operations: operations,
		OwnerTurnDisposition: parsedPlanningDisposition(ledger, operations),
	}
	if len(operations) > 0 {
		next, err := ApplyPlanningMutation(ledger, mutation)
		if err == nil {
			err = validateAddedConstraintConsumers(next, operations, controller.CompletedTaskIDs)
		}
		if err != nil {
			return LedgerMutation{}, controller.rejectPlanningParse(ctx, turn, PlanningMutation, result.CallID, err)
		}
	}
	return mutation, nil
}

func (controller *Controller) planningAssignment(role Role, name, question string, turn PlanningTurn, ledger *EngineeringLedger) Assignment {
	call := controller.assignment(role, nil, controller.Configuration.Planner)
	call.PlanningQuestionID = name
	call.PlanningBaseRevision = turn.ObservedRevision
	payload := struct {
		OwnerTurn *PlanningTurn      `json:"owner_turn"`
		Ledger    *EngineeringLedger `json:"ledger,omitempty"`
	}{OwnerTurn: &turn, Ledger: ledger}
	encoded, _ := json.Marshal(payload)
	call.ConstructionFacts = []ConstructionFact{
		{Kind: "engineering_question", Quote: name + ": " + question},
		{Kind: "bounded_planning_context", Quote: string(encoded)},
	}
	return call
}

// ApplyPlanningMutation previews the parsed operations before the repository
// transaction. The repository remains the durable transaction authority.
func ApplyPlanningMutation(current EngineeringLedger, mutation LedgerMutation) (EngineeringLedger, error) {
	if err := current.ValidateDraft(); err != nil {
		return EngineeringLedger{}, err
	}
	if err := ValidateLedgerMutation(current.Revision, mutation); err != nil {
		return EngineeringLedger{}, err
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return EngineeringLedger{}, err
	}
	var next EngineeringLedger
	if err := json.Unmarshal(raw, &next); err != nil {
		return EngineeringLedger{}, err
	}
	for _, operation := range mutation.Operations {
		if err := applyPlanningOperation(&next, operation); err != nil {
			return EngineeringLedger{}, err
		}
	}
	next.Revision = current.Revision + 1
	if err := next.ValidateDraft(); err != nil {
		return EngineeringLedger{}, fmt.Errorf("planning mutation result: %w", err)
	}
	return next, nil
}

func applyPlanningOperation(ledger *EngineeringLedger, operation model.LedgerMutationOperation) error {
	switch operation.Kind {
	case model.LedgerMutationAdd:
		appendPlanningItem(ledger, operation.Item)
		resetPlanningAgreements(ledger, planningMutationItemID(operation.Item))
		return nil
	case model.LedgerMutationAmend:
		if err := updatePlanningItem(ledger, operation.TargetID, func(item *model.LedgerItem) error {
			if item.Disposition != model.LedgerDispositionActive {
				return fmt.Errorf("planning mutation amend target %s is not active", operation.TargetID)
			}
			return nil
		}); err != nil {
			return err
		}
		resetPlanningAgreements(ledger, operation.TargetID)
		if !replacePlanningItem(ledger, operation.TargetID, operation.Item) {
			return fmt.Errorf("planning mutation target %s does not exist with the supplied type", operation.TargetID)
		}
		resetPlanningAgreements(ledger, operation.TargetID)
		return updatePlanningItem(ledger, operation.TargetID, func(item *model.LedgerItem) error {
			if item.Disposition == model.LedgerDispositionActive {
				item.ReviewState = model.LedgerReviewProposed
			}
			return nil
		})
	case model.LedgerMutationRemove:
		resetPlanningAgreements(ledger, operation.TargetID)
		return updatePlanningItem(ledger, operation.TargetID, func(item *model.LedgerItem) error {
			if item.Disposition != model.LedgerDispositionActive {
				return fmt.Errorf("planning item %s is not active", item.ID)
			}
			item.Disposition, item.ReviewState = model.LedgerDispositionTombstoned, ""
			return nil
		})
	case model.LedgerMutationRestore:
		if err := updatePlanningItem(ledger, operation.TargetID, func(item *model.LedgerItem) error {
			if item.Disposition != model.LedgerDispositionTombstoned {
				return fmt.Errorf("planning item %s is not removed", item.ID)
			}
			item.Disposition, item.ReviewState = model.LedgerDispositionActive, model.LedgerReviewProposed
			return nil
		}); err != nil {
			return err
		}
		resetPlanningAgreements(ledger, operation.TargetID)
		return nil
	case model.LedgerMutationReview:
		return updatePlanningItem(ledger, operation.TargetID, func(item *model.LedgerItem) error {
			if item.Disposition != model.LedgerDispositionActive {
				return fmt.Errorf("removed planning item %s cannot be reviewed", item.ID)
			}
			item.ReviewState = operation.ReviewState
			return nil
		})
	case model.LedgerMutationReorder:
		return updatePlanningItem(ledger, operation.TargetID, func(item *model.LedgerItem) error {
			item.Rank = *operation.Rank
			return nil
		})
	default:
		return fmt.Errorf("unsupported planning mutation %q", operation.Kind)
	}
}

func planningMutationItemID(item *model.LedgerMutationItem) model.LedgerItemID {
	switch {
	case item == nil:
		return ""
	case item.Objective != nil:
		return item.Objective.ID
	case item.Requirement != nil:
		return item.Requirement.ID
	case item.Constraint != nil:
		return item.Constraint.ID
	case item.Group != nil:
		return item.Group.ID
	case item.Task != nil:
		return item.Task.ID
	case item.AcceptanceCriterion != nil:
		return item.AcceptanceCriterion.ID
	case item.Check != nil:
		return item.Check.ID
	case item.Dependency != nil:
		return item.Dependency.ID
	default:
		return ""
	}
}

func resetPlanningAgreements(ledger *EngineeringLedger, changedID model.LedgerItemID) {
	_ = updatePlanningItem(ledger, changedID, func(item *model.LedgerItem) error {
		if item.Disposition == model.LedgerDispositionActive {
			item.ReviewState = model.LedgerReviewProposed
		}
		return nil
	})
	for _, check := range ledger.Checks {
		if check.ID != changedID || check.Disposition != model.LedgerDispositionActive || !check.Required {
			continue
		}
		criteria := map[model.LedgerItemID]bool{}
		for _, id := range check.AcceptanceCriterionIDs {
			criteria[id] = true
			_ = updatePlanningItem(ledger, id, func(item *model.LedgerItem) error {
				if item.Disposition == model.LedgerDispositionActive {
					item.ReviewState = model.LedgerReviewProposed
				}
				return nil
			})
		}
		requirements := map[model.LedgerItemID]bool{}
		for _, id := range check.RequirementIDs {
			requirements[id] = true
		}
		for index := range ledger.Tasks {
			task := &ledger.Tasks[index]
			if task.Disposition != model.LedgerDispositionActive {
				continue
			}
			if planningIDsIntersect(task.RequirementIDs, requirements) && planningIDsIntersect(task.AcceptanceCriterionIDs, criteria) {
				task.ReviewState = model.LedgerReviewProposed
			}
		}
	}
	pending, seen := []model.LedgerItemID{changedID}, map[model.LedgerItemID]bool{changedID: true}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		for index := range ledger.Dependencies {
			dependency := &ledger.Dependencies[index]
			if dependency.Disposition != model.LedgerDispositionActive && dependency.ID != changedID {
				continue
			}
			if dependency.ID != current && dependency.PrerequisiteID != current {
				continue
			}
			if dependency.Disposition == model.LedgerDispositionActive {
				dependency.ReviewState = model.LedgerReviewProposed
			}
			if !seen[dependency.TaskID] {
				seen[dependency.TaskID] = true
				pending = append(pending, dependency.TaskID)
				_ = updatePlanningItem(ledger, dependency.TaskID, func(item *model.LedgerItem) error {
					if item.Disposition == model.LedgerDispositionActive {
						item.ReviewState = model.LedgerReviewProposed
					}
					return nil
				})
			}
		}
	}
}

func planningIDsIntersect(ids []model.LedgerItemID, selected map[model.LedgerItemID]bool) bool {
	for _, id := range ids {
		if selected[id] {
			return true
		}
	}
	return false
}

func appendPlanningItem(ledger *EngineeringLedger, item *model.LedgerMutationItem) {
	switch {
	case item.Objective != nil:
		ledger.Objectives = append(ledger.Objectives, *item.Objective)
	case item.Requirement != nil:
		ledger.Requirements = append(ledger.Requirements, *item.Requirement)
	case item.Constraint != nil:
		ledger.Constraints = append(ledger.Constraints, *item.Constraint)
	case item.Group != nil:
		ledger.Groups = append(ledger.Groups, *item.Group)
	case item.Task != nil:
		ledger.Tasks = append(ledger.Tasks, *item.Task)
	case item.AcceptanceCriterion != nil:
		ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, *item.AcceptanceCriterion)
	case item.Check != nil:
		ledger.Checks = append(ledger.Checks, *item.Check)
	case item.Dependency != nil:
		ledger.Dependencies = append(ledger.Dependencies, *item.Dependency)
	}
}

func replacePlanningItem(ledger *EngineeringLedger, id model.LedgerItemID, item *model.LedgerMutationItem) bool {
	switch {
	case item.Objective != nil:
		for i := range ledger.Objectives {
			if ledger.Objectives[i].ID == id {
				ledger.Objectives[i] = *item.Objective
				return true
			}
		}
	case item.Requirement != nil:
		for i := range ledger.Requirements {
			if ledger.Requirements[i].ID == id {
				ledger.Requirements[i] = *item.Requirement
				return true
			}
		}
	case item.Constraint != nil:
		for i := range ledger.Constraints {
			if ledger.Constraints[i].ID == id {
				ledger.Constraints[i] = *item.Constraint
				return true
			}
		}
	case item.Group != nil:
		for i := range ledger.Groups {
			if ledger.Groups[i].ID == id {
				ledger.Groups[i] = *item.Group
				return true
			}
		}
	case item.Task != nil:
		for i := range ledger.Tasks {
			if ledger.Tasks[i].ID == id {
				ledger.Tasks[i] = *item.Task
				return true
			}
		}
	case item.AcceptanceCriterion != nil:
		for i := range ledger.AcceptanceCriteria {
			if ledger.AcceptanceCriteria[i].ID == id {
				ledger.AcceptanceCriteria[i] = *item.AcceptanceCriterion
				return true
			}
		}
	case item.Check != nil:
		for i := range ledger.Checks {
			if ledger.Checks[i].ID == id {
				ledger.Checks[i] = *item.Check
				return true
			}
		}
	case item.Dependency != nil:
		for i := range ledger.Dependencies {
			if ledger.Dependencies[i].ID == id {
				ledger.Dependencies[i] = *item.Dependency
				return true
			}
		}
	}
	return false
}

func updatePlanningItem(ledger *EngineeringLedger, id model.LedgerItemID, update func(*model.LedgerItem) error) error {
	for _, record := range planningItemPointers(ledger) {
		if record.ID == id {
			return update(record)
		}
	}
	return fmt.Errorf("planning mutation target %s does not exist", id)
}

func planningItemPointers(ledger *EngineeringLedger) []*model.LedgerItem {
	items := make([]*model.LedgerItem, 0)
	for i := range ledger.Objectives {
		items = append(items, &ledger.Objectives[i].LedgerItem)
	}
	for i := range ledger.Requirements {
		items = append(items, &ledger.Requirements[i].LedgerItem)
	}
	for i := range ledger.Constraints {
		items = append(items, &ledger.Constraints[i].LedgerItem)
	}
	for i := range ledger.Groups {
		items = append(items, &ledger.Groups[i].LedgerItem)
	}
	for i := range ledger.Tasks {
		items = append(items, &ledger.Tasks[i].LedgerItem)
	}
	for i := range ledger.AcceptanceCriteria {
		items = append(items, &ledger.AcceptanceCriteria[i].LedgerItem)
	}
	for i := range ledger.Checks {
		items = append(items, &ledger.Checks[i].LedgerItem)
	}
	for i := range ledger.Dependencies {
		items = append(items, &ledger.Dependencies[i].LedgerItem)
	}
	return items
}
