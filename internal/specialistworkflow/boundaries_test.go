package specialistworkflow_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/specialistworkflow"
)

func TestAttemptBudgetRejectsInvalidBoundsAndNilUse(t *testing.T) {
	if _, err := specialistworkflow.NewAttemptBudget(0); !errors.Is(err, specialistworkflow.ErrInvalidAttemptBudget) {
		t.Fatalf("zero attempt budget error=%v", err)
	}
	if _, err := specialistworkflow.NewAttemptBudget(33); !errors.Is(err, specialistworkflow.ErrInvalidAttemptBudget) {
		t.Fatalf("oversized attempt budget error=%v", err)
	}
	if _, err := (&specialistworkflow.AttemptBudget{}).Claim(); !errors.Is(err, specialistworkflow.ErrInvalidAttemptBudget) {
		t.Fatalf("zero-value attempt budget error=%v", err)
	}
	contract := mustRunnerContract(t,
		func(context.Context, runnerConfig, runnerObservation) (bool, error) { return true, nil },
		func(context.Context, runnerConfig, runnerObservation) (runnerFailure, error) {
			return runnerFailure{}, nil
		},
	)
	if _, err := specialistworkflow.RunAttempt(
		context.Background(), nil, runnerState{}, contract,
	); !errors.Is(err, specialistworkflow.ErrInvalidAttemptBudget) {
		t.Fatalf("nil attempt budget error=%v", err)
	}
	if _, err := specialistworkflow.RunAttempt(
		nil, nil, runnerState{}, contract,
	); !errors.Is(err, specialistworkflow.ErrNilContext) {
		t.Fatalf("nil context error=%v", err)
	}
}

func TestContractRejectsMissingLifecycleStage(t *testing.T) {
	var derive specialistworkflow.DeriveConfig[runnerState, runnerConfig]
	_, err := specialistworkflow.NewContract(
		mustRegistration(t, "observe", "observer"),
		derive,
		func(runnerConfig) error { return nil },
		func(context.Context, runnerConfig) (runnerObservation, error) {
			return runnerObservation{}, nil
		},
		func(context.Context, runnerConfig, runnerObservation) (bool, error) { return true, nil },
		func(context.Context, runnerConfig, runnerObservation) (runnerFailure, error) {
			return runnerFailure{}, nil
		},
	)
	if !errors.Is(err, specialistworkflow.ErrInvalidContract) {
		t.Fatalf("missing lifecycle stage error=%v", err)
	}
}

func TestCancellationDuringVerificationPreventsFailureReduction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reduceCalls := 0
	contract := mustRunnerContract(t,
		func(context.Context, runnerConfig, runnerObservation) (bool, error) {
			cancel()
			return false, nil
		},
		func(context.Context, runnerConfig, runnerObservation) (runnerFailure, error) {
			reduceCalls++
			return runnerFailure{}, nil
		},
	)
	budget, _ := specialistworkflow.NewAttemptBudget(1)
	receipt, err := specialistworkflow.RunAttempt(ctx, budget, runnerState{}, contract)
	if !errors.Is(err, context.Canceled) || !receipt.Executed() || reduceCalls != 0 {
		t.Fatalf("receipt=%#v error=%v reduction_calls=%d", receipt, err, reduceCalls)
	}
	if _, ok, observationErr := receipt.Observation(); observationErr != nil || !ok {
		t.Fatal("cancellation during verification discarded the completed observation")
	}
}

func TestAttemptBudgetIsAHardConcurrentExecutionBound(t *testing.T) {
	const contenders = 8
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var executeCalls atomic.Int32
	contract, err := specialistworkflow.NewContract(
		mustRegistration(t, "observe", "concurrent-observer"),
		func(runnerState) (runnerConfig, error) { return runnerConfig{}, nil },
		func(runnerConfig) error { return nil },
		func(context.Context, runnerConfig) (runnerObservation, error) {
			executeCalls.Add(1)
			started <- struct{}{}
			<-release
			return runnerObservation{}, nil
		},
		func(context.Context, runnerConfig, runnerObservation) (bool, error) { return true, nil },
		func(context.Context, runnerConfig, runnerObservation) (runnerFailure, error) {
			return runnerFailure{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	budget, _ := specialistworkflow.NewAttemptBudget(2)
	results := make(chan error, contenders)
	var workers sync.WaitGroup
	for worker := 0; worker < contenders; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, runErr := specialistworkflow.RunAttempt(context.Background(), budget, runnerState{}, contract)
			results <- runErr
		}()
	}
	<-started
	<-started
	close(release)
	workers.Wait()
	close(results)

	succeeded, exhausted := 0, 0
	for runErr := range results {
		switch {
		case runErr == nil:
			succeeded++
		case errors.Is(runErr, specialistworkflow.ErrAttemptBudgetExhausted):
			exhausted++
		default:
			t.Fatalf("unexpected concurrent attempt error=%v", runErr)
		}
	}
	if succeeded != 2 || exhausted != contenders-2 || executeCalls.Load() != 2 || budget.Used() != 2 {
		t.Fatalf(
			"succeeded=%d exhausted=%d executions=%d used=%d",
			succeeded,
			exhausted,
			executeCalls.Load(),
			budget.Used(),
		)
	}
}

type countedBoundedValue struct {
	clones       *atomic.Int32
	oversize     bool
	panicOnClone bool
}

func (value countedBoundedValue) ValidateBounds() error {
	if value.oversize {
		return errors.New("value exceeds its typed hard bound")
	}
	return nil
}

func (value countedBoundedValue) Clone() countedBoundedValue {
	if value.panicOnClone {
		panic("oversized value reached Clone")
	}
	value.clones.Add(1)
	return value
}

func TestRunAttemptRejectsOversizedStateBeforeCloneOrBudgetConsumption(t *testing.T) {
	var clones atomic.Int32
	registration := mustRegistration(t, "bounded", "bounded-observer")
	contract, err := specialistworkflow.NewContract(
		registration,
		func(value countedBoundedValue) (countedBoundedValue, error) { return value, nil },
		func(countedBoundedValue) error { return nil },
		func(context.Context, countedBoundedValue) (countedBoundedValue, error) {
			return countedBoundedValue{clones: &clones}, nil
		},
		func(context.Context, countedBoundedValue, countedBoundedValue) (bool, error) { return true, nil },
		func(context.Context, countedBoundedValue, countedBoundedValue) (countedBoundedValue, error) {
			return countedBoundedValue{clones: &clones}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	budget, _ := specialistworkflow.NewAttemptBudget(1)
	_, err = specialistworkflow.RunAttempt(context.Background(), budget, countedBoundedValue{
		clones: &clones, oversize: true, panicOnClone: true,
	}, contract)
	if !errors.Is(err, specialistworkflow.ErrInvalidBoundedValue) {
		t.Fatalf("RunAttempt error=%v", err)
	}
	if got := clones.Load(); got != 0 {
		t.Fatalf("clone count=%d; oversized state was cloned", got)
	}
	if budget.Used() != 0 {
		t.Fatalf("used attempts=%d; oversized state consumed execution budget", budget.Used())
	}
}
