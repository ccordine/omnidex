package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func TestLanguageCorrectionPersistsProjectedValidatorRejection(t *testing.T) {
	t.Parallel()
	const current = "func Value() int { return 1 }"
	const raw = " \n```go\nfunc Value() int { return hidden() }\n```\n "
	const projected = "func Value() int { return hidden() }"
	contract := gofragment.Contract{Signature: "func Value() int", Current: current}
	finalized := false
	runtime := typedWorkerRuntime{
		Context: t.Context(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		},
		Finalize: func(
			_ assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			if result.Candidate != raw || result.Projection == nil ||
				result.Projection.Source != projected ||
				result.Projection.Source != raw[result.Projection.StartByte:result.Projection.EndByte] ||
				!errors.Is(validationErr, errDirectCodingLanguageCorrectionInvalid) ||
				!strings.Contains(validationErr.Error(), `undeclared capability "hidden"`) {
				t.Fatalf("result=%+v validation=%v", result, validationErr)
			}
			finalized = true
			return nil
		},
	}
	_, err := runDirectCodingLanguageCorrection(
		runtime, "executor", "opaque", current, "Use only permitted identifiers.",
		"go",
		func(candidate string) (string, error) {
			return gofragment.ParseFunction(contract, candidate)
		},
	)
	if !errors.Is(err, errDirectCodingLanguageCorrectionInvalid) || !finalized {
		t.Fatalf("validator rejection=%v finalized=%t", err, finalized)
	}
}

func TestLanguageCorrectionDetectsFencedByteIdenticalSourceAfterProjection(t *testing.T) {
	t.Parallel()
	const current = "func Value() int { return 1 }"
	const raw = "```go\nfunc Value() int { return 1 }\n```"
	finalized := false
	runtime := typedWorkerRuntime{
		Context: t.Context(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		},
		Finalize: func(
			_ assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			if result.Projection == nil || result.Projection.Source != current ||
				!errors.Is(validationErr, errDirectCodingLanguageCorrectionUnchanged) {
				t.Fatalf("result=%+v validation=%v", result, validationErr)
			}
			finalized = true
			return nil
		},
	}
	_, err := runDirectCodingLanguageCorrection(
		runtime, "executor", "opaque", current, "Change the declaration.",
		"go",
		func(candidate string) (string, error) {
			t.Fatalf("byte-identical projected source reached validator: %q", candidate)
			return "", nil
		},
	)
	if !errors.Is(err, errDirectCodingLanguageCorrectionUnchanged) || !finalized {
		t.Fatalf("unchanged correction=%v finalized=%t", err, finalized)
	}
}

func TestLanguageRepairAcceptedTransitionCeilingPreventsFurtherExecutorCalls(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24 function syntax",
		Signature: "func Value() int", Behavior: "Return one value.",
	}
	block := assemblyline.SourceBlock{
		ID: "feature.value", Signature: input.Signature, API: input.Signature,
		TaskID: "task_001", Role: assemblyline.SourceBlockTaskImplementation,
	}
	document := assemblyline.SourceDocument{
		ID: "feature", AdapterID: "go", Blocks: []assemblyline.SourceBlock{block},
	}
	stage := &directCodingProgram{Source: assemblyline.SourceBlueprint{
		Documents: []assemblyline.SourceDocument{document},
	}}
	executor := &directCodingLanguageProjectStageExecutor{
		config: directCodingLanguageStageConfig{Language: "go"},
		acceptedRepairTransitions: map[string]int{
			block.ID: maxDirectCodingLanguageRepairTransitions,
		},
		repairGuidance: make(map[string]map[string]struct{}),
		repairSources:  make(map[string]map[string]struct{}),
	}
	calls := 0
	_, err := executor.repairLanguageBlockWithRuntime(
		typedWorkerRuntime{
			Context: t.Context(), MaxAttempts: maxTypedWorkerAttempts,
			Execute: func(
				assemblyline.PortableJob, string,
			) (assemblyline.PortableResult, error) {
				calls++
				return assemblyline.PortableResult{}, nil
			},
		},
		"guidance", "executor", stage,
		assemblyline.SourceBlockRef{Document: document, Block: block}, input,
		"func Value() int { return missing() }",
		`SOURCE_DIAGNOSTIC: Go fragment references undeclared capability "missing"`,
		validateDirectCodingGoFragment,
	)
	if err == nil || !strings.Contains(err.Error(), "accepted code-owned repair transitions") ||
		calls != 0 {
		t.Fatalf("ceiling error=%v calls=%d", err, calls)
	}
}
