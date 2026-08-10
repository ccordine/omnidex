package cognitionruntime

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func (runtime *Runtime) advanceSatisfied(
	ctx context.Context,
	binding Binding,
	prepared PreparedSnapshot,
	completion cognition.CompletionResult,
) (StepResult, error) {
	progress, err := runtime.episodes.AdvanceSatisfied(
		ctx, completionCommand(prepared, binding, completion),
	)
	if err != nil {
		return StepResult{}, fmt.Errorf("advance satisfied cognition obligation: %w", err)
	}
	if err := validateProgress(prepared, completion, progress); err != nil {
		return StepResult{}, err
	}
	if progress.State == ProgressActive {
		copy := completion.Clone()
		return StepResult{
			State: StepObligationAdvanced, Binding: binding, Revision: progress.Revision,
			Completion: &copy,
		}, nil
	}
	return runtime.sealProgress(ctx, binding, progress)
}

func (runtime *Runtime) failTerminal(
	ctx context.Context,
	binding Binding,
	prepared PreparedSnapshot,
	completion cognition.CompletionResult,
) (StepResult, error) {
	progress, err := runtime.episodes.FailTerminal(
		ctx, completionCommand(prepared, binding, completion),
	)
	if err != nil {
		return StepResult{}, fmt.Errorf("fail terminal cognition episode: %w", err)
	}
	if err := validateProgress(prepared, completion, progress); err != nil {
		return StepResult{}, err
	}
	if progress.State != ProgressFailed {
		return StepResult{}, fmt.Errorf("%w: terminal failure remained nonterminal", ErrInvalidProgress)
	}
	return runtime.sealProgress(ctx, binding, progress)
}

func (runtime *Runtime) sealProgress(
	ctx context.Context,
	binding Binding,
	progress EpisodeProgress,
) (StepResult, error) {
	command, err := sealCommand(binding, progress)
	if err != nil {
		return StepResult{}, err
	}
	seal, err := runtime.sealer.Seal(ctx, command.Clone())
	if err != nil {
		return StepResult{}, fmt.Errorf("seal cognition episode: %w", err)
	}
	if err := seal.ValidateFor(command); err != nil {
		return StepResult{}, err
	}
	state := StepEpisodeFailed
	if command.Outcome == TerminalCompleted {
		state = StepEpisodeCompleted
	}
	completion := command.Completion.Clone()
	return StepResult{
		State: state, Binding: binding, Revision: progress.Revision,
		Completion: &completion, Seal: &seal,
	}, nil
}
