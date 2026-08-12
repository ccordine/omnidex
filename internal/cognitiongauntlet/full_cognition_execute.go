package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func executeFullCognition(
	ctx context.Context,
	request FullCognitionRunRequest,
	authority PairedRunAuthority,
	episode cognition.EpisodeRef,
	scenario labyrinth.Scenario,
	brain cognitionpolicy.BrainRef,
	binding cognitionruntime.Binding,
	components fullRuntimeComponents,
) (cognitionruntime.RunResult, []RestartTrace, error) {
	result := cognitionruntime.RunResult{}
	restarts := make([]RestartTrace, 0, len(request.RestartAfterCycles))
	restartIndex := 0
	for result.Cycles < uint32(authority.Budget.RuntimeCycles) {
		step, err := components.runtime.Step(ctx, binding)
		result.Cycles++
		if step.PolicyCalled {
			result.PolicyCalls++
		}
		result.EnvironmentActions += step.EnvironmentActions
		if err != nil {
			return result, restarts, fmt.Errorf("execute production cognition cycle %d: %w", result.Cycles, err)
		}
		if int(result.PolicyCalls) > authority.Budget.ModelCalls ||
			int(result.EnvironmentActions) > authority.Budget.EnvironmentActions ||
			int(result.EnvironmentActions) > authority.Budget.ToolOperations {
			return result, restarts, fmt.Errorf("production cognition execution exceeded its frozen budget")
		}
		if step.State == cognitionruntime.StepEpisodeCompleted ||
			step.State == cognitionruntime.StepEpisodeFailed {
			if restartIndex != len(request.RestartAfterCycles) {
				return result, restarts, fmt.Errorf("episode terminated before every registered restart was executed")
			}
			result.Terminal = step
			return result, restarts, nil
		}
		if restartIndex >= len(request.RestartAfterCycles) ||
			request.RestartAfterCycles[restartIndex] != result.Cycles {
			continue
		}
		before, err := captureRuntimeCheckpoint(ctx, components.repository, episode.ID)
		if err != nil {
			return result, restarts, err
		}
		replacement, err := newFullRuntimeComponents(
			ctx, request.Pool, request.Client, brain, authority.RatGeneration.Fixed.Brain,
			request.HostStore,
			scenario, episode, request.Attempt, request.Surface,
		)
		if err != nil {
			return result, restarts, err
		}
		restartTransition, err := replacement.environment.Start(ctx, scenario.Ref())
		if err != nil {
			return result, restarts, fmt.Errorf("restart durable Labyrinth host: %w", err)
		}
		activation, err := observeRuntimeProviderActivation(
			ctx, replacement, episode, binding.Attempt,
		)
		if err != nil {
			return result, restarts, err
		}
		stored, err := startFullCognitionEpisode(
			ctx, replacement.store, request, authority, replacement.brainBootstrap, activation,
			episode, scenario, restartTransition,
		)
		if err != nil {
			return result, restarts, err
		}
		replacement, err = activateRuntimeComponents(ctx, replacement, stored, activation)
		if err != nil {
			return result, restarts, err
		}
		after, err := captureRuntimeCheckpoint(ctx, replacement.repository, episode.ID)
		if err != nil {
			return result, restarts, err
		}
		receipt, err := NewRestartTrace(result.Cycles, before, after)
		if err != nil {
			return result, restarts, err
		}
		restarts = append(restarts, receipt)
		components = replacement
		restartIndex++
	}
	return result, restarts, fmt.Errorf(
		"%w: production cognition execution exhausted %d frozen runtime cycles",
		cognitionruntime.ErrRunCycleLimit, authority.Budget.RuntimeCycles,
	)
}
