package cognitionruntime

import (
	"fmt"
	"math"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

type ActionStatus string

const (
	ActionPrepared   ActionStatus = "prepared"
	ActionDispatched ActionStatus = "dispatched"
	ActionSucceeded  ActionStatus = "succeeded"
	ActionFailed     ActionStatus = "failed"
)

type ActionRecord struct {
	Episode           cognition.EpisodeRef           `json:"episode"`
	ExpectedRevision  cognition.WorldRevision        `json:"expected_revision"`
	SnapshotSHA256    string                         `json:"snapshot_sha256"`
	ContextProjection cognition.ContextProjectionRef `json:"context_projection"`
	Schema            cognition.ActionSchema         `json:"schema"`
	Decision          cognition.CognitionDecision    `json:"decision"`
	Action            cognition.RegisteredAction     `json:"action"`
	Status            ActionStatus                   `json:"status"`
	Failure           *cognition.ActionFailure       `json:"failure,omitempty"`
	ResultRevision    *cognition.WorldRevision       `json:"result_revision,omitempty"`
}

type PrepareActionCommand struct {
	Binding        Binding                      `json:"binding"`
	Coordinator    cognition.CoordinatorStep    `json:"coordinator"`
	Reconciliation ReconciliationReceipt        `json:"reconciliation"`
	Recovery       *AcceptedDecisionRecoveryRef `json:"recovery,omitempty"`
}

type ActionMutation struct {
	Binding          Binding                 `json:"binding"`
	ActionID         cognition.ActionID      `json:"action_id"`
	ExpectedRevision cognition.WorldRevision `json:"expected_revision"`
}

type FailureMutation struct {
	ActionMutation
	Failure cognition.ActionFailure `json:"failure"`
}

type TransitionMutation struct {
	ActionMutation
	Transition cognition.Transition `json:"transition"`
}

func (record ActionRecord) ValidateFor(binding Binding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	if record.Episode != binding.Episode || record.ExpectedRevision.EpisodeID != binding.Episode.ID {
		return fmt.Errorf("%w: action record belongs to another episode", ErrInvalidJournalState)
	}
	if !sameStep(record.Action.Actor, binding.Attempt) {
		return fmt.Errorf("%w: action record belongs to another step", ErrInvalidJournalState)
	}
	if !validSHA256(record.SnapshotSHA256) {
		return fmt.Errorf("%w: action snapshot hash is invalid", ErrInvalidJournalState)
	}
	if err := record.ContextProjection.Validate(); err != nil {
		return fmt.Errorf("%w: action projection: %v", ErrInvalidJournalState, err)
	}
	if err := record.ExpectedRevision.Validate(); err != nil {
		return fmt.Errorf("%w: expected revision: %v", ErrInvalidJournalState, err)
	}
	if err := record.Action.Validate(record.Schema); err != nil {
		return fmt.Errorf("%w: registered action: %v", ErrInvalidJournalState, err)
	}
	if err := record.Decision.Validate(record.Schema); err != nil {
		return fmt.Errorf("%w: decision: %v", ErrInvalidJournalState, err)
	}
	if record.Decision.ObligationID == "" || record.Action.Schema != record.Schema.Ref() ||
		!reflect.DeepEqual(record.Action.Request, record.Decision.Action) ||
		!reflect.DeepEqual(record.Action.EvidenceRefs, record.Decision.EvidenceRefs) {
		return fmt.Errorf("%w: registered action differs from its accepted decision", ErrInvalidJournalState)
	}
	return validateActionResolution(record)
}

func validateActionResolution(record ActionRecord) error {
	switch record.Status {
	case ActionPrepared, ActionDispatched:
		if record.Failure != nil || record.ResultRevision != nil {
			return fmt.Errorf("%w: unresolved action has terminal detail", ErrInvalidJournalState)
		}
	case ActionSucceeded:
		if record.Failure != nil || record.ResultRevision == nil ||
			record.ResultRevision.EpisodeID != record.Episode.ID ||
			record.ExpectedRevision.Number == math.MaxUint64 ||
			record.ResultRevision.Number != record.ExpectedRevision.Number+1 {
			return fmt.Errorf("%w: succeeded action has invalid result revision", ErrInvalidJournalState)
		}
		if err := record.ResultRevision.Validate(); err != nil {
			return fmt.Errorf("%w: result revision: %v", ErrInvalidJournalState, err)
		}
	case ActionFailed:
		if record.Failure == nil || record.ResultRevision != nil {
			return fmt.Errorf("%w: failed action has invalid terminal detail", ErrInvalidJournalState)
		}
		if err := record.Failure.Validate(record.Action, record.ExpectedRevision); err != nil {
			return fmt.Errorf("%w: failure: %v", ErrInvalidJournalState, err)
		}
	default:
		return fmt.Errorf("%w: action status %q is not registered", ErrInvalidJournalState, record.Status)
	}
	return nil
}

func (record ActionRecord) Clone() ActionRecord {
	record.Schema = record.Schema.Clone()
	record.Decision = record.Decision.Clone()
	record.Action = record.Action.Clone()
	if record.Failure != nil {
		failure := record.Failure.Clone()
		record.Failure = &failure
	}
	if record.ResultRevision != nil {
		revision := *record.ResultRevision
		record.ResultRevision = &revision
	}
	return record
}

func authorizeAction(record ActionRecord, binding Binding) (cognition.RegisteredAction, error) {
	if err := record.ValidateFor(binding); err != nil {
		return cognition.RegisteredAction{}, err
	}
	action := record.Action.Clone()
	action.Actor = binding.Attempt
	if err := action.Validate(record.Schema); err != nil {
		return cognition.RegisteredAction{}, fmt.Errorf("%w: replacement action: %v", ErrInvalidJournalState, err)
	}
	return action, nil
}

func validateActionMutation(command ActionMutation, record ActionRecord, status ActionStatus) error {
	if err := command.Binding.Validate(); err != nil {
		return err
	}
	if command.ActionID == "" || command.ExpectedRevision.EpisodeID != command.Binding.Episode.ID {
		return fmt.Errorf("%w: action mutation identity is incomplete", ErrInvalidJournalState)
	}
	if err := record.ValidateFor(command.Binding); err != nil {
		return err
	}
	if record.Action.ID != command.ActionID || record.ExpectedRevision != command.ExpectedRevision || record.Status != status {
		return fmt.Errorf("%w: action mutation returned a different record", ErrInvalidJournalState)
	}
	return nil
}
