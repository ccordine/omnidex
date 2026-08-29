package specialistworkflow_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/specialistworkflow"
)

func TestRunAttemptHonorsBudgetAndCancellationBoundaries(t *testing.T) {
	contract := mustRunnerContract(t,
		func(context.Context, runnerConfig, runnerObservation) (bool, error) { return true, nil },
		func(context.Context, runnerConfig, runnerObservation) (runnerFailure, error) {
			return runnerFailure{}, nil
		},
	)
	budget, _ := specialistworkflow.NewAttemptBudget(1)
	if _, err := specialistworkflow.RunAttempt(context.Background(), budget, runnerState{}, contract); err != nil {
		t.Fatal(err)
	}
	if _, err := specialistworkflow.RunAttempt(context.Background(), budget, runnerState{}, contract); !errors.Is(err, specialistworkflow.ErrAttemptBudgetExhausted) {
		t.Fatalf("exhausted budget error=%v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	fresh, _ := specialistworkflow.NewAttemptBudget(1)
	if _, err := specialistworkflow.RunAttempt(canceled, fresh, runnerState{}, contract); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled error=%v", err)
	}

	verifyCalls := 0
	var observationClones atomic.Int32
	var cancelDuringExecute context.CancelFunc
	executionContract, err := specialistworkflow.NewContract(
		mustRegistration(t, "observe", "cancel-observer"),
		func(runnerState) (runnerConfig, error) { return runnerConfig{}, nil },
		func(runnerConfig) error { return nil },
		func(context.Context, runnerConfig) (runnerObservation, error) {
			cancelDuringExecute()
			return runnerObservation{values: []string{"discarded observation"}, clones: &observationClones}, nil
		},
		func(context.Context, runnerConfig, runnerObservation) (bool, error) {
			verifyCalls++
			return true, nil
		},
		func(context.Context, runnerConfig, runnerObservation) (runnerFailure, error) {
			return runnerFailure{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelExecution := context.WithCancel(context.Background())
	cancelDuringExecute = cancelExecution
	executionBudget, _ := specialistworkflow.NewAttemptBudget(1)
	receipt, err := specialistworkflow.RunAttempt(ctx, executionBudget, runnerState{}, executionContract)
	if !errors.Is(err, context.Canceled) || !receipt.Executed() || verifyCalls != 0 || observationClones.Load() != 0 {
		t.Fatalf(
			"canceled execution receipt=%#v error=%v verify_calls=%d observation_clones=%d",
			receipt, err, verifyCalls, observationClones.Load(),
		)
	}
	_, ok, observationErr := receipt.Observation()
	if observationErr != nil {
		t.Fatal(observationErr)
	}
	if ok {
		t.Fatal("canceled execution retained an unvalidated observation")
	}
}
