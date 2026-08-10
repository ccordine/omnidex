package cognition

import (
	"context"
	"fmt"
	"reflect"
)

// EnvironmentReceipt is the exact durable result of one registered action.
// Exactly one of Transition or Failure is present.
type EnvironmentReceipt struct {
	Action     RegisteredAction `json:"action"`
	Expected   WorldRevision    `json:"expected"`
	Transition *Transition      `json:"transition,omitempty"`
	Failure    *ActionFailure   `json:"failure,omitempty"`
}

func NewEnvironmentTransitionReceipt(
	episode EpisodeRef,
	action RegisteredAction,
	expected WorldRevision,
	transition Transition,
) (EnvironmentReceipt, error) {
	receipt := EnvironmentReceipt{Action: action.Clone(), Expected: expected, Transition: &transition}
	if err := receipt.Validate(episode); err != nil {
		return EnvironmentReceipt{}, err
	}
	return receipt, nil
}

func NewEnvironmentFailureReceipt(
	episode EpisodeRef,
	action RegisteredAction,
	expected WorldRevision,
	failure ActionFailure,
) (EnvironmentReceipt, error) {
	receipt := EnvironmentReceipt{Action: action.Clone(), Expected: expected, Failure: &failure}
	if err := receipt.Validate(episode); err != nil {
		return EnvironmentReceipt{}, err
	}
	return receipt, nil
}

func (receipt EnvironmentReceipt) Validate(episode EpisodeRef) error {
	if err := episode.Validate(); err != nil {
		return err
	}
	if err := receipt.Expected.Validate(); err != nil || receipt.Expected.EpisodeID != episode.ID {
		return fmt.Errorf("%w: environment receipt has invalid expected revision", ErrInvalidTransition)
	}
	if (receipt.Transition == nil) == (receipt.Failure == nil) {
		return fmt.Errorf("%w: environment receipt requires exactly one result", ErrInvalidTransition)
	}
	if receipt.Transition != nil {
		return receipt.Transition.ValidateApply(episode, receipt.Expected, receipt.Action)
	}
	return receipt.Failure.Validate(receipt.Action, receipt.Expected)
}

func (receipt EnvironmentReceipt) Clone() EnvironmentReceipt {
	receipt.Action = receipt.Action.Clone()
	if receipt.Transition != nil {
		clone := receipt.Transition.Clone()
		receipt.Transition = &clone
	}
	if receipt.Failure != nil {
		clone := receipt.Failure.Clone()
		receipt.Failure = &clone
	}
	return receipt
}

type EnvironmentJournalState struct {
	Episode         EpisodeRef          `json:"episode"`
	Scenario        ScenarioRef         `json:"scenario"`
	Start           Transition          `json:"start"`
	Current         WorldRevision       `json:"current"`
	CurrentReceipt  *EnvironmentReceipt `json:"current_receipt,omitempty"`
	Terminal        bool                `json:"terminal"`
	TerminalReceipt *EnvironmentReceipt `json:"terminal_receipt,omitempty"`
}

func (state EnvironmentJournalState) Validate() error {
	if err := state.Episode.Validate(); err != nil {
		return err
	}
	if err := state.Scenario.Validate(); err != nil {
		return err
	}
	if err := state.Start.ValidateStart(); err != nil || state.Start.Current.EpisodeID != state.Episode.ID {
		return fmt.Errorf("%w: environment journal start is invalid", ErrInvalidTransition)
	}
	if err := state.Current.Validate(); err != nil || state.Current.EpisodeID != state.Episode.ID {
		return fmt.Errorf("%w: environment journal current revision is invalid", ErrInvalidRevision)
	}
	if state.Start.Terminal {
		if !state.Terminal || state.Current != state.Start.Current ||
			state.CurrentReceipt != nil || state.TerminalReceipt != nil {
			return fmt.Errorf("%w: terminal start journal state is inconsistent", ErrInvalidTransition)
		}
		return nil
	}
	if !state.Terminal {
		if state.TerminalReceipt != nil || state.Current.Number < state.Start.Current.Number ||
			(state.Current.Number == state.Start.Current.Number && state.Current != state.Start.Current) {
			return fmt.Errorf("%w: active environment journal revision is invalid", ErrInvalidTransition)
		}
		if state.Current == state.Start.Current {
			if state.CurrentReceipt != nil {
				return fmt.Errorf("%w: initial environment state has an action receipt", ErrInvalidTransition)
			}
			return nil
		}
		if state.CurrentReceipt == nil || state.CurrentReceipt.Transition == nil ||
			state.CurrentReceipt.Transition.Terminal ||
			state.CurrentReceipt.Transition.Current != state.Current {
			return fmt.Errorf("%w: active environment progress receipt is invalid", ErrInvalidTransition)
		}
		return state.CurrentReceipt.Validate(state.Episode)
	}
	if state.TerminalReceipt == nil || state.TerminalReceipt.Transition == nil ||
		!state.TerminalReceipt.Transition.Terminal || state.TerminalReceipt.Transition.Current != state.Current {
		return fmt.Errorf("%w: terminal environment journal receipt is invalid", ErrInvalidTransition)
	}
	if state.CurrentReceipt == nil || !reflect.DeepEqual(state.CurrentReceipt, state.TerminalReceipt) {
		return fmt.Errorf("%w: terminal current receipt differs from terminal authority", ErrInvalidTransition)
	}
	return state.TerminalReceipt.Validate(state.Episode)
}

func (state EnvironmentJournalState) Clone() EnvironmentJournalState {
	state.Start = state.Start.Clone()
	if state.CurrentReceipt != nil {
		clone := state.CurrentReceipt.Clone()
		state.CurrentReceipt = &clone
	}
	if state.TerminalReceipt != nil {
		clone := state.TerminalReceipt.Clone()
		state.TerminalReceipt = &clone
	}
	return state
}

// EnvironmentJournal owns idempotency, revision fencing, and terminal state.
// Review is an optimization only; Commit must atomically repeat every check.
type EnvironmentJournal interface {
	StartEnvironment(context.Context, EpisodeRef, ScenarioRef, Transition) (Transition, error)
	ReviewEnvironmentAction(context.Context, EpisodeRef, ScenarioRef, WorldRevision, RegisteredAction) (EnvironmentReceipt, bool, error)
	CommitEnvironmentAction(context.Context, EpisodeRef, ScenarioRef, EnvironmentReceipt) (EnvironmentReceipt, error)
	EnvironmentState(context.Context, EpisodeRef, ScenarioRef) (EnvironmentJournalState, error)
}
