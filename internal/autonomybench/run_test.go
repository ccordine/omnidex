package autonomybench

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunDoesNotLoadEvaluationUntilBuildReturns(t *testing.T) {
	t.Parallel()

	const requestText = "Build a small browser music studio with channels, drum pads, and a keyboard."
	buildReturned := false
	builder := BuilderFunc(func(_ context.Context, input BuildInput) (BuildObservation, error) {
		if input.UserRequest != requestText {
			t.Fatalf("builder received %q, want the unmodified user request", input.UserRequest)
		}
		if input.Workspace != "/isolated/workspace" {
			t.Fatalf("builder workspace = %q", input.Workspace)
		}
		if reflect.TypeOf(input).NumField() != 2 {
			t.Fatalf("BuildInput grew beyond request and workspace: %#v", input)
		}
		buildReturned = true
		return BuildObservation{ModelCalls: 4, FilesChanged: 7}, nil
	})

	loader := EvaluationLoaderFunc(func(_ context.Context, requestID string) (EvaluationPlan, error) {
		if !buildReturned {
			t.Fatal("evaluation was loaded before the build returned")
		}
		return EvaluationPlan{
			RequestID: requestID,
			Checks:    []Check{{ID: "hidden-drum-pad-check", Weight: 2}},
		}, nil
	})
	evaluator := EvaluatorFunc(func(_ context.Context, input EvaluationInput) ([]CheckResult, error) {
		return []CheckResult{{ID: input.Checks[0].ID, Passed: true, Evidence: "observed externally"}}, nil
	})

	result, err := Run(context.Background(), RequestCase{
		ID:          "music-studio",
		UserRequest: requestText,
		Workspace:   "/isolated/workspace",
	}, builder, loader, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusPassed || result.EarnedWeight != 2 || result.TotalWeight != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunMeasuresPartialWorkspaceAfterBuildFailure(t *testing.T) {
	t.Parallel()

	builderFailure := errors.New("framework stopped after typecheck failure")
	builder := BuilderFunc(func(_ context.Context, _ BuildInput) (BuildObservation, error) {
		return BuildObservation{ModelCalls: 9, FilesChanged: 5}, builderFailure
	})
	loader := EvaluationLoaderFunc(func(_ context.Context, requestID string) (EvaluationPlan, error) {
		return EvaluationPlan{
			RequestID: requestID,
			Checks: []Check{
				{ID: "loads", Weight: 1},
				{ID: "sequencer-works", Weight: 3},
			},
		}, nil
	})
	evaluator := EvaluatorFunc(func(_ context.Context, _ EvaluationInput) ([]CheckResult, error) {
		return []CheckResult{
			{ID: "loads", Passed: true, Evidence: "page rendered"},
			{ID: "sequencer-works", Passed: false, Evidence: "play did not advance"},
		}, nil
	})

	result, err := Run(context.Background(), RequestCase{
		ID: "partial-app", UserRequest: "Build the app.", Workspace: "/isolated/partial",
	}, builder, loader, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusPartial {
		t.Fatalf("status = %q, want %q", result.Status, StatusPartial)
	}
	if result.BuildError != builderFailure.Error() {
		t.Fatalf("build error = %q", result.BuildError)
	}
	if result.EarnedWeight != 1 || result.TotalWeight != 4 {
		t.Fatalf("score = %d/%d, want 1/4", result.EarnedWeight, result.TotalWeight)
	}
	if len(result.Checks) != 2 {
		t.Fatalf("checks = %#v", result.Checks)
	}
}

func TestRunFailsLoudlyWhenEvaluatorReturnsIncompleteResults(t *testing.T) {
	t.Parallel()

	builder := BuilderFunc(func(_ context.Context, _ BuildInput) (BuildObservation, error) {
		return BuildObservation{}, nil
	})
	loader := EvaluationLoaderFunc(func(_ context.Context, requestID string) (EvaluationPlan, error) {
		return EvaluationPlan{
			RequestID: requestID,
			Checks:    []Check{{ID: "loads", Weight: 1}, {ID: "saves", Weight: 1}},
		}, nil
	})
	evaluator := EvaluatorFunc(func(_ context.Context, _ EvaluationInput) ([]CheckResult, error) {
		return []CheckResult{{ID: "loads", Passed: true}}, nil
	})

	_, err := Run(context.Background(), RequestCase{
		ID: "invalid-evaluation", UserRequest: "Build the app.", Workspace: "/isolated/invalid",
	}, builder, loader, evaluator)
	if err == nil {
		t.Fatal("expected incomplete evaluation to fail")
	}
}
