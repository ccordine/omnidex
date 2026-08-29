package specialistworkflow_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/specialistworkflow"
)

type compilerState struct {
	command      string
	shouldAccept bool
}

type compilerConfig struct{ command string }
type compilerObservation struct {
	artifactRef string
	exitCode    int
	diagnostic  string
}
type compilerFailure struct {
	kind       string
	diagnostic string
}

func (value compilerState) ValidateBounds() error {
	if len(value.command) > 256 {
		return fmt.Errorf("compiler state exceeds fixture bounds")
	}
	return nil
}
func (value compilerConfig) ValidateBounds() error {
	if len(value.command) > 256 {
		return fmt.Errorf("compiler config exceeds fixture bounds")
	}
	return nil
}
func (value compilerObservation) ValidateBounds() error {
	if len(value.artifactRef)+len(value.diagnostic) > 512 {
		return fmt.Errorf("compiler observation exceeds fixture bounds")
	}
	return nil
}
func (value compilerFailure) ValidateBounds() error {
	if len(value.kind)+len(value.diagnostic) > 512 {
		return fmt.Errorf("compiler failure exceeds fixture bounds")
	}
	return nil
}
func (value compilerState) Clone() compilerState             { return value }
func (value compilerConfig) Clone() compilerConfig           { return value }
func (value compilerObservation) Clone() compilerObservation { return value }
func (value compilerFailure) Clone() compilerFailure         { return value }

func TestCompilerFixtureUsesSameLifecycleWithoutBrowserOrInference(t *testing.T) {
	contract := compilerContract(t)
	budget, _ := specialistworkflow.NewAttemptBudget(1)
	receipt, err := specialistworkflow.RunAttempt(context.Background(), budget, compilerState{
		command: "compile fixture", shouldAccept: true,
	}, contract)
	if err != nil {
		t.Fatal(err)
	}
	observation, ok, observationErr := receipt.Observation()
	if observationErr != nil {
		t.Fatal(observationErr)
	}
	if !receipt.Verified() || !ok || observation.artifactRef != "compiler-run:accepted" {
		t.Fatalf("receipt verified=%t observation=%#v present=%t", receipt.Verified(), observation, ok)
	}
	if receipt.Registration().Capability() != "program.compilation" ||
		receipt.Registration().Workflow() != "compiler.observer" {
		t.Fatalf("wrong workflow registration: %#v", receipt.Registration())
	}
}

func TestCompilerFixtureReducesExactDiagnosticWithoutRetry(t *testing.T) {
	contract := compilerContract(t)
	budget, _ := specialistworkflow.NewAttemptBudget(1)
	receipt, err := specialistworkflow.RunAttempt(context.Background(), budget, compilerState{
		command: "compile fixture", shouldAccept: false,
	}, contract)
	if err != nil {
		t.Fatal(err)
	}
	failure, ok, failureErr := receipt.Failure()
	if failureErr != nil {
		t.Fatal(failureErr)
	}
	if receipt.Verified() || !ok || failure.kind != "compiler_diagnostic" ||
		failure.diagnostic != "undefined symbol" {
		t.Fatalf("receipt verified=%t failure=%#v present=%t", receipt.Verified(), failure, ok)
	}
	if _, err := specialistworkflow.RunAttempt(context.Background(), budget, compilerState{}, contract); err == nil {
		t.Fatal("compiler workflow silently retried after its bounded attempt")
	}
}

func compilerContract(t *testing.T) specialistworkflow.Contract[
	compilerState, compilerConfig, compilerObservation, compilerFailure,
] {
	t.Helper()
	contract, err := specialistworkflow.NewContract(
		mustRegistration(t, "program.compilation", "compiler.observer"),
		func(state compilerState) (compilerConfig, error) {
			command := state.command
			if state.shouldAccept {
				command += " --accept"
			}
			return compilerConfig{command: command}, nil
		},
		func(config compilerConfig) error {
			if config.command == "" {
				return fmt.Errorf("compiler command is required")
			}
			return nil
		},
		func(_ context.Context, config compilerConfig) (compilerObservation, error) {
			if config.command == "compile fixture --accept" {
				return compilerObservation{artifactRef: "compiler-run:accepted"}, nil
			}
			return compilerObservation{
				artifactRef: "compiler-run:rejected", exitCode: 1, diagnostic: "undefined symbol",
			}, nil
		},
		func(_ context.Context, _ compilerConfig, observation compilerObservation) (bool, error) {
			return observation.exitCode == 0, nil
		},
		func(_ context.Context, _ compilerConfig, observation compilerObservation) (compilerFailure, error) {
			return compilerFailure{kind: "compiler_diagnostic", diagnostic: observation.diagnostic}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}
