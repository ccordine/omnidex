package cognitionenv

import (
	"context"
	"reflect"
	"sync"

	"github.com/gryph/omnidex/internal/cognition"
)

type memoryJournal struct {
	mu       sync.Mutex
	state    *cognition.EnvironmentJournalState
	receipts map[cognition.ActionID]cognition.EnvironmentReceipt
}

func (journal *memoryJournal) StartEnvironment(
	_ context.Context, episode cognition.EpisodeRef, scenario cognition.ScenarioRef,
	start cognition.Transition,
) (cognition.Transition, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.state != nil {
		if journal.state.Episode != episode || journal.state.Scenario != scenario ||
			!reflect.DeepEqual(journal.state.Start, start) {
			return cognition.Transition{}, cognition.ErrEnvironmentJournalConflict
		}
		return journal.state.Start.Clone(), nil
	}
	journal.state = &cognition.EnvironmentJournalState{
		Episode: episode, Scenario: scenario, Start: start.Clone(), Current: start.Current,
	}
	return start.Clone(), nil
}

func (journal *memoryJournal) ReviewEnvironmentAction(
	_ context.Context, episode cognition.EpisodeRef, scenario cognition.ScenarioRef,
	expected cognition.WorldRevision, action cognition.RegisteredAction,
) (cognition.EnvironmentReceipt, bool, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.review(episode, scenario, expected, action)
}

func (journal *memoryJournal) CommitEnvironmentAction(
	_ context.Context, episode cognition.EpisodeRef, scenario cognition.ScenarioRef,
	candidate cognition.EnvironmentReceipt,
) (cognition.EnvironmentReceipt, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if receipt, replay, err := journal.review(
		episode, scenario, candidate.Expected, candidate.Action,
	); err != nil {
		return cognition.EnvironmentReceipt{}, err
	} else if replay {
		return receipt, nil
	}
	if err := candidate.Validate(episode); err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	stored := candidate.Clone()
	journal.receipts[candidate.Action.ID] = stored
	if candidate.Transition != nil {
		journal.state.Current = candidate.Transition.Current
		journal.state.CurrentReceipt = receiptPointer(stored)
		journal.state.Terminal = candidate.Transition.Terminal
		if candidate.Transition.Terminal {
			journal.state.TerminalReceipt = receiptPointer(stored)
		}
	}
	return stored.Clone(), nil
}

func (journal *memoryJournal) EnvironmentState(
	_ context.Context, episode cognition.EpisodeRef, scenario cognition.ScenarioRef,
) (cognition.EnvironmentJournalState, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.state == nil || journal.state.Episode != episode || journal.state.Scenario != scenario {
		return cognition.EnvironmentJournalState{}, cognition.ErrEnvironmentJournalNotStarted
	}
	return cloneJournalState(*journal.state), nil
}

func (journal *memoryJournal) review(
	episode cognition.EpisodeRef, scenario cognition.ScenarioRef,
	expected cognition.WorldRevision, action cognition.RegisteredAction,
) (cognition.EnvironmentReceipt, bool, error) {
	if journal.state == nil || journal.state.Episode != episode || journal.state.Scenario != scenario {
		return cognition.EnvironmentReceipt{}, false, cognition.ErrEnvironmentJournalNotStarted
	}
	if receipt, exists := journal.receipts[action.ID]; exists {
		if receipt.Expected != expected || !reflect.DeepEqual(receipt.Action, action) {
			return cognition.EnvironmentReceipt{}, false, cognition.ErrEnvironmentJournalConflict
		}
		return receipt.Clone(), true, nil
	}
	if journal.state.Terminal {
		return cognition.EnvironmentReceipt{}, false, cognition.ErrEnvironmentJournalTerminal
	}
	if journal.state.Current != expected {
		return cognition.EnvironmentReceipt{}, false, cognition.ErrEnvironmentJournalStaleRevision
	}
	return cognition.EnvironmentReceipt{}, false, nil
}

func cloneJournalState(state cognition.EnvironmentJournalState) cognition.EnvironmentJournalState {
	return state.Clone()
}

func receiptPointer(receipt cognition.EnvironmentReceipt) *cognition.EnvironmentReceipt {
	return &receipt
}
