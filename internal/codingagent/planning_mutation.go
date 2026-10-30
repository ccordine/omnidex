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
	ItemID              model.LedgerItemID `json:"item_id,omitempty"`
	OperationID         string             `json:"operation_id"`
	OwnerTurnID         string             `json:"owner_turn_id"`
	Text                string             `json:"text"`
	ObservedRevision    int64              `json:"observed_revision"`
	NewerOwnerDirection *PlanningDirection `json:"newer_owner_direction,omitempty"`
}

// PlanningDirection retains later explicit authority while an older queued
// message is reconciled. The queued message itself remains unchanged.
type PlanningDirection struct {
	OperationID string `json:"operation_id"`
	Text        string `json:"text"`
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
	Move        int                      `json:"move,omitempty"`
	Item        *model.LedgerTask        `json:"item,omitempty"`
	Kind        model.LedgerMutationKind `json:"kind"`
	TargetID    model.LedgerItemID       `json:"target_id"`
	ReviewState model.LedgerReviewState  `json:"review_state,omitempty"`
	Rank        *int64                   `json:"rank,omitempty"`
	Rationale   string                   `json:"rationale"`
}

// Manual task contents are already explicit owner input. Code supplies identity
// and lifecycle fields without asking a model to reinterpret the edit.
func DirectPlanningItemMutation(ledger EngineeringLedger, turn PlanningTurn, action DirectPlanningAction) (LedgerMutation, error) {
	if action.Kind == model.LedgerMutationReorder && action.Move != 0 {
		return moveLedgerItem(ledger, turn, action)
	}
	if action.Kind != model.LedgerMutationAdd && action.Kind != model.LedgerMutationAmend {
		return DirectPlanningMutation(turn, action)
	}
	if action.Item == nil {
		return LedgerMutation{}, fmt.Errorf("task contents are required")
	}
	task := *action.Item
	var original *model.LedgerTask
	if action.Kind == model.LedgerMutationAdd {
		var rank int64
		for _, item := range ledger.Tasks {
			rank = max(rank, item.Rank+1)
		}
		task.LedgerItem = LedgerItem{ID: model.LedgerItemID("manual-" + turn.OperationID), Rank: rank,
			Provenance:  model.LedgerProvenance{Kind: "owner_turn", Reference: turn.OwnerTurnID, Revision: ledger.Revision + 1},
			Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}
	} else {
		found := false
		for index := range ledger.Tasks {
			item := &ledger.Tasks[index]
			if item.ID == action.TargetID {
				task.LedgerItem, original, found = item.LedgerItem, item, true
				break
			}
		}
		if !found {
			return LedgerMutation{}, fmt.Errorf("selected task does not exist")
		}
	}
	bindPlanningDetails(&task, original, model.LedgerProvenance{Kind: "owner_turn", Reference: turn.OwnerTurnID, Revision: ledger.Revision + 1})
	op := model.LedgerMutationOperation{Kind: action.Kind, Item: &model.LedgerMutationItem{Task: &task}, Rationale: action.Rationale}
	if action.Kind == model.LedgerMutationAmend {
		op.TargetID = task.ID
	}
	mutation := LedgerMutation{OperationID: turn.OperationID, BaseRevision: turn.ObservedRevision, OwnerTurnID: turn.OwnerTurnID,
		OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{task.ID}), Operations: []model.LedgerMutationOperation{op}}
	return mutation, mutation.Validate()
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
	for _, item := range planningItemPointers(&ledger) {
		if item.Disposition == model.LedgerDispositionActive {
			item.ReviewState = model.LedgerReviewProposed
		}
	}
	if err := ledger.ValidateDraft(); err != nil {
		return InitialPlanningProposal{}, controller.rejectPlanningParse(ctx, turn, PlanningInitial, result.CallID, err)
	}
	if err := controller.orderPlanningTasks(ctx, turn, ledger.Tasks); err != nil {
		return InitialPlanningProposal{}, err
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
	input := ledger
	if turn.ItemID != "" {
		input.Tasks = nil
		for _, task := range ledger.Tasks {
			if task.ID == turn.ItemID {
				input.Tasks = append(input.Tasks, task)
			}
		}
		if len(input.Tasks) == 0 {
			return LedgerMutation{}, fmt.Errorf("selected ledger item is unavailable")
		}
	}
	preceding := precedingIncompletePlanningTasks(ledger, turn.ItemID, controller.CompletedTaskIDs)
	response, err := controller.planningResponse(ctx, turn, &input, preceding...)
	if err != nil {
		return LedgerMutation{}, err
	}
	call := controller.planningAssignment(PlanningMutation, mutationPlanningQuestion,
		"Which planning ledger updates are expressed in this conversation response?", turn, &input)
	call.Route = controller.Configuration.LedgerParser
	call.PrecedingIncompleteTasks = preceding
	if turn.ItemID != "" {
		call.ConstructionFacts = append(call.ConstructionFacts, ConstructionFact{Kind: "selected_ledger_item", Quote: selectedItemPlanningScope})
	}
	call.PlanningResponse = response
	call.ValidationNotes, err = controller.planningValidationNotes(ctx, turn.OperationID)
	if err != nil {
		return LedgerMutation{}, err
	}
	result, err := controller.invoke(ctx, call, 1)
	if err != nil {
		return LedgerMutation{}, err
	}
	var parsed struct {
		Operations []model.LedgerMutationOperation `json:"operations"`
	}
	if err := decodeContract(result.Content, &parsed); err != nil {
		return LedgerMutation{}, controller.rejectPlanningParse(ctx, turn, PlanningMutation, result.CallID, err)
	}
	operations := parsed.Operations
	if turn.ItemID != "" {
		operations = nil
		for _, op := range parsed.Operations {
			if op.Item == nil || op.Item.Task == nil {
				continue
			}
			task := *op.Item.Task
			task.LedgerItem = input.Tasks[0].LedgerItem
			operations = []model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: turn.ItemID, Item: &model.LedgerMutationItem{Task: &task}, Rationale: op.Rationale}}
			break
		}
	}
	mutation := LedgerMutation{
		OperationID: turn.OperationID, OwnerTurnID: turn.OwnerTurnID,
		BaseRevision: turn.ObservedRevision, Operations: operations,
		OwnerTurnDisposition: parsedPlanningDisposition(ledger, operations),
	}
	if len(operations) > 0 {
		_, err := ApplyPlanningMutation(ledger, mutation)
		if err != nil {
			return LedgerMutation{}, controller.rejectPlanningParse(ctx, turn, PlanningMutation, result.CallID, err)
		}
	}
	if err := controller.orderPlanningAdditions(ctx, ledger, turn, &mutation); err != nil {
		return LedgerMutation{}, err
	}
	return mutation, nil
}

func (controller *Controller) planningAssignment(role Role, name, question string, turn PlanningTurn, ledger *EngineeringLedger) Assignment {
	call := controller.assignment(role, nil, controller.Configuration.Planner)
	call.PlanningQuestionID = name
	call.PlanningBaseRevision = turn.ObservedRevision
	call.PlanningLedger = ledger
	call.CompletedTaskIDs = append([]string{}, controller.CompletedTaskIDs...)
	if turn.ItemID != "" {
		call.CompletedTaskIDs = nil
	}
	payload := struct {
		OwnerTurn *PlanningTurn          `json:"owner_turn"`
		Ledger    *PlanningLedgerContext `json:"ledger,omitempty"`
	}{OwnerTurn: &turn, Ledger: ActivePlanningLedgerContext(ledger, call.CompletedTaskIDs...)}
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
		wasRemoved := false
		if err := updatePlanningItem(ledger, operation.TargetID, func(item *model.LedgerItem) error {
			if item.Disposition == model.LedgerDispositionActive {
				// Only the repository can retrieve the exact accepted contract.
				return nil
			}
			if item.Disposition != model.LedgerDispositionTombstoned {
				return fmt.Errorf("planning item %s is not removed", item.ID)
			}
			wasRemoved = true
			item.Disposition, item.ReviewState = model.LedgerDispositionActive, model.LedgerReviewProposed
			return nil
		}); err != nil {
			return err
		}
		if wasRemoved {
			resetPlanningAgreements(ledger, operation.TargetID)
		}
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
	if item != nil && item.Task != nil {
		return item.Task.ID
	}
	return ""
}
func resetPlanningAgreements(ledger *EngineeringLedger, id model.LedgerItemID) {
	_ = updatePlanningItem(ledger, id, func(item *model.LedgerItem) error {
		if item.Disposition == model.LedgerDispositionActive {
			item.ReviewState = model.LedgerReviewProposed
		}
		return nil
	})
}
func planningItemPointers(ledger *EngineeringLedger) []*model.LedgerItem {
	out := make([]*model.LedgerItem, 0, len(ledger.Tasks))
	for i := range ledger.Tasks {
		out = append(out, &ledger.Tasks[i].LedgerItem)
	}
	return out
}
func updatePlanningItem(ledger *EngineeringLedger, id model.LedgerItemID, update func(*model.LedgerItem) error) error {
	for _, item := range planningItemPointers(ledger) {
		if item.ID == id {
			return update(item)
		}
	}
	return fmt.Errorf("planning task %s does not exist", id)
}
func appendPlanningItem(ledger *EngineeringLedger, item *model.LedgerMutationItem) {
	if item != nil && item.Task != nil {
		ledger.Tasks = append(ledger.Tasks, *item.Task)
	}
}
func replacePlanningItem(ledger *EngineeringLedger, id model.LedgerItemID, item *model.LedgerMutationItem) bool {
	if item == nil || item.Task == nil {
		return false
	}
	for i := range ledger.Tasks {
		if ledger.Tasks[i].ID == id {
			ledger.Tasks[i] = *item.Task
			return true
		}
	}
	return false
}
