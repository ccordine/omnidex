package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestAcceptanceGroundingReviewStopsExactNoOp(t *testing.T) {
	t.Parallel()
	input, fields := directCodingGroundingReviewStopFixture(t)
	values := make(map[string]any, len(fields))
	for _, field := range fields {
		values[field] = true
	}
	values[fields[0]] = "invalid"
	initialBytes, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			if calls == 1 {
				return directCodingPortableCandidate(job, string(initialBytes)), nil
			}
			var payload assemblyline.ResponseCorrectionInput
			if err := json.Unmarshal(job.Payload, &payload); err != nil {
				return assemblyline.PortableResult{}, err
			}
			return directCodingPortableCandidate(job, fmt.Sprintf(`{%q:"invalid"}`, payload.TargetField)), nil
		},
	}
	_, err = runDirectCodingAcceptanceGroundingReview(runtime, "reviewer", "acceptance.001", input)
	if err == nil || !strings.Contains(err.Error(), "changed 0 JSON leaves") || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestAcceptanceGroundingReviewStopsPriorStateCycle(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	for _, candidate := range []string{"state-a", "state-b"} {
		if err := rememberGroundingReviewState(seen, candidate); err != nil {
			t.Fatal(err)
		}
	}
	if err := rememberGroundingReviewState(seen, "state-a"); err == nil ||
		!strings.Contains(err.Error(), "repeated prior retained state") {
		t.Fatalf("cycle error=%v", err)
	}
}

func TestAcceptanceGroundingReviewRejectsUnrelatedLeafCorrection(t *testing.T) {
	t.Parallel()
	input, fields := directCodingGroundingReviewStopFixture(t)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			switch calls {
			case 1:
				return directCodingPortableCandidate(
					job, directCodingGroundingRaw(t, map[string]bool{fields[0]: true}),
				), nil
			case 2:
				return directCodingPortableCandidate(
					job, directCodingGroundingRaw(t, map[string]bool{fields[0]: false}),
				), nil
			default:
				return assemblyline.PortableResult{}, errors.New("unrelated correction was allowed to continue")
			}
		},
	}
	_, err := runDirectCodingAcceptanceGroundingReview(runtime, "reviewer", "acceptance.001", input)
	if err == nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestAcceptanceGroundingReviewRejectsExtraInitialFieldBeforeCorrection(t *testing.T) {
	t.Parallel()
	input, _ := directCodingGroundingReviewStopFixture(t)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			if calls > 1 {
				return assemblyline.PortableResult{}, errors.New("extra-field state entered correction")
			}
			return directCodingPortableCandidate(job, `{"invented_authority":true}`), nil
		},
	}
	_, err := runDirectCodingAcceptanceGroundingReview(runtime, "reviewer", "acceptance.001", input)
	if err == nil || calls != 1 || !strings.Contains(err.Error(), "unsupported fields") {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestAcceptanceGroundingReviewRejectsEqualCardinalityExtraSubstitutionBeforeCorrection(t *testing.T) {
	t.Parallel()
	input, fields := directCodingGroundingReviewStopFixture(t)
	values := make(map[string]bool, len(fields))
	for _, field := range fields {
		values[field] = true
	}
	delete(values, fields[0])
	values["invented_authority"] = true
	candidate := directCodingGroundingRaw(t, values)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			if calls > 1 {
				return assemblyline.PortableResult{}, errors.New("equal-cardinality extra entered correction")
			}
			return directCodingPortableCandidate(job, candidate), nil
		},
	}
	_, err := runDirectCodingAcceptanceGroundingReview(runtime, "reviewer", "acceptance.001", input)
	if err == nil || calls != 1 || !strings.Contains(err.Error(), "unsupported fields") {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestAcceptanceGroundingReviewStopsContextProviderAndFinalizeFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*typedWorkerRuntime, context.CancelFunc, *int)
		want      string
		wantCalls int
	}{
		{
			name: "context",
			configure: func(runtime *typedWorkerRuntime, cancel context.CancelFunc, _ *int) {
				runtime.Finalize = func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
					if validationErr != nil {
						cancel()
					}
					return nil
				}
			},
			want: "context canceled", wantCalls: 1,
		},
		{
			name: "provider",
			configure: func(runtime *typedWorkerRuntime, _ context.CancelFunc, calls *int) {
				base := runtime.Execute
				runtime.Execute = func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
					if *calls == 1 {
						(*calls)++
						return assemblyline.PortableResult{}, errors.New("provider unavailable")
					}
					return base(job, model)
				}
			},
			want: "provider unavailable", wantCalls: 2,
		},
		{
			name: "finalize",
			configure: func(runtime *typedWorkerRuntime, _ context.CancelFunc, _ *int) {
				runtime.Finalize = func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
					if validationErr == nil {
						return fmt.Errorf("expected rejected result")
					}
					return errors.New("persist rejection failed")
				}
			},
			want: "persist rejection failed", wantCalls: 1,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input, _ := directCodingGroundingReviewStopFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			runtime := typedWorkerRuntime{
				Context: ctx,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					return directCodingPortableCandidate(job, `{}`), nil
				},
			}
			test.configure(&runtime, cancel, &calls)
			_, err := runDirectCodingAcceptanceGroundingReview(runtime, "reviewer", "acceptance.001", input)
			if err == nil || !strings.Contains(err.Error(), test.want) || calls != test.wantCalls {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

func directCodingGroundingReviewStopFixture(
	t *testing.T,
) (assemblyline.ApplicationAcceptanceGroundingReviewInput, []string) {
	t.Helper()
	program := directCodingGroundingFixtureProgram(
		t, "inventory browser", "show stock items",
		[]string{"The stock-item collection is visible."},
		`expect(screen.getByText("Stock items")).toBeInTheDocument();`,
	)
	input := directCodingGroundingInput(t, program, "acceptance.001")
	fields := directCodingGroundingMatrixFields(input)
	if len(fields) < 2 {
		t.Fatalf("fixture has %d matrix fields; need at least two", len(fields))
	}
	return input, fields
}
