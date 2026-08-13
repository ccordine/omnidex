package specialistworkflow_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/specialistworkflow"
)

type runnerState struct{ values []string }
type runnerConfig struct{ values []string }
type runnerObservation struct {
	values []string
	clones *atomic.Int32
}
type runnerFailure struct{ values []string }

func (value runnerState) ValidateBounds() error       { return validateRunnerValues(value.values) }
func (value runnerConfig) ValidateBounds() error      { return validateRunnerValues(value.values) }
func (value runnerObservation) ValidateBounds() error { return validateRunnerValues(value.values) }
func (value runnerFailure) ValidateBounds() error     { return validateRunnerValues(value.values) }
func (value runnerState) Clone() runnerState          { return runnerState{values: cloneStrings(value.values)} }
func (value runnerConfig) Clone() runnerConfig {
	return runnerConfig{values: cloneStrings(value.values)}
}
func (value runnerObservation) Clone() runnerObservation {
	if value.clones != nil {
		value.clones.Add(1)
	}
	return runnerObservation{values: cloneStrings(value.values), clones: value.clones}
}
func (value runnerFailure) Clone() runnerFailure {
	return runnerFailure{values: cloneStrings(value.values)}
}

func TestRunAttemptOwnsValuesAndExecutesEachLifecycleStageOnce(t *testing.T) {
	registration := mustRegistration(t, "observe", "observer")
	deriveCalls, validateCalls, executeCalls, verifyCalls, reduceCalls := 0, 0, 0, 0, 0
	contract, err := specialistworkflow.NewContract(
		registration,
		func(state runnerState) (runnerConfig, error) {
			deriveCalls++
			state.values[0] = "derive mutation"
			return runnerConfig{values: []string{"exact config"}}, nil
		},
		func(config runnerConfig) error {
			validateCalls++
			config.values[0] = "validation mutation"
			return nil
		},
		func(_ context.Context, config runnerConfig) (runnerObservation, error) {
			executeCalls++
			if config.values[0] != "exact config" {
				t.Fatalf("executor received aliased config %q", config.values[0])
			}
			config.values[0] = "execution mutation"
			return runnerObservation{values: []string{"observed"}}, nil
		},
		func(_ context.Context, config runnerConfig, observation runnerObservation) (bool, error) {
			verifyCalls++
			config.values[0] = "verification config mutation"
			observation.values[0] = "verification observation mutation"
			return true, nil
		},
		func(_ context.Context, _ runnerConfig, _ runnerObservation) (runnerFailure, error) {
			reduceCalls++
			return runnerFailure{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := specialistworkflow.NewAttemptBudget(1)
	if err != nil {
		t.Fatal(err)
	}
	state := runnerState{values: []string{"authoritative"}}
	receipt, err := specialistworkflow.RunAttempt(context.Background(), budget, state, contract)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Executed() || !receipt.Verified() || receipt.Attempt() != 1 {
		t.Fatalf("unexpected receipt: executed=%t verified=%t attempt=%d", receipt.Executed(), receipt.Verified(), receipt.Attempt())
	}
	if deriveCalls != 1 || validateCalls != 1 || executeCalls != 1 || verifyCalls != 1 || reduceCalls != 0 {
		t.Fatalf("lifecycle counts=%d/%d/%d/%d/%d", deriveCalls, validateCalls, executeCalls, verifyCalls, reduceCalls)
	}
	if state.values[0] != "authoritative" {
		t.Fatalf("caller state mutated: %#v", state)
	}
	observation, ok, err := receipt.Observation()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || observation.values[0] != "observed" {
		t.Fatalf("observation=%#v present=%t", observation, ok)
	}
	observation.values[0] = "caller mutation"
	again, _, err := receipt.Observation()
	if err != nil {
		t.Fatal(err)
	}
	if again.values[0] != "observed" {
		t.Fatalf("receipt exposed observation alias: %#v", again)
	}
	if _, ok, err := receipt.Failure(); err != nil || ok {
		t.Fatal("verified receipt contains a failure")
	}
}

func TestRunAttemptReducesFailedAcceptanceExactlyOnce(t *testing.T) {
	reduceCalls := 0
	contract := mustRunnerContract(t,
		func(context.Context, runnerConfig, runnerObservation) (bool, error) { return false, nil },
		func(_ context.Context, _ runnerConfig, observation runnerObservation) (runnerFailure, error) {
			reduceCalls++
			return runnerFailure{values: []string{"reduced:" + observation.values[0]}}, nil
		},
	)
	budget, _ := specialistworkflow.NewAttemptBudget(1)
	receipt, err := specialistworkflow.RunAttempt(
		context.Background(), budget, runnerState{values: []string{"state"}}, contract,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Executed() || receipt.Verified() || reduceCalls != 1 {
		t.Fatalf("failed receipt executed=%t verified=%t reductions=%d", receipt.Executed(), receipt.Verified(), reduceCalls)
	}
	failure, ok, err := receipt.Failure()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || failure.values[0] != "reduced:observed" {
		t.Fatalf("failure=%#v present=%t", failure, ok)
	}
	failure.values[0] = "caller mutation"
	again, _, err := receipt.Failure()
	if err != nil {
		t.Fatal(err)
	}
	if again.values[0] != "reduced:observed" {
		t.Fatalf("receipt exposed failure alias: %#v", again)
	}
}

func TestRunAttemptNeverRetriesOrReducesOperationalErrors(t *testing.T) {
	want := errors.New("executor unavailable")
	executeCalls, verifyCalls, reduceCalls := 0, 0, 0
	contract, err := specialistworkflow.NewContract(
		mustRegistration(t, "observe", "observer"),
		func(runnerState) (runnerConfig, error) { return runnerConfig{values: []string{"config"}}, nil },
		func(runnerConfig) error { return nil },
		func(context.Context, runnerConfig) (runnerObservation, error) {
			executeCalls++
			return runnerObservation{}, want
		},
		func(context.Context, runnerConfig, runnerObservation) (bool, error) {
			verifyCalls++
			return true, nil
		},
		func(context.Context, runnerConfig, runnerObservation) (runnerFailure, error) {
			reduceCalls++
			return runnerFailure{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	budget, _ := specialistworkflow.NewAttemptBudget(3)
	receipt, err := specialistworkflow.RunAttempt(context.Background(), budget, runnerState{}, contract)
	if !errors.Is(err, want) || !receipt.Executed() || executeCalls != 1 || verifyCalls != 0 || reduceCalls != 0 {
		t.Fatalf("err=%v receipt=%#v calls=%d/%d/%d", err, receipt, executeCalls, verifyCalls, reduceCalls)
	}
}

func TestRunAttemptDoesNotConsumeBudgetForIncompleteConfiguration(t *testing.T) {
	want := errors.New("exact selector is unresolved")
	state := runnerState{values: []string{"unresolved"}}
	contract, err := specialistworkflow.NewContract(
		mustRegistration(t, "observe", "observer"),
		func(state runnerState) (runnerConfig, error) {
			if state.values[0] == "unresolved" {
				return runnerConfig{}, want
			}
			return runnerConfig{values: []string{"config"}}, nil
		},
		func(runnerConfig) error { return nil },
		func(context.Context, runnerConfig) (runnerObservation, error) {
			return runnerObservation{values: []string{"observed"}}, nil
		},
		func(context.Context, runnerConfig, runnerObservation) (bool, error) { return true, nil },
		func(context.Context, runnerConfig, runnerObservation) (runnerFailure, error) {
			return runnerFailure{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	budget, _ := specialistworkflow.NewAttemptBudget(1)
	if _, err := specialistworkflow.RunAttempt(context.Background(), budget, state, contract); !errors.Is(err, want) {
		t.Fatalf("configuration error=%v", err)
	}
	state.values[0] = "resolved"
	receipt, err := specialistworkflow.RunAttempt(context.Background(), budget, state, contract)
	if err != nil || receipt.Attempt() != 1 || !receipt.Verified() {
		t.Fatalf("resumed receipt=%#v error=%v", receipt, err)
	}
}

func mustRegistration(t *testing.T, capability, workflow string) specialistworkflow.Registration {
	t.Helper()
	registration, err := specialistworkflow.NewRegistration(
		specialistworkflow.CapabilityID(capability), specialistworkflow.WorkflowID(workflow), "1",
	)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func mustRunnerContract(
	t *testing.T,
	verify specialistworkflow.Verify[runnerConfig, runnerObservation],
	reduce specialistworkflow.ReduceFailure[runnerConfig, runnerObservation, runnerFailure],
) specialistworkflow.Contract[runnerState, runnerConfig, runnerObservation, runnerFailure] {
	t.Helper()
	contract, err := specialistworkflow.NewContract(
		mustRegistration(t, "observe", "observer"),
		func(runnerState) (runnerConfig, error) { return runnerConfig{values: []string{"config"}}, nil },
		func(runnerConfig) error { return nil },
		func(context.Context, runnerConfig) (runnerObservation, error) {
			return runnerObservation{values: []string{"observed"}}, nil
		},
		verify,
		reduce,
	)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }
