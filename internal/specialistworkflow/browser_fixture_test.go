package specialistworkflow_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/specialistworkflow"
)

type browserCandidate struct {
	id      string
	summary string
}

type browserState struct {
	target       string
	candidates   []browserCandidate
	selectorID   string
	waitStrategy string
}

type browserConfig struct {
	target       string
	selectorID   string
	waitStrategy string
}

type browserObservation struct {
	evidenceRef string
	loading     bool
	visible     bool
}

type browserFailure struct{ kind string }

func (value browserState) ValidateBounds() error {
	if len(value.target) > 256 || len(value.selectorID) > 128 || len(value.waitStrategy) > 64 || len(value.candidates) > 8 {
		return fmt.Errorf("browser state exceeds fixture bounds")
	}
	return nil
}
func (value browserConfig) ValidateBounds() error {
	if len(value.target) > 256 || len(value.selectorID) > 128 || len(value.waitStrategy) > 64 {
		return fmt.Errorf("browser configuration exceeds fixture bounds")
	}
	return nil
}
func (value browserObservation) ValidateBounds() error {
	if len(value.evidenceRef) > 256 {
		return fmt.Errorf("browser observation exceeds fixture bounds")
	}
	return nil
}
func (value browserFailure) ValidateBounds() error {
	if len(value.kind) > 64 {
		return fmt.Errorf("browser failure exceeds fixture bounds")
	}
	return nil
}
func (value browserState) Clone() browserState {
	clone := value
	clone.candidates = append([]browserCandidate(nil), value.candidates...)
	return clone
}
func (value browserConfig) Clone() browserConfig           { return value }
func (value browserObservation) Clone() browserObservation { return value }
func (value browserFailure) Clone() browserFailure         { return value }

type browserSelectorCall struct {
	question   string
	candidates []browserCandidate
}

type browserSelectorLeaf struct{ candidateID string }

type browserSelectorRequiredError struct{ call browserSelectorCall }

func (err *browserSelectorRequiredError) Error() string {
	return "browser selector requires semantic resolution"
}

func TestBrowserFixtureUsesCodeOwnedRetryWithoutSemanticResolution(t *testing.T) {
	semanticCalls := 0
	state := browserState{
		target: "http://fixture.invalid", selectorID: "selector.chart",
		waitStrategy: "network_settled",
	}
	contract := browserContract(t)
	budget, _ := specialistworkflow.NewAttemptBudget(2)

	first, err := specialistworkflow.RunAttempt(context.Background(), budget, state, contract)
	if err != nil {
		t.Fatal(err)
	}
	failure, ok, failureErr := first.Failure()
	if failureErr != nil {
		t.Fatal(failureErr)
	}
	if first.Verified() || !ok || failure.kind != "loading_present" {
		t.Fatalf("first receipt verified=%t failure=%#v", first.Verified(), failure)
	}
	state = applyBrowserFailure(state, failure)
	second, err := specialistworkflow.RunAttempt(context.Background(), budget, state, contract)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Verified() || second.Attempt() != 2 || semanticCalls != 0 {
		t.Fatalf("second receipt verified=%t attempt=%d semantic_calls=%d", second.Verified(), second.Attempt(), semanticCalls)
	}
}

func TestBrowserFixtureCrossesOneDomainOwnedSemanticLeafThenCodeResumes(t *testing.T) {
	state := browserState{
		target:       "http://fixture.invalid",
		waitStrategy: "loading_marker_absent",
		candidates: []browserCandidate{
			{id: "selector.canvas", summary: "rendering surface"},
			{id: "selector.container", summary: "layout container"},
		},
	}
	contract := browserContract(t)
	budget, _ := specialistworkflow.NewAttemptBudget(1)

	_, err := specialistworkflow.RunAttempt(context.Background(), budget, state, contract)
	var required *browserSelectorRequiredError
	if !errors.As(err, &required) {
		t.Fatalf("derivation error=%v", err)
	}
	semanticCalls := 0
	resolve := func(call browserSelectorCall) (browserSelectorLeaf, error) {
		semanticCalls++
		if call.question != "Which candidate denotes the rendered result?" || len(call.candidates) != 2 {
			return browserSelectorLeaf{}, fmt.Errorf("unexpected selector call")
		}
		return browserSelectorLeaf{candidateID: "selector.canvas"}, nil
	}
	leaf, err := resolve(required.call)
	if err != nil {
		t.Fatal(err)
	}
	state, err = acceptBrowserSelector(state, required.call, leaf)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := specialistworkflow.RunAttempt(context.Background(), budget, state, contract)
	if err != nil {
		t.Fatal(err)
	}
	observation, ok, observationErr := receipt.Observation()
	if observationErr != nil {
		t.Fatal(observationErr)
	}
	if !receipt.Verified() || receipt.Attempt() != 1 || semanticCalls != 1 ||
		!ok || observation.evidenceRef != "browser-run:selector.canvas" {
		t.Fatalf("receipt=%#v observation=%#v present=%t semantic_calls=%d", receipt, observation, ok, semanticCalls)
	}
}

func browserContract(t *testing.T) specialistworkflow.Contract[
	browserState, browserConfig, browserObservation, browserFailure,
] {
	t.Helper()
	registration := mustRegistration(t, "rendered.observation", "browser.observer")
	contract, err := specialistworkflow.NewContract(
		registration,
		func(state browserState) (browserConfig, error) {
			if state.selectorID == "" {
				return browserConfig{}, &browserSelectorRequiredError{call: browserSelectorCall{
					question:   "Which candidate denotes the rendered result?",
					candidates: append([]browserCandidate(nil), state.candidates...),
				}}
			}
			return browserConfig{
				target: state.target, selectorID: state.selectorID, waitStrategy: state.waitStrategy,
			}, nil
		},
		func(config browserConfig) error {
			if config.target == "" || config.selectorID == "" {
				return fmt.Errorf("browser configuration is incomplete")
			}
			switch config.waitStrategy {
			case "network_settled", "loading_marker_absent":
				return nil
			default:
				return fmt.Errorf("unsupported wait strategy %q", config.waitStrategy)
			}
		},
		func(_ context.Context, config browserConfig) (browserObservation, error) {
			return browserObservation{
				evidenceRef: "browser-run:" + config.selectorID,
				loading:     config.waitStrategy == "network_settled",
				visible:     config.waitStrategy == "loading_marker_absent",
			}, nil
		},
		func(_ context.Context, _ browserConfig, observation browserObservation) (bool, error) {
			return observation.visible && !observation.loading, nil
		},
		func(_ context.Context, _ browserConfig, observation browserObservation) (browserFailure, error) {
			if observation.loading {
				return browserFailure{kind: "loading_present"}, nil
			}
			return browserFailure{kind: "rendered_result_absent"}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func applyBrowserFailure(state browserState, failure browserFailure) browserState {
	next := state.Clone()
	if failure.kind == "loading_present" {
		next.waitStrategy = "loading_marker_absent"
	}
	return next
}

func acceptBrowserSelector(
	state browserState,
	call browserSelectorCall,
	leaf browserSelectorLeaf,
) (browserState, error) {
	for _, candidate := range call.candidates {
		if candidate.id == leaf.candidateID {
			next := state.Clone()
			next.selectorID = leaf.candidateID
			return next, nil
		}
	}
	return browserState{}, fmt.Errorf("selector leaf %q is outside the bounded candidates", leaf.candidateID)
}
