package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func (store *Store) StartEnvironment(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	start cognition.Transition,
) (cognition.Transition, error) {
	if store == nil || store.repository == nil {
		return cognition.Transition{}, fmt.Errorf("cognition environment store is uninitialized")
	}
	return store.repository.StartCognitionEnvironment(ctx, episode, scenario, start)
}

func (store *Store) ReviewEnvironmentAction(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.EnvironmentReceipt, bool, error) {
	if store == nil || store.repository == nil {
		return cognition.EnvironmentReceipt{}, false, fmt.Errorf("cognition environment store is uninitialized")
	}
	return store.repository.ReviewCognitionEnvironmentAction(ctx, episode, scenario, expected, action)
}

func (store *Store) CommitEnvironmentAction(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	receipt cognition.EnvironmentReceipt,
) (cognition.EnvironmentReceipt, error) {
	if store == nil || store.repository == nil {
		return cognition.EnvironmentReceipt{}, fmt.Errorf("cognition environment store is uninitialized")
	}
	return store.repository.CommitCognitionEnvironmentAction(ctx, episode, scenario, receipt)
}

func (store *Store) EnvironmentState(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
) (cognition.EnvironmentJournalState, error) {
	if store == nil || store.repository == nil {
		return cognition.EnvironmentJournalState{}, fmt.Errorf("cognition environment store is uninitialized")
	}
	return store.repository.CognitionEnvironmentState(ctx, episode, scenario)
}
