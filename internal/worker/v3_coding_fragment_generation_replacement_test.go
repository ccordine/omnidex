package worker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestTypeScriptOutputLimitGetsOneExplicitSourceReplacement(t *testing.T) {
	t.Parallel()
	const rejectedPrefix = "function formatGuestLabel(name: string"
	const source = `function formatGuestLabel(name: string, checkedIn: boolean): string { return checkedIn ? name + " checked in" : name; }`
	calls := 0
	finalized := false
	execute := func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
		calls++
		if model != "coder" {
			t.Fatalf("model=%q", model)
		}
		switch calls {
		case 1:
			if job.Kind != assemblyline.WorkFragmentGeneration {
				t.Fatalf("initial kind=%q", job.Kind)
			}
			return assemblyline.PortableResult{}, fragmentOutputLimitFixture()
		case 2:
			assertReplacementEnvelope(t, job, rejectedPrefix)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: source}, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return assemblyline.PortableResult{}, nil
		}
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: execute,
		ExecuteFragmentGenerationReplacement: func(
			job assemblyline.PortableJob,
			model string,
			origin queue.StationGapReplacementOrigin,
		) (assemblyline.PortableResult, error) {
			assertReplacementOrigin(t, origin)
			return execute(job, model)
		},
		Finalize: func(
			job assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			if job.Kind != assemblyline.WorkFragmentGenerationReplacement ||
				validationErr != nil || result.Projection == nil ||
				result.Projection.Kind != assemblyline.PortableResultProjectionTypeScriptFunction {
				t.Fatalf("final job=%q result=%+v validation=%v", job.Kind, result, validationErr)
			}
			finalized = true
			return nil
		},
	}
	got, err := runDirectCodingTypeScriptFragmentWorker(
		runtime, "coder", directCodingTypeScriptFragmentJob{
			dialect: "TypeScript 5.9.3",
			block: assemblyline.SourceBlock{
				ID:        "guest.label",
				Signature: "function formatGuestLabel(name: string, checkedIn: boolean): string",
				Contract:  "Return the guest name with a checked-in suffix only when checked in.",
				API:       "function formatGuestLabel(name: string, checkedIn: boolean): string",
			},
		},
	)
	if err != nil || got != source || calls != 2 || !finalized {
		t.Fatalf("source=%q calls=%d finalized=%t error=%v", got, calls, finalized, err)
	}
}

func TestGoOutputLimitGetsOneExplicitSourceReplacement(t *testing.T) {
	t.Parallel()
	const rejectedPrefix = "func RetryDelay(attempt int) int {"
	const source = "func RetryDelay(attempt int) int {\n\tif attempt < 1 {\n\t\treturn 0\n\t}\n\treturn attempt * 2\n}"
	calls := 0
	finalized := false
	execute := func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
		calls++
		if model != "coder" {
			t.Fatalf("model=%q", model)
		}
		switch calls {
		case 1:
			if job.Kind != assemblyline.WorkFragmentGeneration {
				t.Fatalf("initial kind=%q", job.Kind)
			}
			return assemblyline.PortableResult{}, fragmentOutputLimitFixture()
		case 2:
			assertReplacementEnvelope(t, job, rejectedPrefix)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: source}, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return assemblyline.PortableResult{}, nil
		}
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: execute,
		ExecuteFragmentGenerationReplacement: func(
			job assemblyline.PortableJob,
			model string,
			origin queue.StationGapReplacementOrigin,
		) (assemblyline.PortableResult, error) {
			assertReplacementOrigin(t, origin)
			return execute(job, model)
		},
		Finalize: func(
			job assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			if job.Kind != assemblyline.WorkFragmentGenerationReplacement ||
				validationErr != nil || result.Projection == nil ||
				result.Projection.Kind != assemblyline.PortableResultProjectionSourceDeclaration {
				t.Fatalf("final job=%q result=%+v validation=%v", job.Kind, result, validationErr)
			}
			finalized = true
			return nil
		},
	}
	got, err := runDirectCodingGoFragmentGenerationWorker(
		runtime, "coder", directCodingGoGenerationJob{
			Subject: "delay.declaration",
			Input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func RetryDelay(attempt int) int",
				Behavior: "Return zero before the first attempt and twice the attempt otherwise.",
			},
		},
	)
	if err != nil || got != source || calls != 2 || !finalized {
		t.Fatalf("source=%q calls=%d finalized=%t error=%v", got, calls, finalized, err)
	}
}

func TestFragmentOutputLimitReplacementCannotRepeat(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func RetryDelay(attempt int) int",
		Behavior: "Return a bounded retry delay.",
	}
	initial, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	execute := func(
		job assemblyline.PortableJob,
		_ string,
	) (assemblyline.PortableResult, error) {
		calls++
		if calls == 1 && job.Kind != assemblyline.WorkFragmentGeneration {
			t.Fatalf("initial kind=%q", job.Kind)
		}
		if calls == 2 && job.Kind != assemblyline.WorkFragmentGenerationReplacement {
			t.Fatalf("replacement kind=%q", job.Kind)
		}
		return assemblyline.PortableResult{}, fragmentOutputLimitFixture()
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), Execute: execute,
		ExecuteFragmentGenerationReplacement: func(
			job assemblyline.PortableJob,
			model string,
			origin queue.StationGapReplacementOrigin,
		) (assemblyline.PortableResult, error) {
			assertReplacementOrigin(t, origin)
			return execute(job, model)
		},
	}
	_, _, err = executeInitialFragmentGenerationWithReplacement(
		runtime, initial, input, "coder",
	)
	if err == nil || calls != 2 || !strings.Contains(err.Error(), "replacement failed") {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestFragmentOutputLimitReplacementRequiresPersistedInitialFailure(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func RetryDelay(attempt int) int",
		Behavior: "Return a bounded retry delay.",
	}
	initial, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	outputLimit := rawFragmentOutputLimitFixture()
	persistenceFailure := errors.New("terminal station gap outcome was not persisted")
	failedBoundary := persistedStationGapFailure(outputLimit, persistenceFailure)
	var routed *persistedFragmentGenerationOutputLimitFailure
	if errors.As(failedBoundary, &routed) {
		t.Fatal("persistence failure retained output-limit routing authority")
	}
	calls := 0
	runtime := typedWorkerRuntime{Context: context.Background(), Execute: func(
		job assemblyline.PortableJob,
		_ string,
	) (assemblyline.PortableResult, error) {
		calls++
		if job.Kind != assemblyline.WorkFragmentGeneration {
			t.Fatalf("unexpected replacement kind=%q", job.Kind)
		}
		return assemblyline.PortableResult{}, failedBoundary
	}}
	_, _, err = executeInitialFragmentGenerationWithReplacement(
		runtime, initial, input, "coder",
	)
	if err == nil || calls != 1 || !strings.Contains(err.Error(), persistenceFailure.Error()) {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestRawFragmentOutputLimitCannotRouteReplacement(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func RetryDelay(attempt int) int",
		Behavior: "Return a bounded retry delay.",
	}
	initial, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(
			job assemblyline.PortableJob,
			_ string,
		) (assemblyline.PortableResult, error) {
			calls++
			if job.Kind != assemblyline.WorkFragmentGeneration {
				t.Fatalf("unexpected replacement kind=%q", job.Kind)
			}
			return assemblyline.PortableResult{}, rawFragmentOutputLimitFixture()
		},
		ExecuteFragmentGenerationReplacement: func(
			assemblyline.PortableJob,
			string,
			queue.StationGapReplacementOrigin,
		) (assemblyline.PortableResult, error) {
			t.Fatal("raw unpersisted output-limit evidence routed a replacement")
			return assemblyline.PortableResult{}, nil
		},
	}
	_, _, err = executeInitialFragmentGenerationWithReplacement(
		runtime, initial, input, "coder",
	)
	var raw *llm.ExactPreparedOutputLimitReachedError
	if err == nil || calls != 1 || !errors.As(err, &raw) {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
