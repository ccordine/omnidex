package cognitionruntime

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

type Runtime struct {
	coordinator    *cognition.Coordinator
	environment    cognition.Environment
	snapshots      SnapshotPreparer
	accepted       AcceptedDecisionJournal
	policyRecovery PolicyRecoveryJournal
	completion     CompletionEvaluator
	episodes       EpisodeJournal
	reconciler     DecisionReconciler
	actions        ActionJournal
	sealer         TerminalSealer
}

const MaxRunCycles uint32 = 1_000_000

func New(dependencies Dependencies) (*Runtime, error) {
	required := []struct {
		name  string
		value any
	}{
		{"policy", dependencies.Policy}, {"environment", dependencies.Environment},
		{"snapshot preparer", dependencies.Snapshots}, {"accepted decision journal", dependencies.Accepted},
		{"policy recovery journal", dependencies.PolicyRecovery},
		{"completion evaluator", dependencies.Completion},
		{"episode journal", dependencies.Episodes}, {"decision reconciler", dependencies.Reconciler},
		{"action journal", dependencies.Actions}, {"terminal sealer", dependencies.TerminalSeal},
	}
	for _, dependency := range required {
		if nilDependency(dependency.value) {
			return nil, fmt.Errorf("%w: %s is nil", ErrInvalidConfiguration, dependency.name)
		}
	}
	coordinator, err := cognition.NewCoordinator(dependencies.Policy)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfiguration, err)
	}
	return &Runtime{
		coordinator: coordinator, environment: dependencies.Environment,
		snapshots: dependencies.Snapshots, accepted: dependencies.Accepted,
		policyRecovery: dependencies.PolicyRecovery, completion: dependencies.Completion,
		episodes: dependencies.Episodes, reconciler: dependencies.Reconciler,
		actions: dependencies.Actions, sealer: dependencies.TerminalSeal,
	}, nil
}

func (runtime *Runtime) Step(ctx context.Context, binding Binding) (StepResult, error) {
	if err := runtime.validateCall(ctx, binding); err != nil {
		return StepResult{}, err
	}
	if result, found, err := runtime.recover(ctx, binding); err != nil || found {
		return result, err
	}
	abandonment, err := runtime.policyRecovery.AbandonIndeterminate(ctx, binding)
	if err != nil {
		return StepResult{Binding: binding}, fmt.Errorf("abandon indeterminate cognition policy call: %w", err)
	}
	if abandonment != nil {
		if err := abandonment.ValidateFor(binding); err != nil {
			return StepResult{Binding: binding}, err
		}
	}
	result, err := runtime.stepPrepared(ctx, binding)
	if abandonment != nil {
		ref := abandonment.Ref()
		result.PolicyCallAbandonment = &ref
		result.AbandonedPolicyCalls = 1
	}
	return result, err
}

func (runtime *Runtime) stepPrepared(ctx context.Context, binding Binding) (StepResult, error) {
	prepared, err := runtime.snapshots.PrepareSnapshot(ctx, binding)
	if err != nil {
		return StepResult{}, fmt.Errorf("prepare cognition snapshot: %w", err)
	}
	if err := prepared.ValidateFor(binding); err != nil {
		return StepResult{}, err
	}
	prepared = prepared.clone()
	request := completionRequest(prepared, binding)
	completion, err := runtime.completion.Evaluate(ctx, request)
	if err != nil {
		return StepResult{}, fmt.Errorf("evaluate cognition completion: %w", err)
	}
	if err := validateCompletionResult(prepared, completion); err != nil {
		return StepResult{}, err
	}
	completion = completion.Clone()
	if completion.Outcome == cognition.CompletionSatisfied {
		step, err := runtime.coordinator.Step(
			ctx, prepared.Snapshot, completion, prepared.CompletionEvidenceRefs,
		)
		if err != nil {
			return StepResult{}, err
		}
		if err := validateCoordinatorSatisfied(prepared, binding, completion, step); err != nil {
			return StepResult{}, err
		}
		return runtime.advanceSatisfied(ctx, binding, prepared, completion)
	}
	if prepared.EnvironmentTerminal {
		return runtime.failTerminal(ctx, binding, prepared, completion)
	}
	return runtime.decideAndExecute(ctx, binding, prepared, completion)
}

func (runtime *Runtime) Run(
	ctx context.Context,
	binding Binding,
	limits RunLimits,
) (RunResult, error) {
	if limits.MaxCycles == 0 || limits.MaxCycles > MaxRunCycles {
		return RunResult{}, fmt.Errorf(
			"%w: max cycles must be between 1 and %d", ErrInvalidConfiguration, MaxRunCycles,
		)
	}
	if err := runtime.validateCall(ctx, binding); err != nil {
		return RunResult{}, err
	}
	result := RunResult{}
	seenAbandonments := make(map[string]struct{})
	for result.Cycles < limits.MaxCycles {
		step, err := runtime.Step(ctx, binding)
		result.Cycles++
		if step.PolicyCalled {
			result.PolicyCalls++
		}
		if step.RecoveredDecision {
			result.RecoveredDecisions++
		}
		if step.RecoveredAction {
			result.RecoveredActions++
		}
		if step.RecoveredProgress {
			result.RecoveredProgress++
		}
		if step.RecoveredPolicyOutcome {
			result.RecoveredPolicyOutcomes++
		}
		if step.PolicyCallAbandonment == nil {
			if step.AbandonedPolicyCalls != 0 {
				return result, fmt.Errorf("%w: abandonment count has no exact reference", ErrInvalidJournalState)
			}
		} else {
			if step.AbandonedPolicyCalls != 1 || step.PolicyCallAbandonment.Validate() != nil {
				return result, fmt.Errorf("%w: abandonment result is invalid", ErrInvalidJournalState)
			}
			if _, seen := seenAbandonments[step.PolicyCallAbandonment.ID]; !seen {
				seenAbandonments[step.PolicyCallAbandonment.ID] = struct{}{}
				result.AbandonedPolicyCalls++
			}
		}
		result.EnvironmentActions += step.EnvironmentActions
		if err != nil {
			return result, err
		}
		if step.State == StepEpisodeCompleted || step.State == StepEpisodeFailed || step.State == StepEpisodeCanceled {
			result.Terminal = step
			return result, nil
		}
	}
	return result, fmt.Errorf("%w: exhausted %d cycles", ErrRunCycleLimit, limits.MaxCycles)
}

func (runtime *Runtime) validateCall(ctx context.Context, binding Binding) error {
	if runtime == nil || runtime.coordinator == nil || nilDependency(runtime.environment) ||
		nilDependency(runtime.snapshots) || nilDependency(runtime.completion) ||
		nilDependency(runtime.accepted) || nilDependency(runtime.policyRecovery) ||
		nilDependency(runtime.episodes) || nilDependency(runtime.reconciler) ||
		nilDependency(runtime.actions) || nilDependency(runtime.sealer) {
		return fmt.Errorf("%w: runtime is uninitialized", ErrInvalidConfiguration)
	}
	if ctx == nil {
		return fmt.Errorf("%w: context is nil", ErrInvalidConfiguration)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return binding.Validate()
}

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
