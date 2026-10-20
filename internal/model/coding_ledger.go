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

type LedgerProvenance struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	Revision  int64  `json:"revision"`
}

type LedgerItem struct {
	ID          LedgerItemID      `json:"id"`
	Provenance  LedgerProvenance  `json:"provenance"`
	Rationale   string            `json:"rationale"`
	Rank        int64             `json:"rank"`
	Disposition LedgerDisposition `json:"disposition"`
	ReviewState LedgerReviewState `json:"review_state,omitempty"`
}

type LedgerTask struct {
	LedgerItem
	Goal                 string                   `json:"goal"`
	Objective            string                   `json:"objective,omitempty"`
	OwnerContext         []TaskContext            `json:"owner_context,omitempty"`
	Constraints          []string                 `json:"constraints,omitempty"`
	FileResponsibilities []TaskFileResponsibility `json:"file_responsibilities,omitempty"`
	Acceptance           []TaskAcceptance         `json:"acceptance,omitempty"`
	ReadFiles            []string                 `json:"read_files"`
	WriteFiles           []string                 `json:"write_files"`
	DeleteFiles          []string                 `json:"delete_files"`
}

// These values belong to one task, never independently scheduled ledger items.
// Goal retains the original instructions; context retains its original source.
type TaskContext struct {
	Text       string           `json:"text"`
	Provenance LedgerProvenance `json:"provenance"`
}

type TaskFileAction string

const (
	TaskFileInspect TaskFileAction = "inspect"
	TaskFileChange  TaskFileAction = "change"
	TaskFileCreate  TaskFileAction = "create"
	TaskFileDelete  TaskFileAction = "delete"
)

type TaskFileResponsibility struct {
	Path           string         `json:"path"`
	Action         TaskFileAction `json:"action"`
	Responsibility string         `json:"responsibility"`
}

// Verification is nested under the criterion it proves, so sparse amendments
// and task projection cannot detach checks from their acceptance obligation.
type TaskAcceptance struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	Verification []string `json:"verification"`
}

func (task LedgerTask) Instructions() string {
	var out strings.Builder
	out.WriteString(task.Goal)
	if task.Objective != "" {
		fmt.Fprintf(&out, "\n\nObjective: %s", task.Objective)
	}
	for _, context := range task.OwnerContext {
		fmt.Fprintf(&out, "\n\nOwner context (%s %s revision %d):\n%s", context.Provenance.Kind, context.Provenance.Reference, context.Provenance.Revision, context.Text)
	}
	for _, constraint := range task.Constraints {
		fmt.Fprintf(&out, "\nConstraint: %s", constraint)
	}
	for _, group := range []struct {
		name  string
		files []string
	}{{"Inspect", task.ReadFiles}, {"Change/create", task.WriteFiles}, {"Delete", task.DeleteFiles}} {
		for _, file := range group.files {
			fmt.Fprintf(&out, "\n%s: %s", group.name, file)
		}
	}
	for _, file := range task.FileResponsibilities {
		fmt.Fprintf(&out, "\n%s %s: %s", file.Action, file.Path, file.Responsibility)
	}
	for _, criterion := range task.Acceptance {
		fmt.Fprintf(&out, "\n\nAcceptance %s: %s", criterion.ID, criterion.Text)
		for _, check := range criterion.Verification {
			fmt.Fprintf(&out, "\nVerify %s: %s", criterion.ID, check)
		}
	}
	return out.String()
}

func (task LedgerTask) ValidatePacket() error {
	for _, context := range task.OwnerContext {
		if strings.TrimSpace(context.Text) == "" || strings.TrimSpace(context.Provenance.Kind) == "" || strings.TrimSpace(context.Provenance.Reference) == "" || context.Provenance.Revision < 0 {
			return fmt.Errorf("task %s context requires text and provenance", task.ID)
		}
	}
	for _, constraint := range task.Constraints {
		if strings.TrimSpace(constraint) == "" {
			return fmt.Errorf("task %s has a blank constraint", task.ID)
		}
	}
	for _, file := range task.FileResponsibilities {
		if err := ValidateTaskFileReference(file.Path); err != nil {
			return err
		}
		var files []string
		switch file.Action {
		case TaskFileInspect:
			files = task.ReadFiles
		case TaskFileChange, TaskFileCreate:
			files = task.WriteFiles
		case TaskFileDelete:
			files = task.DeleteFiles
		default:
			return fmt.Errorf("task %s has invalid file action %q", task.ID, file.Action)
		}
		found := false
		for _, path := range files {
			found = found || path == file.Path
		}
		if !found || strings.TrimSpace(file.Responsibility) == "" {
			return fmt.Errorf("task %s has an unbound file responsibility for %s", task.ID, file.Path)
		}
	}
	seen := map[string]bool{}
	for _, criterion := range task.Acceptance {
		if strings.TrimSpace(criterion.ID) == "" || seen[criterion.ID] || strings.TrimSpace(criterion.Text) == "" || len(criterion.Verification) == 0 {
			return fmt.Errorf("task %s requires distinct acceptance criteria with bound verification", task.ID)
		}
		seen[criterion.ID] = true
		for _, check := range criterion.Verification {
			if strings.TrimSpace(check) == "" {
				return fmt.Errorf("task %s has blank verification", task.ID)
			}
		}
	}
	return nil
}

// EngineeringLedger contains only the ordered instructions approved for execution.
type EngineeringLedger struct {
	ID       string       `json:"id"`
	Revision int64        `json:"revision"`
	Tasks    []LedgerTask `json:"tasks"`
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
	Task *LedgerTask `json:"task,omitempty"`
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
	if strings.TrimSpace(ledger.ID) == "" || ledger.Revision < 0 {
		return fmt.Errorf("ledger requires an identity and non-negative revision")
	}
	seen := map[LedgerItemID]bool{}
	for _, task := range ledger.Tasks {
		if seen[task.ID] {
			return fmt.Errorf("duplicate task %s", task.ID)
		}
		seen[task.ID] = true
		if _, err := (LedgerMutationItem{Task: &task}).validate(); err != nil {
			return err
		}
	}
	return nil
}
func (ledger EngineeringLedger) ValidateDraft() error { return ledger.Validate() }

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

func (item LedgerMutationItem) count() int {
	if item.Task != nil {
		return 1
	}
	return 0
}
func (item LedgerMutationItem) validate() (LedgerItemID, error) {
	if item.Task == nil {
		return "", fmt.Errorf("task payload is required")
	}
	task := item.Task
	if err := task.LedgerItem.Validate(); err != nil {
		return task.ID, err
	}
	if strings.TrimSpace(task.Goal) == "" {
		return task.ID, fmt.Errorf("task %s requires instructions", task.ID)
	}
	if err := task.ValidatePacket(); err != nil {
		return task.ID, err
	}
	return task.ID, validateLedgerTaskFiles(*task)
}
