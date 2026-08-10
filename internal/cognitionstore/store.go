package cognitionstore

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/queue"
)

// Store adapts the production PostgreSQL queue authority to the domain-neutral
// cognition runtime. It deliberately does not implement CompletionEvaluator;
// that authority must be injected by the environment host.
type Store struct {
	repository *queue.Repository
	facts      cognitionstate.FactAcceptanceAuthority
}

func New(
	repository *queue.Repository,
	facts cognitionstate.FactAcceptanceAuthority,
) (*Store, error) {
	if repository == nil {
		return nil, fmt.Errorf("cognition store requires a PostgreSQL repository")
	}
	if err := facts.Validate(); err != nil {
		return nil, fmt.Errorf("cognition store fact authority: %w", err)
	}
	return &Store{repository: repository, facts: facts}, nil
}

func (store *Store) requireFactAuthority(
	ctx context.Context,
	episode cognition.EpisodeRef,
) error {
	if store == nil || store.repository == nil {
		return fmt.Errorf("cognition store is uninitialized")
	}
	persisted, err := store.repository.CognitionEpisode(ctx, episode.ID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(persisted.FactAuthority, store.facts.Reference()) {
		return fmt.Errorf("cognition fact authority changed for episode %q", episode.ID)
	}
	return nil
}

var (
	_ cognitionruntime.SnapshotPreparer        = (*Store)(nil)
	_ cognitionruntime.AcceptedDecisionJournal = (*Store)(nil)
	_ cognitionruntime.PolicyRecoveryJournal   = (*Store)(nil)
	_ cognitionruntime.DecisionReconciler      = (*Store)(nil)
	_ cognitionruntime.ActionJournal           = (*Store)(nil)
	_ cognitionruntime.EpisodeJournal          = (*Store)(nil)
	_ cognitionruntime.TerminalSealer          = (*Store)(nil)
	_ cognitionpolicy.ProjectionLoader         = (*Store)(nil)
	_ cognitionpolicy.CallJournal              = (*Store)(nil)
	_ cognition.EnvironmentJournal             = (*Store)(nil)
)
