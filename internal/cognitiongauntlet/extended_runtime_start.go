package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/queue"
)

func startExtendedRuntimeEpisode(
	ctx context.Context,
	store *cognitionstore.Store,
	request ExtendedRuntimeRunRequest,
	authority PairedRunAuthority,
	brain cognitionpolicy.AttestedBrain,
	episode cognition.EpisodeRef,
	scenario labyrinth.Scenario,
	start cognition.Transition,
) (queue.CognitionEpisode, error) {
	budget, err := authority.Budget.RuntimeBudget()
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	goal := scenario.Goal()
	completion, err := labyrinth.NewCompletionAuthority(scenario)
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	check, err := completion.Resolve(goal)
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	rootID, err := cognition.DeriveObligationID(
		episode.ID, cognition.InitialObligationGeneration, "", goal, check,
	)
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	if err := cognitionpolicy.ValidateRuntimeBudget(brain.Ref, budget); err != nil {
		return queue.CognitionEpisode{}, fmt.Errorf("validate extended runtime budget: %w", err)
	}
	stored, err := store.StartEpisode(ctx, queue.CognitionEpisodeStart{
		Authority: request.Attempt, EpisodeID: episode.ID, Scenario: scenario.Ref(),
		AttestedBrain: brain, Goal: goal, Completion: completion,
		ActionCatalog: scenario.Catalog(), Budget: budget,
		Root: cognition.ObligationSpec{
			ID: rootID, Desired: goal, DependsOn: []cognition.ObligationID{},
			SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
		},
		Transition: start,
	})
	if err != nil {
		return queue.CognitionEpisode{}, fmt.Errorf("start extended production cognition episode: %w", err)
	}
	return stored, nil
}
