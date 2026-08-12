package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func startPublicCognitionEpisode(
	ctx context.Context,
	store *cognitionstore.Store,
	attempt model.StepAttemptAuthority,
	bundle PublicInferenceBundle,
	bootstrap cognitionpolicy.BrainBootstrap,
	activation cognitionpolicy.ProviderProcessActivation,
	episode cognition.EpisodeRef,
	start cognition.Transition,
) (queue.CognitionEpisode, error) {
	budget, err := bundle.Authority.Budget.RuntimeBudget()
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	check, err := bundle.Completion.Resolve(bundle.Goal)
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	rootID, err := cognition.DeriveObligationID(
		episode.ID, cognition.InitialObligationGeneration, "", bundle.Goal, check,
	)
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	if err := cognitionpolicy.ValidateRuntimeBudget(bootstrap.AttestedBrain.Ref, budget); err != nil {
		return queue.CognitionEpisode{}, fmt.Errorf("validate public cognition runtime budget: %w", err)
	}
	stored, err := store.StartEpisode(ctx, queue.CognitionEpisodeStart{
		Authority: attempt, EpisodeID: episode.ID, Scenario: bundle.Authority.Scenario,
		BrainBootstrap: bootstrap, ProviderProcessActivation: activation,
		Goal: bundle.Goal, Completion: bundle.Completion, ActionCatalog: bundle.Catalog, Budget: budget,
		Root: cognition.ObligationSpec{
			ID: rootID, Desired: bundle.Goal, DependsOn: []cognition.ObligationID{},
			SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
		},
		Transition: start,
	})
	if err != nil {
		return queue.CognitionEpisode{}, fmt.Errorf("start public production cognition episode: %w", err)
	}
	return stored, nil
}
