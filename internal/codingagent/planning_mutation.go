package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	initialPlanningQuestion  = "initial-ledger-proposal"
	targetPlanningQuestion   = "owner-turn-target-resolution"
	mutationPlanningQuestion = "owner-turn-ledger-mutation"
	defaultPlanningItemLimit = 64
)

// PlanningTurn is the complete authority for one conversational edit. The
// observed revision is supplied by the client and is never inferred from model
// output or conversation memory.
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

// InitialPlanningProposal is the only provider result allowed to contain a
// complete ledger. Subsequent provider calls return LedgerMutation instead.
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
			Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{action.TargetID},
		},
		Operations: []model.LedgerMutationOperation{operation},
	}
	if err := ValidateLedgerMutation(turn.ObservedRevision, mutation); err != nil {
		return LedgerMutation{}, err
	}
	return mutation, nil
}

const maxInitialPlanningContractAttempts = 3

// ProposePlanningLedger is the one station permitted to return a full ledger.
// A proposal that fails the deterministic contract is never persisted; a
// bounded fresh replacement receives only the exact code validation error.
func (controller *Controller) ProposePlanningLedger(ctx context.Context, turn PlanningTurn) (InitialPlanningProposal, error) {
	if err := controller.validatePlanning(); err != nil {
		return InitialPlanningProposal{}, err
	}
	if err := turn.validate(true); err != nil {
		return InitialPlanningProposal{}, err
	}
	files, err := controller.Environment.ReadFiles(ctx)
	if err != nil {
		return InitialPlanningProposal{}, err
	}
	sort.Strings(files)
	call := controller.planningAssignment(PlanningInitial, initialPlanningQuestion,
		"What normalized engineering ledger exactly represents this initial owner objective?", turn, nil)
	call.Requirements, call.Objective, call.ReadFiles = controller.Requirements, controller.Objective, files
	call.ArtifactBindings, call.PlanningEnvironment = controller.ArtifactBindings, controller.PlanningEnvironment
	call.ConstructionFacts = append(call.ConstructionFacts, controller.ConstructionFacts...)

	var contractErr error
	for attempt := 1; attempt <= maxInitialPlanningContractAttempts; attempt++ {
		if contractErr != nil {
			call.ValidationNotes = []string{"Code rejected the prior complete proposal: " + contractErr.Error() + ". Return a complete replacement that satisfies this exact contract."}
		}
		result, err := controller.invoke(ctx, call, int64(attempt))
		if err != nil {
			return InitialPlanningProposal{}, err
		}
		var proposal InitialPlanningProposal
		if err := decodeContract(result.Content, &proposal); err != nil {
			contractErr = err
			continue
		}
		proposal.Ledger.Revision = 1
		if err := validateInitialPlanningProposal(proposal); err != nil {
			contractErr = err
			continue
		}
		return proposal, nil
	}
	return InitialPlanningProposal{}, fmt.Errorf("initial planning proposal remained invalid after %d attempts: %w", maxInitialPlanningContractAttempts, contractErr)
}

func validateInitialPlanningProposal(proposal InitialPlanningProposal) error {
	if err := proposal.Ledger.Validate(); err != nil {
		return fmt.Errorf("initial planning ledger: %w", err)
	}
	activeObjectives := 0
	for _, objective := range proposal.Ledger.Objectives {
		if objective.Disposition == model.LedgerDispositionActive {
			activeObjectives++
		}
	}
	if activeObjectives != 1 {
		return fmt.Errorf("initial planning ledger requires exactly one active objective")
	}
	if err := validateInitialTurnDisposition(proposal.Ledger, proposal.OwnerTurnDisposition); err != nil {
		return err
	}
	return nil
}

// PlanPlanningMutation asks one narrow semantic question over a bounded slice
// of the exact persisted revision. The provider cannot choose operation, turn,
// or base-revision authority.
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
	if err := ledger.ValidateDraft(); err != nil {
		return LedgerMutation{}, err
	}
	relevant, err := controller.resolvePlanningTargets(ctx, ledger, turn)
	if err != nil {
		return LedgerMutation{}, err
	}
	if len(relevant) > defaultPlanningItemLimit {
		return LedgerMutation{}, fmt.Errorf("owner turn resolves to %d explicit ledger records; split it into turns of at most %d records", len(relevant), defaultPlanningItemLimit)
	}
	contextLedger := BoundedPlanningLedger(ledger, relevant, defaultPlanningItemLimit)
	call := controller.planningAssignment(PlanningMutation, mutationPlanningQuestion,
		"Which typed ledger operations exactly represent this owner turn?", turn, &contextLedger)
	result, err := controller.invoke(ctx, call, 1)
	if err != nil {
		return LedgerMutation{}, err
	}
	var mutation LedgerMutation
	if err := decodeContract(result.Content, &mutation); err != nil {
		return LedgerMutation{}, err
	}
	mutation.OperationID = turn.OperationID
	mutation.OwnerTurnID = turn.OwnerTurnID
	mutation.BaseRevision = turn.ObservedRevision
	if err := ValidateLedgerMutation(ledger.Revision, mutation); err != nil {
		return LedgerMutation{}, err
	}
	return mutation, nil
}

type PlanningTargetSelection struct {
	Question     string               `json:"question"`
	BaseRevision int64                `json:"base_revision"`
	ItemIDs      []model.LedgerItemID `json:"item_ids"`
}

// resolvePlanningTargets performs semantic reference resolution before the
// mutation context is bounded. Each provider call sees at most one partition
// and answers only whether records in that partition are referenced.
func (controller *Controller) resolvePlanningTargets(ctx context.Context, ledger EngineeringLedger, turn PlanningTurn) ([]model.LedgerItemID, error) {
	resolved := PlanningRelevantItemIDs(ledger, turn.Text)
	items := planningItemPointers(&ledger)
	if len(items) <= defaultPlanningItemLimit {
		return resolved, nil
	}
	seen := make(map[model.LedgerItemID]bool, len(resolved))
	for _, id := range resolved {
		seen[id] = true
	}
	for offset := 0; offset < len(items); offset += defaultPlanningItemLimit {
		end := offset + defaultPlanningItemLimit
		if end > len(items) {
			end = len(items)
		}
		partitionIDs := make([]model.LedgerItemID, 0, end-offset)
		allowed := make(map[model.LedgerItemID]bool, end-offset)
		for _, item := range items[offset:end] {
			partitionIDs = append(partitionIDs, item.ID)
			allowed[item.ID] = true
		}
		partition := BoundedPlanningLedger(ledger, partitionIDs, defaultPlanningItemLimit)
		question := fmt.Sprintf("Which records in persisted ledger partition %d/%d are semantically referenced by this exact owner turn?", offset/defaultPlanningItemLimit+1, (len(items)+defaultPlanningItemLimit-1)/defaultPlanningItemLimit)
		call := controller.planningAssignment(PlanningTarget, targetPlanningQuestion, question, turn, &partition)
		result, err := controller.invoke(ctx, call, 1)
		if err != nil {
			return nil, err
		}
		var selection PlanningTargetSelection
		if err := decodeContract(result.Content, &selection); err != nil {
			return nil, err
		}
		if selection.Question != targetPlanningQuestion || selection.BaseRevision != ledger.Revision {
			return nil, fmt.Errorf("planning target selection does not answer %q at revision %d", targetPlanningQuestion, ledger.Revision)
		}
		for _, id := range selection.ItemIDs {
			if !allowed[id] {
				return nil, fmt.Errorf("planning target selection returned item %q outside its bounded partition", id)
			}
			if !seen[id] {
				seen[id] = true
				resolved = append(resolved, id)
			}
		}
	}
	return resolved, nil
}

// PlanningRelevantItemIDs resolves only exact stable identities. Semantic
// references and paraphrases are resolved by bounded PlanningTarget calls.
func PlanningRelevantItemIDs(ledger EngineeringLedger, exactTurn string) []model.LedgerItemID {
	seen := map[model.LedgerItemID]bool{}
	result := make([]model.LedgerItemID, 0)
	for _, reference := range planningItemReferences(&ledger) {
		matched := reference.id != "" && strings.Contains(exactTurn, string(reference.id))
		if matched && !seen[reference.id] {
			seen[reference.id] = true
			result = append(result, reference.id)
		}
	}
	return result
}

type planningItemReference struct {
	id   model.LedgerItemID
	text []string
}

func planningItemReferences(ledger *EngineeringLedger) []planningItemReference {
	result := make([]planningItemReference, 0, len(planningItemPointers(ledger)))
	for _, value := range ledger.Objectives {
		result = append(result, planningItemReference{value.ID, []string{value.Outcome}})
	}
	for _, value := range ledger.Requirements {
		result = append(result, planningItemReference{value.ID, []string{value.Outcome}})
	}
	for _, value := range ledger.Constraints {
		result = append(result, planningItemReference{value.ID, []string{value.Rule}})
	}
	for _, value := range ledger.Groups {
		result = append(result, planningItemReference{value.ID, []string{value.Title}})
	}
	for _, value := range ledger.Tasks {
		result = append(result, planningItemReference{value.ID, []string{value.Goal, value.DeliveryObligation}})
	}
	for _, value := range ledger.AcceptanceCriteria {
		result = append(result, planningItemReference{value.ID, []string{value.Outcome}})
	}
	for _, value := range ledger.Checks {
		result = append(result, planningItemReference{value.ID, []string{value.Observation, strings.Join(value.Argv, " ")}})
	}
	for _, value := range ledger.DecisionsAndQuestions {
		result = append(result, planningItemReference{value.ID, []string{value.Prompt, value.Answer}})
	}
	for _, value := range ledger.Dependencies {
		result = append(result, planningItemReference{value.ID, nil})
	}
	for _, value := range ledger.Findings {
		result = append(result, planningItemReference{value.ID, []string{value.Observation}})
	}
	return result
}

func (controller *Controller) planningAssignment(role Role, name, question string, turn PlanningTurn, ledger *EngineeringLedger) Assignment {
	call := controller.assignment(role, nil, controller.Configuration.Planner)
	call.PlanningQuestionID = name
	call.PlanningBaseRevision = turn.ObservedRevision
	payload := struct {
		OwnerTurn *PlanningTurn      `json:"owner_turn,omitempty"`
		Ledger    *EngineeringLedger `json:"ledger,omitempty"`
	}{Ledger: ledger}
	if role != PlanningAudit {
		payload.OwnerTurn = &turn
	}
	encoded, _ := json.Marshal(payload)
	call.ConstructionFacts = []ConstructionFact{
		{Kind: "engineering_question", Quote: name + ": " + question},
		{Kind: "bounded_planning_context", Quote: string(encoded)},
	}
	return call
}

// ApplyPlanningMutation is the code-owned mechanical preview used before a
// repository transaction and coverage call. The repository remains the durable
// transaction authority.
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
	case item.DecisionQuestion != nil:
		return item.DecisionQuestion.ID
	case item.Dependency != nil:
		return item.Dependency.ID
	case item.Finding != nil:
		return item.Finding.ID
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
	case item.DecisionQuestion != nil:
		ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, *item.DecisionQuestion)
	case item.Dependency != nil:
		ledger.Dependencies = append(ledger.Dependencies, *item.Dependency)
	case item.Finding != nil:
		ledger.Findings = append(ledger.Findings, *item.Finding)
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
	case item.DecisionQuestion != nil:
		for i := range ledger.DecisionsAndQuestions {
			if ledger.DecisionsAndQuestions[i].ID == id {
				ledger.DecisionsAndQuestions[i] = *item.DecisionQuestion
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
	case item.Finding != nil:
		for i := range ledger.Findings {
			if ledger.Findings[i].ID == id {
				ledger.Findings[i] = *item.Finding
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
	for i := range ledger.DecisionsAndQuestions {
		items = append(items, &ledger.DecisionsAndQuestions[i].LedgerItem)
	}
	for i := range ledger.Dependencies {
		items = append(items, &ledger.Dependencies[i].LedgerItem)
	}
	for i := range ledger.Findings {
		items = append(items, &ledger.Findings[i].LedgerItem)
	}
	return items
}

func validateInitialTurnDisposition(ledger EngineeringLedger, disposition model.OwnerTurnDisposition) error {
	known := map[model.LedgerItemID]bool{}
	questions := map[model.LedgerItemID]bool{}
	for _, item := range planningItemPointers(&ledger) {
		known[item.ID] = true
	}
	for _, question := range ledger.DecisionsAndQuestions {
		questions[question.ID] = question.Kind == model.LedgerQuestion
	}
	for _, disposition := range disposition.StatementDispositions() {
		switch disposition.Kind {
		case model.OwnerTurnRepresented:
			if disposition.Reason != "" || len(disposition.QuestionIDs) != 0 {
				return fmt.Errorf("represented initial owner turn contains fields from another disposition")
			}
			if len(disposition.ItemIDs) == 0 {
				return fmt.Errorf("initial owner turn requires represented ledger items")
			}
			for _, id := range disposition.ItemIDs {
				if !known[id] {
					return fmt.Errorf("initial owner turn references unknown item %s", id)
				}
			}
		case model.OwnerTurnUnresolved:
			if disposition.Reason != "" || len(disposition.ItemIDs) != 0 {
				return fmt.Errorf("unresolved initial owner turn contains fields from another disposition")
			}
			if len(disposition.QuestionIDs) == 0 {
				return fmt.Errorf("unresolved initial owner turn requires a question")
			}
			for _, id := range disposition.QuestionIDs {
				if !questions[id] {
					return fmt.Errorf("initial owner turn references non-question %s", id)
				}
			}
		case model.OwnerTurnOutOfScope:
			if len(disposition.ItemIDs) != 0 || len(disposition.QuestionIDs) != 0 {
				return fmt.Errorf("out-of-scope initial owner turn contains represented items")
			}
			if strings.TrimSpace(disposition.Reason) == "" {
				return fmt.Errorf("out-of-scope initial owner turn requires a reason")
			}
		default:
			return fmt.Errorf("initial owner turn requires an explicit disposition")
		}
	}
	return nil
}
