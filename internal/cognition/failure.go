package cognition

import "fmt"

type ActionFailureCode string

const (
	ActionFailureInvalidAction       ActionFailureCode = "invalid_action"
	ActionFailurePreconditionFailed  ActionFailureCode = "precondition_failed"
	ActionFailureUnauthorized        ActionFailureCode = "unauthorized"
	ActionFailureStaleRevision       ActionFailureCode = "stale_revision"
	ActionFailureIdempotencyConflict ActionFailureCode = "idempotency_conflict"
	ActionFailureTerminal            ActionFailureCode = "terminal"
	ActionFailureBudget              ActionFailureCode = "budget"
)

// ActionFailure is the public, typed rejection returned instead of a state
// transition. Revision is required to equal the caller's expected revision.
type ActionFailure struct {
	Code          ActionFailureCode `json:"code"`
	ActionID      ActionID          `json:"action_id"`
	Revision      WorldRevision     `json:"revision"`
	PublicMessage string            `json:"public_message"`
	EvidenceRefs  []EvidenceRef     `json:"evidence_refs"`
}

func NewActionFailure(
	code ActionFailureCode,
	action RegisteredAction,
	expected WorldRevision,
	publicMessage string,
	evidenceRefs []EvidenceRef,
) (ActionFailure, error) {
	failure := ActionFailure{
		Code: code, ActionID: action.ID, Revision: expected,
		PublicMessage: publicMessage,
		EvidenceRefs:  cloneSlice(evidenceRefs),
	}
	if err := failure.Validate(action, expected); err != nil {
		return ActionFailure{}, err
	}
	return failure, nil
}

func (failure ActionFailure) Error() string {
	return fmt.Sprintf("%v: action=%s code=%s: %s", ErrActionFailed, failure.ActionID, failure.Code, failure.PublicMessage)
}

func (failure ActionFailure) Unwrap() error { return ErrActionFailed }

func (failure ActionFailure) Validate(action RegisteredAction, expected WorldRevision) error {
	if !registeredFailureCode(failure.Code) {
		return fmt.Errorf("%w: code %q is not registered", ErrInvalidFailure, failure.Code)
	}
	if err := action.validateBase(); err != nil {
		return fmt.Errorf("%w: action: %v", ErrInvalidFailure, err)
	}
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("%w: expected revision: %v", ErrInvalidFailure, err)
	}
	if failure.ActionID != action.ID {
		return fmt.Errorf("%w: failure action identity does not match the rejected action", ErrInvalidFailure)
	}
	if failure.Revision != expected {
		return fmt.Errorf("%w: rejected action must leave the expected revision unchanged", ErrInvalidFailure)
	}
	if err := validateExactText(failure.PublicMessage, "public failure message", MaxFailureMessageBytes); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFailure, err)
	}
	if err := validateEvidenceRefs(failure.EvidenceRefs); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFailure, err)
	}
	available := make(map[string]struct{}, len(action.EvidenceRefs))
	for _, ref := range action.EvidenceRefs {
		available[evidenceIdentity(ref)] = struct{}{}
	}
	for index, ref := range failure.EvidenceRefs {
		if ref.Revision.EpisodeID != expected.EpisodeID || ref.Revision.Number > expected.Number {
			return fmt.Errorf("%w: evidence %d is not available at the unchanged revision", ErrInvalidFailure, index)
		}
		if _, exists := available[evidenceIdentity(ref)]; !exists {
			return fmt.Errorf("%w: evidence %d was not supplied with the rejected action", ErrInvalidFailure, index)
		}
	}
	return nil
}

func (failure ActionFailure) Clone() ActionFailure {
	failure.EvidenceRefs = cloneSlice(failure.EvidenceRefs)
	return failure
}

func registeredFailureCode(code ActionFailureCode) bool {
	switch code {
	case ActionFailureInvalidAction, ActionFailurePreconditionFailed,
		ActionFailureUnauthorized, ActionFailureStaleRevision,
		ActionFailureIdempotencyConflict, ActionFailureTerminal,
		ActionFailureBudget:
		return true
	default:
		return false
	}
}
