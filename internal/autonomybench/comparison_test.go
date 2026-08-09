package autonomybench

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRunComparisonBuildsBothVariantsBeforeLoadingWithheldEvaluation(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, 5)
	const requestText = "Build a browser application from this ordinary request."
	baseline := BuilderFunc(func(_ context.Context, input BuildInput) (BuildObservation, error) {
		if input.UserRequest != requestText || input.Workspace != "/fresh/baseline" {
			t.Fatalf("baseline input=%#v", input)
		}
		if reflect.TypeOf(input).NumField() != 2 {
			t.Fatalf("BuildInput exposed comparison state: %#v", input)
		}
		events = append(events, "baseline-build")
		return BuildObservation{ModelCalls: 10, UnitsAccepted: 4}, nil
	})
	assisted := BuilderFunc(func(_ context.Context, input BuildInput) (BuildObservation, error) {
		if input.UserRequest != requestText || input.Workspace != "/fresh/assisted" {
			t.Fatalf("assisted input=%#v", input)
		}
		events = append(events, "assisted-build")
		return BuildObservation{ModelCalls: 14, UnitsAccepted: 6}, nil
	})
	loader := EvaluationLoaderFunc(func(_ context.Context, requestID string) (EvaluationPlan, error) {
		events = append(events, "load-rubric")
		if !reflect.DeepEqual(events, []string{"baseline-build", "assisted-build", "load-rubric"}) {
			t.Fatalf("rubric loaded before both builds stopped: %#v", events)
		}
		return EvaluationPlan{RequestID: requestID, Checks: []Check{
			{ID: "loads", Weight: 1}, {ID: "interaction", Weight: 3},
		}}, nil
	})
	evaluator := EvaluatorFunc(func(_ context.Context, input EvaluationInput) ([]CheckResult, error) {
		events = append(events, "evaluate:"+input.Workspace)
		interaction := input.Workspace == "/fresh/assisted"
		return []CheckResult{
			{ID: "loads", Passed: true, Evidence: "observer loaded the page"},
			{ID: "interaction", Passed: interaction, Evidence: "observer exercised the interaction"},
		}, nil
	})

	result, err := RunComparison(context.Background(), ComparisonRequest{
		ID: "paired-app", UserRequest: requestText,
		BaselineWorkspace: "/fresh/baseline", AssistedWorkspace: "/fresh/assisted",
	}, baseline, assisted, loader, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != ComparisonSchemaV1 || result.Baseline.EarnedWeight != 1 || result.Assisted.EarnedWeight != 4 {
		t.Fatalf("comparison result=%#v", result)
	}
	if result.EarnedWeightDelta != 3 || len(result.Checks) != 2 {
		t.Fatalf("comparison delta=%#v", result)
	}
	if result.Checks[1].BaselinePassed || !result.Checks[1].AssistedPassed {
		t.Fatalf("interaction delta=%#v", result.Checks[1])
	}
}

func TestRunComparisonRejectsSharedWorkspaceBeforeEitherBuild(t *testing.T) {
	t.Parallel()
	called := false
	builder := BuilderFunc(func(context.Context, BuildInput) (BuildObservation, error) {
		called = true
		return BuildObservation{}, nil
	})
	_, err := RunComparison(context.Background(), ComparisonRequest{
		ID: "shared", UserRequest: "Build the app.",
		BaselineWorkspace: "/same", AssistedWorkspace: "/same/.",
	}, builder, builder, EvaluationLoaderFunc(nil), EvaluatorFunc(nil))
	if err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("shared workspace error=%v", err)
	}
	if called {
		t.Fatal("invalid comparison started a build")
	}
}

func TestRunComparisonEvaluatesBothPartialWorkspacesAfterBuildFailures(t *testing.T) {
	t.Parallel()
	baselineFailure := errors.New("baseline compiler stopped")
	assistedFailure := errors.New("assisted compiler stopped")
	baseline := BuilderFunc(func(context.Context, BuildInput) (BuildObservation, error) {
		return BuildObservation{UnitsAccepted: 2}, baselineFailure
	})
	assisted := BuilderFunc(func(context.Context, BuildInput) (BuildObservation, error) {
		return BuildObservation{UnitsAccepted: 3}, assistedFailure
	})
	loader := EvaluationLoaderFunc(func(_ context.Context, requestID string) (EvaluationPlan, error) {
		return EvaluationPlan{RequestID: requestID, Checks: []Check{{ID: "partial", Weight: 1}}}, nil
	})
	evaluated := make([]string, 0, 2)
	evaluator := EvaluatorFunc(func(_ context.Context, input EvaluationInput) ([]CheckResult, error) {
		evaluated = append(evaluated, input.Workspace)
		return []CheckResult{{ID: "partial", Passed: true, Evidence: "partial behavior observed"}}, nil
	})
	result, err := RunComparison(context.Background(), ComparisonRequest{
		ID: "partial", UserRequest: "Build the app.",
		BaselineWorkspace: "/fresh/a", AssistedWorkspace: "/fresh/b",
	}, baseline, assisted, loader, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	if result.Baseline.BuildError != baselineFailure.Error() || result.Assisted.BuildError != assistedFailure.Error() {
		t.Fatalf("build failures were hidden: %#v", result)
	}
	if !reflect.DeepEqual(evaluated, []string{"/fresh/a", "/fresh/b"}) {
		t.Fatalf("evaluated workspaces=%#v", evaluated)
	}
}
