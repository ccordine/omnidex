package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

type ablationGoalEvaluator interface {
	EvaluateGoal(
		context.Context,
		cognition.EpisodeRef,
		cognition.WorldRevision,
		cognition.GoalExpression,
	) (bool, error)
}

type ablationCompletionAuthority interface {
	Satisfied(context.Context, *ablationState, cognition.Transition) (bool, error)
}

type localAblationCompletion struct {
	evaluator ablationGoalEvaluator
}

func (authority localAblationCompletion) Satisfied(
	ctx context.Context,
	state *ablationState,
	transition cognition.Transition,
) (bool, error) {
	if authority.evaluator == nil {
		return false, fmt.Errorf("local ablation completion evaluator is unavailable")
	}
	return authority.evaluator.EvaluateGoal(
		ctx, state.episode, transition.Current, state.goal,
	)
}

type runtimeAblationCompletion struct {
	evaluator cognitionruntime.CompletionEvaluator
}

func (authority runtimeAblationCompletion) Satisfied(
	ctx context.Context,
	state *ablationState,
	transition cognition.Transition,
) (bool, error) {
	if authority.evaluator == nil {
		return false, fmt.Errorf("remote ablation completion evaluator is unavailable")
	}
	binding, err := cognitionruntime.NewBinding(state.episode, state.actor)
	if err != nil {
		return false, err
	}
	evidence := make([]cognition.EvidenceRef, len(transition.Observations))
	for index, observation := range transition.Observations {
		evidence[index] = observation.EvidenceRef()
	}
	snapshotSHA, err := digestJSON(struct {
		Episode    cognition.EpisodeRef    `json:"episode"`
		Revision   cognition.WorldRevision `json:"revision"`
		Obligation cognition.Obligation    `json:"obligation"`
		Evidence   []cognition.EvidenceRef `json:"evidence"`
	}{state.episode, transition.Current, state.obligation.Clone(), evidence})
	if err != nil {
		return false, err
	}
	result, err := authority.evaluator.Evaluate(ctx, cognitionruntime.CompletionRequest{
		Binding: binding, SnapshotSHA256: snapshotSHA,
		Goal: state.goal.Clone(), Revision: transition.Current,
		Obligation: state.obligation.Clone(), EvidenceRefs: evidence,
		EnvironmentTerminal: transition.Terminal, PublicOutcome: transition.PublicOutcome,
	})
	if err != nil {
		return false, err
	}
	if err := result.ValidateFor(state.obligation, transition.Current, evidence); err != nil {
		return false, err
	}
	return result.Outcome == cognition.CompletionSatisfied, nil
}
