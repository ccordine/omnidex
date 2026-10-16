package model

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Ledger records are normalized: references connect records, while every record
// keeps its own stable identity and review history.
type LedgerItemID string
type LedgerDisposition string

const (
	LedgerDispositionActive     LedgerDisposition = "active"
	LedgerDispositionTombstoned LedgerDisposition = "tombstoned"
)

type LedgerReviewState string

const (
	LedgerReviewProposed        LedgerReviewState = "proposed"
	LedgerReviewAgreed          LedgerReviewState = "agreed"
	LedgerReviewNeedsDiscussion LedgerReviewState = "needs_discussion"
)

type LedgerPriority string

const (
	LedgerPriorityCritical LedgerPriority = "critical"
	LedgerPriorityHigh     LedgerPriority = "high"
	LedgerPriorityNormal   LedgerPriority = "normal"
	LedgerPriorityLow      LedgerPriority = "low"
)

type LedgerProvenance struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	Revision  int64  `json:"revision"`
}

type LedgerItem struct {
	ID          LedgerItemID      `json:"id"`
	Provenance  LedgerProvenance  `json:"provenance"`
	Rationale   string            `json:"rationale"`
	Priority    LedgerPriority    `json:"priority"`
	Rank        int64             `json:"rank"`
	Disposition LedgerDisposition `json:"disposition"`
	ReviewState LedgerReviewState `json:"review_state,omitempty"`
}

type LedgerObjective struct {
	LedgerItem
	Outcome string `json:"outcome"`
}
type LedgerRequirement struct {
	LedgerItem
	Outcome string `json:"outcome"`
}
type LedgerConstraint struct {
	LedgerItem
	Rule string `json:"rule"`
}
type LedgerGroup struct {
	LedgerItem
	Title         string       `json:"title"`
	ParentGroupID LedgerItemID `json:"parent_group_id,omitempty"`
}
type LedgerTask struct {
	LedgerItem
	Goal                   string         `json:"goal"`
	GroupID                LedgerItemID   `json:"group_id,omitempty"`
	RequirementIDs         []LedgerItemID `json:"requirement_ids"`
	ConstraintIDs          []LedgerItemID `json:"constraint_ids"`
	AcceptanceCriterionIDs []LedgerItemID `json:"acceptance_criterion_ids"`
	DeliveryObligation     string         `json:"delivery_obligation,omitempty"`
	KnownAffectedSystems   []string       `json:"known_affected_systems,omitempty"`
	ReadFiles              []string       `json:"read_files"`
	WriteFiles             []string       `json:"write_files"`
	DeleteFiles            []string       `json:"delete_files"`
}
type LedgerAcceptanceCriterion struct {
	LedgerItem
	Outcome        string         `json:"outcome"`
	RequirementIDs []LedgerItemID `json:"requirement_ids"`
	TaskIDs        []LedgerItemID `json:"task_ids,omitempty"`
	Required       bool           `json:"required"`
}
type LedgerCheck struct {
	LedgerItem
	Argv                   []string       `json:"argv,omitempty"`
	Observation            string         `json:"observation,omitempty"`
	RequirementIDs         []LedgerItemID `json:"requirement_ids"`
	AcceptanceCriterionIDs []LedgerItemID `json:"acceptance_criterion_ids"`
	Required               bool           `json:"required"`
}

type LedgerDependencyKind string

const (
	LedgerDependencyRequires LedgerDependencyKind = "requires"
	LedgerDependencyBlocks   LedgerDependencyKind = "blocks"
)

type LedgerDependency struct {
	LedgerItem
	TaskID         LedgerItemID         `json:"task_id"`
	PrerequisiteID LedgerItemID         `json:"prerequisite_id"`
	Kind           LedgerDependencyKind `json:"kind"`
}

// EngineeringLedger has no aggregate item-count limit.
type EngineeringLedger struct {
	ID                 string                      `json:"id"`
	Revision           int64                       `json:"revision"`
	Objectives         []LedgerObjective           `json:"objectives"`
	Requirements       []LedgerRequirement         `json:"requirements"`
	Constraints        []LedgerConstraint          `json:"constraints"`
	Groups             []LedgerGroup               `json:"groups"`
	Tasks              []LedgerTask                `json:"tasks"`
	AcceptanceCriteria []LedgerAcceptanceCriterion `json:"acceptance_criteria"`
	Checks             []LedgerCheck               `json:"checks"`
	Dependencies       []LedgerDependency          `json:"dependencies"`
}

func validateLedgerTaskFiles(task LedgerTask) error {
	for _, group := range []struct {
		name  string
		files []string
	}{
		{name: "read", files: task.ReadFiles},
		{name: "write", files: task.WriteFiles},
		{name: "delete", files: task.DeleteFiles},
	} {
		seen := map[string]bool{}
		for _, file := range group.files {
			if err := ValidateTaskFileReference(file); err != nil {
				return fmt.Errorf("task %s has invalid %s file %q", task.ID, group.name, file)
			}
			if seen[file] {
				return fmt.Errorf("task %s repeats %s file %q", task.ID, group.name, file)
			}
			seen[file] = true
		}
	}
	return nil
}

// File references describe task context, not filesystem permission grants.
// They may name absolute paths, generated files, configuration, or remote paths.
func ValidateTaskFileReference(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("task file reference must be nonblank UTF-8 text of at most 4096 bytes")
	}
	return nil
}

type OwnerTurnDispositionKind string

const (
	OwnerTurnRepresented OwnerTurnDispositionKind = "represented"
	OwnerTurnOutOfScope  OwnerTurnDispositionKind = "out_of_scope"
)

type OwnerTurnDisposition struct {
	Statements []OwnerStatementDisposition `json:"statements"`
}

// OwnerStatementDisposition records one independently actionable clause. A
// turn may combine represented work and visible out-of-scope clauses without
// forcing those distinct facts into one label.
type OwnerStatementDisposition struct {
	Kind    OwnerTurnDispositionKind `json:"kind"`
	ItemIDs []LedgerItemID           `json:"item_ids,omitempty"`
	Reason  string                   `json:"reason,omitempty"`
}

func RepresentedOwnerTurn(itemIDs []LedgerItemID) OwnerTurnDisposition {
	return OwnerTurnDisposition{Statements: []OwnerStatementDisposition{{
		Kind: OwnerTurnRepresented, ItemIDs: append([]LedgerItemID(nil), itemIDs...),
	}}}
}

func OutOfScopeOwnerTurn(reason string) OwnerTurnDisposition {
	return OwnerTurnDisposition{Statements: []OwnerStatementDisposition{{
		Kind: OwnerTurnOutOfScope, Reason: reason,
	}}}
}

func (disposition OwnerStatementDisposition) Validate() error {
	switch disposition.Kind {
	case OwnerTurnRepresented:
		if err := validateLedgerItemIDs(disposition.ItemIDs, "represented owner turn"); err != nil {
			return err
		}
		if disposition.Reason != "" {
			return fmt.Errorf("represented owner turn contains fields from another disposition")
		}
	case OwnerTurnOutOfScope:
		if strings.TrimSpace(disposition.Reason) == "" {
			return fmt.Errorf("out-of-scope owner turn requires a reason")
		}
		if len(disposition.ItemIDs) != 0 {
			return fmt.Errorf("out-of-scope owner turn contains fields from another disposition")
		}
	default:
		return fmt.Errorf("owner statement requires a disposition")
	}
	return nil
}

func (disposition OwnerTurnDisposition) StatementDispositions() []OwnerStatementDisposition {
	return append([]OwnerStatementDisposition(nil), disposition.Statements...)
}

type LedgerMutationKind string

const (
	LedgerMutationAdd     LedgerMutationKind = "add"
	LedgerMutationAmend   LedgerMutationKind = "amend"
	LedgerMutationRemove  LedgerMutationKind = "remove"
	LedgerMutationRestore LedgerMutationKind = "restore"
	LedgerMutationReview  LedgerMutationKind = "set_review_state"
	LedgerMutationReorder LedgerMutationKind = "reorder"
)

type LedgerMutationItem struct {
	Objective           *LedgerObjective           `json:"objective,omitempty"`
	Requirement         *LedgerRequirement         `json:"requirement,omitempty"`
	Constraint          *LedgerConstraint          `json:"constraint,omitempty"`
	Group               *LedgerGroup               `json:"group,omitempty"`
	Task                *LedgerTask                `json:"task,omitempty"`
	AcceptanceCriterion *LedgerAcceptanceCriterion `json:"acceptance_criterion,omitempty"`
	Check               *LedgerCheck               `json:"check,omitempty"`
	Dependency          *LedgerDependency          `json:"dependency,omitempty"`
}
type LedgerMutationOperation struct {
	Kind        LedgerMutationKind  `json:"kind"`
	TargetID    LedgerItemID        `json:"target_id,omitempty"`
	Item        *LedgerMutationItem `json:"item,omitempty"`
	ReviewState LedgerReviewState   `json:"review_state,omitempty"`
	Rank        *int64              `json:"rank,omitempty"`
	Rationale   string              `json:"rationale"`
}
type LedgerMutation struct {
	OperationID          string                    `json:"operation_id"`
	BaseRevision         int64                     `json:"base_revision"`
	OwnerTurnID          string                    `json:"owner_turn_id"`
	OwnerTurnDisposition OwnerTurnDisposition      `json:"owner_turn_disposition"`
	Operations           []LedgerMutationOperation `json:"operations"`
}

func (item LedgerItem) Validate() error {
	if strings.TrimSpace(string(item.ID)) == "" {
		return fmt.Errorf("ledger item requires a stable identity")
	}
	if strings.TrimSpace(item.Provenance.Kind) == "" || strings.TrimSpace(item.Provenance.Reference) == "" || item.Provenance.Revision < 0 {
		return fmt.Errorf("ledger item %s requires provenance", item.ID)
	}
	switch item.Priority {
	case LedgerPriorityCritical, LedgerPriorityHigh, LedgerPriorityNormal, LedgerPriorityLow:
	default:
		return fmt.Errorf("ledger item %s has invalid priority", item.ID)
	}
	if item.Rank < 0 {
		return fmt.Errorf("ledger item %s has invalid rank", item.ID)
	}
	switch item.Disposition {
	case LedgerDispositionActive:
		switch item.ReviewState {
		case LedgerReviewProposed, LedgerReviewAgreed, LedgerReviewNeedsDiscussion:
		default:
			return fmt.Errorf("active ledger item %s requires a review state", item.ID)
		}
	case LedgerDispositionTombstoned:
		if item.ReviewState != "" {
			return fmt.Errorf("tombstoned ledger item %s cannot have a review state", item.ID)
		}
	default:
		return fmt.Errorf("ledger item %s has invalid disposition", item.ID)
	}
	return nil
}

func (ledger EngineeringLedger) Validate() error {
	return ledger.validate(true)
}

// ValidateDraft verifies identities, payloads, and graph shape while allowing a
// conversational revision to be temporarily incomplete. Complete task/check
// coverage and active-reference requirements are enforced again at sign-off.
func (ledger EngineeringLedger) ValidateDraft() error {
	return ledger.validate(false)
}

func (ledger EngineeringLedger) validate(complete bool) error {
	if strings.TrimSpace(ledger.ID) == "" || ledger.Revision < 0 {
		return fmt.Errorf("ledger requires an identity and non-negative revision")
	}
	ids := map[LedgerItemID]string{}
	dispositions := map[LedgerItemID]LedgerDisposition{}
	add := func(item LedgerItem, kind string) error {
		if err := item.Validate(); err != nil {
			return err
		}
		if prior := ids[item.ID]; prior != "" {
			return fmt.Errorf("ledger item %s is both %s and %s", item.ID, prior, kind)
		}
		ids[item.ID] = kind
		dispositions[item.ID] = item.Disposition
		return nil
	}
	for i := range ledger.Objectives {
		if err := add(ledger.Objectives[i].LedgerItem, "objective"); err != nil {
			return err
		}
	}
	for i := range ledger.Requirements {
		if err := add(ledger.Requirements[i].LedgerItem, "requirement"); err != nil {
			return err
		}
	}
	for i := range ledger.Constraints {
		if err := add(ledger.Constraints[i].LedgerItem, "constraint"); err != nil {
			return err
		}
	}
	for i := range ledger.Groups {
		if err := add(ledger.Groups[i].LedgerItem, "group"); err != nil {
			return err
		}
	}
	for i := range ledger.Tasks {
		if err := add(ledger.Tasks[i].LedgerItem, "task"); err != nil {
			return err
		}
	}
	for i := range ledger.AcceptanceCriteria {
		if err := add(ledger.AcceptanceCriteria[i].LedgerItem, "acceptance criterion"); err != nil {
			return err
		}
	}
	for i := range ledger.Checks {
		if err := add(ledger.Checks[i].LedgerItem, "check"); err != nil {
			return err
		}
	}
	for i := range ledger.Dependencies {
		if err := add(ledger.Dependencies[i].LedgerItem, "dependency"); err != nil {
			return err
		}
	}

	require := func(id LedgerItemID, kind string, active bool) error {
		if ids[id] != kind {
			return fmt.Errorf("ledger reference %s must identify a %s", id, kind)
		}
		if active && dispositions[id] != LedgerDispositionActive {
			return fmt.Errorf("active ledger relationship requires active %s %s", kind, id)
		}
		return nil
	}
	for _, value := range ledger.Objectives {
		if strings.TrimSpace(value.Outcome) == "" {
			return fmt.Errorf("objective %s requires an outcome", value.ID)
		}
	}
	for _, value := range ledger.Requirements {
		if strings.TrimSpace(value.Outcome) == "" {
			return fmt.Errorf("requirement %s requires an outcome", value.ID)
		}
	}
	for _, value := range ledger.Constraints {
		if strings.TrimSpace(value.Rule) == "" {
			return fmt.Errorf("constraint %s requires a rule", value.ID)
		}
	}
	for _, value := range ledger.Groups {
		if strings.TrimSpace(value.Title) == "" {
			return fmt.Errorf("group %s requires a title", value.ID)
		}
		if value.ParentGroupID != "" {
			if err := require(value.ParentGroupID, "group", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
	}
	for _, value := range ledger.Tasks {
		if strings.TrimSpace(value.Goal) == "" {
			return fmt.Errorf("task %s requires a goal", value.ID)
		}
		if value.GroupID != "" {
			if err := require(value.GroupID, "group", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
		if complete && len(value.RequirementIDs) == 0 && strings.TrimSpace(value.DeliveryObligation) == "" {
			return fmt.Errorf("task %s requires a requirement or delivery obligation", value.ID)
		}
		if complete && len(value.AcceptanceCriterionIDs) == 0 {
			return fmt.Errorf("task %s requires acceptance criteria", value.ID)
		}
		for _, id := range value.RequirementIDs {
			if err := require(id, "requirement", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
		for _, id := range value.ConstraintIDs {
			if err := require(id, "constraint", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
		for _, id := range value.AcceptanceCriterionIDs {
			if err := require(id, "acceptance criterion", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
		if err := validateLedgerTaskFiles(value); err != nil {
			return err
		}
	}
	for _, value := range ledger.AcceptanceCriteria {
		if strings.TrimSpace(value.Outcome) == "" || complete && len(value.RequirementIDs) == 0 {
			return fmt.Errorf("acceptance criterion %s requires an outcome and requirements", value.ID)
		}
		for _, id := range value.RequirementIDs {
			if err := require(id, "requirement", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
		for _, id := range value.TaskIDs {
			if err := require(id, "task", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
	}
	for _, value := range ledger.Checks {
		if (len(value.Argv) == 0) == (strings.TrimSpace(value.Observation) == "") {
			return fmt.Errorf("check %s requires exactly one command or observation", value.ID)
		}
		if len(value.Argv) > 0 && strings.TrimSpace(value.Argv[0]) == "" {
			return fmt.Errorf("check %s requires a nonblank command executable", value.ID)
		}
		if complete && (len(value.RequirementIDs) == 0 || len(value.AcceptanceCriterionIDs) == 0) {
			return fmt.Errorf("check %s requires explicit requirement and acceptance bindings", value.ID)
		}
		for _, id := range value.RequirementIDs {
			if err := require(id, "requirement", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
		for _, id := range value.AcceptanceCriterionIDs {
			if err := require(id, "acceptance criterion", complete && value.Disposition == LedgerDispositionActive); err != nil {
				return err
			}
		}
	}
	for _, value := range ledger.Dependencies {
		if value.TaskID == value.PrerequisiteID || (value.Kind != LedgerDependencyRequires && value.Kind != LedgerDependencyBlocks) {
			return fmt.Errorf("dependency %s is invalid", value.ID)
		}
		if err := require(value.TaskID, "task", complete && value.Disposition == LedgerDispositionActive); err != nil {
			return err
		}
		if err := require(value.PrerequisiteID, "task", complete && value.Disposition == LedgerDispositionActive); err != nil {
			return err
		}
	}
	// Validate normalized coverage without flattening it into task text.
	taskCoverage := map[LedgerItemID]bool{}
	acceptanceCoverage := map[LedgerItemID]bool{}
	checkCoverage := map[LedgerItemID]bool{}
	for _, task := range ledger.Tasks {
		if task.Disposition != LedgerDispositionActive {
			continue
		}
		for _, id := range task.RequirementIDs {
			taskCoverage[id] = true
		}
	}
	for _, criterion := range ledger.AcceptanceCriteria {
		if criterion.Disposition != LedgerDispositionActive {
			continue
		}
		for _, id := range criterion.RequirementIDs {
			acceptanceCoverage[id] = true
		}
	}
	for _, check := range ledger.Checks {
		if check.Disposition != LedgerDispositionActive {
			continue
		}
		for _, id := range check.AcceptanceCriterionIDs {
			checkCoverage[id] = true
		}
	}
	for _, requirement := range ledger.Requirements {
		if !complete {
			break
		}
		if requirement.Disposition == LedgerDispositionActive && (!taskCoverage[requirement.ID] || !acceptanceCoverage[requirement.ID]) {
			return fmt.Errorf("requirement %s requires active task and acceptance coverage", requirement.ID)
		}
	}
	for _, criterion := range ledger.AcceptanceCriteria {
		if !complete {
			break
		}
		if criterion.Disposition == LedgerDispositionActive && criterion.Required && !checkCoverage[criterion.ID] {
			return fmt.Errorf("required acceptance criterion %s requires an active check", criterion.ID)
		}
	}

	// Dependency records remain normalized edges; code, not a planner, proves
	// that the active prerequisite graph is acyclic.
	edges := map[LedgerItemID][]LedgerItemID{}
	for _, dependency := range ledger.Dependencies {
		if dependency.Disposition == LedgerDispositionActive {
			edges[dependency.TaskID] = append(edges[dependency.TaskID], dependency.PrerequisiteID)
		}
	}
	visiting, visited := map[LedgerItemID]bool{}, map[LedgerItemID]bool{}
	var visit func(LedgerItemID) error
	visit = func(id LedgerItemID) error {
		if visiting[id] {
			return fmt.Errorf("ledger dependency cycle contains task %s", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, prerequisite := range edges[id] {
			if err := visit(prerequisite); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range edges {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func (mutation LedgerMutation) Validate() error {
	if strings.TrimSpace(mutation.OperationID) == "" || mutation.BaseRevision < 0 || strings.TrimSpace(mutation.OwnerTurnID) == "" {
		return fmt.Errorf("ledger mutation requires operation identity, exact base revision and owner turn")
	}
	disposition := mutation.OwnerTurnDisposition
	statements := disposition.StatementDispositions()
	if len(statements) == 0 {
		return fmt.Errorf("ledger mutation requires owner-statement dispositions")
	}
	hasMaterialDisposition := false
	for _, statement := range statements {
		if statement.Kind != OwnerTurnOutOfScope {
			hasMaterialDisposition = true
		}
	}
	if len(mutation.Operations) == 0 && hasMaterialDisposition {
		return fmt.Errorf("ledger mutation requires typed operations")
	}
	for _, disposition := range statements {
		if err := disposition.Validate(); err != nil {
			return err
		}
		if disposition.Kind == OwnerTurnOutOfScope && len(statements) == 1 && len(mutation.Operations) != 0 {
			return fmt.Errorf("out-of-scope owner turn contains fields or operations from another disposition")
		}
	}
	seenOperations := make([]LedgerMutationOperation, 0, len(mutation.Operations))
	for _, operation := range mutation.Operations {
		for _, prior := range seenOperations {
			if reflect.DeepEqual(prior, operation) {
				return fmt.Errorf("ledger mutation contains an exact duplicate operation")
			}
		}
		seenOperations = append(seenOperations, operation)
		targetSet := operation.TargetID != ""
		targetValid := strings.TrimSpace(string(operation.TargetID)) != ""
		switch operation.Kind {
		case LedgerMutationAdd, LedgerMutationAmend:
			if operation.Item == nil || operation.Item.count() != 1 {
				return fmt.Errorf("%s operation requires exactly one typed item", operation.Kind)
			}
			if operation.ReviewState != "" || operation.Rank != nil {
				return fmt.Errorf("%s operation contains fields from another operation", operation.Kind)
			}
			if operation.Kind == LedgerMutationAdd && targetSet {
				return fmt.Errorf("add operation contains fields from another operation")
			}
			if operation.Kind == LedgerMutationAmend && !targetValid {
				return fmt.Errorf("amend operation requires a target")
			}
			payloadID, err := operation.Item.validate()
			if err != nil {
				return fmt.Errorf("%s operation has invalid payload: %w", operation.Kind, err)
			}
			if operation.Kind == LedgerMutationAdd {
				base := operation.Item.ledgerItem()
				if base.Disposition != LedgerDispositionActive || base.ReviewState != LedgerReviewProposed {
					return fmt.Errorf("add operation item %s must start active and proposed", payloadID)
				}
			}
			if operation.Kind == LedgerMutationAmend && payloadID != operation.TargetID {
				return fmt.Errorf("amend operation payload identity %s does not match target %s", payloadID, operation.TargetID)
			}
		case LedgerMutationRemove, LedgerMutationRestore:
			if !targetValid || operation.Item != nil || operation.ReviewState != "" || operation.Rank != nil {
				return fmt.Errorf("%s operation requires only a target", operation.Kind)
			}
		case LedgerMutationReview:
			if !targetValid || operation.Item != nil || operation.Rank != nil ||
				(operation.ReviewState != LedgerReviewProposed && operation.ReviewState != LedgerReviewAgreed && operation.ReviewState != LedgerReviewNeedsDiscussion) {
				return fmt.Errorf("review operation requires only a target and valid state")
			}
		case LedgerMutationReorder:
			if !targetValid || operation.Item != nil || operation.ReviewState != "" || operation.Rank == nil || *operation.Rank < 0 {
				return fmt.Errorf("reorder operation requires only a target and rank")
			}
		default:
			return fmt.Errorf("unknown ledger mutation operation %q", operation.Kind)
		}
	}
	return nil
}
func (item LedgerMutationItem) ledgerItem() LedgerItem {
	if base := item.Base(); base != nil {
		return *base
	}
	return LedgerItem{}
}

func (item LedgerMutationItem) count() int {
	count := 0
	for _, present := range []bool{item.Objective != nil, item.Requirement != nil, item.Constraint != nil, item.Group != nil, item.Task != nil, item.AcceptanceCriterion != nil, item.Check != nil, item.Dependency != nil} {
		if present {
			count++
		}
	}
	return count
}

func validateLedgerItemIDs(ids []LedgerItemID, subject string) error {
	if len(ids) == 0 {
		return fmt.Errorf("%s requires item references", subject)
	}
	for _, id := range ids {
		if strings.TrimSpace(string(id)) == "" {
			return fmt.Errorf("%s contains a blank item reference", subject)
		}
	}
	return nil
}

func (item LedgerMutationItem) validate() (LedgerItemID, error) {
	requireIDs := func(ids []LedgerItemID, subject string) error {
		for _, id := range ids {
			if strings.TrimSpace(string(id)) == "" {
				return fmt.Errorf("%s contains a blank item reference", subject)
			}
		}
		return nil
	}
	validateBase := func(base LedgerItem) (LedgerItemID, error) {
		if err := base.Validate(); err != nil {
			return "", err
		}
		return base.ID, nil
	}

	switch {
	case item.Objective != nil:
		id, err := validateBase(item.Objective.LedgerItem)
		if err == nil && strings.TrimSpace(item.Objective.Outcome) == "" {
			err = fmt.Errorf("objective %s requires an outcome", id)
		}
		return id, err
	case item.Requirement != nil:
		id, err := validateBase(item.Requirement.LedgerItem)
		if err == nil && strings.TrimSpace(item.Requirement.Outcome) == "" {
			err = fmt.Errorf("requirement %s requires an outcome", id)
		}
		return id, err
	case item.Constraint != nil:
		id, err := validateBase(item.Constraint.LedgerItem)
		if err == nil && strings.TrimSpace(item.Constraint.Rule) == "" {
			err = fmt.Errorf("constraint %s requires a rule", id)
		}
		return id, err
	case item.Group != nil:
		id, err := validateBase(item.Group.LedgerItem)
		if err == nil && strings.TrimSpace(item.Group.Title) == "" {
			err = fmt.Errorf("group %s requires a title", id)
		}
		return id, err
	case item.Task != nil:
		id, err := validateBase(item.Task.LedgerItem)
		if err != nil {
			return id, err
		}
		if strings.TrimSpace(item.Task.Goal) == "" {
			return id, fmt.Errorf("task %s requires a goal", id)
		}
		if err := requireIDs(item.Task.AcceptanceCriterionIDs, "task acceptance bindings"); err != nil {
			return id, err
		}
		for _, requirementID := range item.Task.RequirementIDs {
			if strings.TrimSpace(string(requirementID)) == "" {
				return id, fmt.Errorf("task requirement bindings contain a blank item reference")
			}
		}
		for _, constraintID := range item.Task.ConstraintIDs {
			if strings.TrimSpace(string(constraintID)) == "" {
				return id, fmt.Errorf("task constraint bindings contain a blank item reference")
			}
		}
		if err := validateLedgerTaskFiles(*item.Task); err != nil {
			return id, err
		}
		return id, nil
	case item.AcceptanceCriterion != nil:
		id, err := validateBase(item.AcceptanceCriterion.LedgerItem)
		if err != nil {
			return id, err
		}
		if strings.TrimSpace(item.AcceptanceCriterion.Outcome) == "" {
			return id, fmt.Errorf("acceptance criterion %s requires an outcome", id)
		}
		if err := requireIDs(item.AcceptanceCriterion.RequirementIDs, "acceptance criterion requirement bindings"); err != nil {
			return id, err
		}
		for _, taskID := range item.AcceptanceCriterion.TaskIDs {
			if strings.TrimSpace(string(taskID)) == "" {
				return id, fmt.Errorf("acceptance criterion task bindings contain a blank item reference")
			}
		}
		return id, nil
	case item.Check != nil:
		id, err := validateBase(item.Check.LedgerItem)
		if err != nil {
			return id, err
		}
		if (len(item.Check.Argv) == 0) == (strings.TrimSpace(item.Check.Observation) == "") {
			return id, fmt.Errorf("check %s requires exactly one command or observation", id)
		}
		if len(item.Check.Argv) > 0 && strings.TrimSpace(item.Check.Argv[0]) == "" {
			return id, fmt.Errorf("check %s requires a nonblank command executable", id)
		}
		if err := requireIDs(item.Check.RequirementIDs, "check requirement bindings"); err != nil {
			return id, err
		}
		if err := requireIDs(item.Check.AcceptanceCriterionIDs, "check acceptance bindings"); err != nil {
			return id, err
		}
		return id, nil
	case item.Dependency != nil:
		id, err := validateBase(item.Dependency.LedgerItem)
		if err != nil {
			return id, err
		}
		if strings.TrimSpace(string(item.Dependency.TaskID)) == "" ||
			strings.TrimSpace(string(item.Dependency.PrerequisiteID)) == "" ||
			item.Dependency.TaskID == item.Dependency.PrerequisiteID ||
			(item.Dependency.Kind != LedgerDependencyRequires && item.Dependency.Kind != LedgerDependencyBlocks) {
			return id, fmt.Errorf("dependency %s is invalid", id)
		}
		return id, nil
	default:
		return "", fmt.Errorf("typed item payload is required")
	}
}
