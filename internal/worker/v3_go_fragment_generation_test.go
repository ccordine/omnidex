package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoFragmentGenerationReturnsOnlyOneSignatureBoundDeclaration(t *testing.T) {
	t.Parallel()
	const raw = "func Added() int { return 2 }"
	const declaration = raw
	var prompt string
	finalized := false
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			var err error
			prompt, err = assemblyline.RenderPortableJob(job)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, err
		},
		Finalize: func(
			_ assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			if validationErr != nil || result.Candidate != raw || result.Projection == nil ||
				result.Projection.Kind != assemblyline.PortableResultProjectionSourceDeclaration ||
				result.Projection.Source != declaration || result.Projection.StartByte != 0 ||
				result.Projection.EndByte != len(raw) || result.Projection.DiscardedBytes != 0 {
				t.Fatalf("finalized result=%+v validation=%v", result, validationErr)
			}
			finalized = true
			return nil
		},
	}
	got, err := runDirectCodingGoFragmentGenerationWorker(runtime, "coder", directCodingGoGenerationJob{
		Subject: "desired_artifact_opaque",
		Input: assemblyline.FragmentGenerationInput{
			Language: "go", Dialect: "Go 1.24", Signature: "func Added() int", Behavior: "return two",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != declaration || !finalized {
		t.Fatalf("declaration=%q", got)
	}
	for _, forbidden := range []string{"added.go", "/workspace", "create_file", "delete_file"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("model prompt leaked %q: %s", forbidden, prompt)
		}
	}
}

func TestGoFragmentGenerationRejectsOperationShapedResponse(t *testing.T) {
	t.Parallel()
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			raw, _ := json.Marshal(map[string]string{"create_file": "added.go"})
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, nil
		},
	}
	_, err := runDirectCodingGoFragmentGenerationWorker(runtime, "coder", directCodingGoGenerationJob{
		Subject: "desired_artifact_opaque",
		Input: assemblyline.FragmentGenerationInput{
			Language: "go", Dialect: "Go 1.24", Signature: "func Added() int", Behavior: "return two",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "parse Go fragment") {
		t.Fatalf("operation-shaped response error=%v", err)
	}
}

func TestGoFragmentGenerationValidatesCapabilityAndGlobalChannelsTogether(t *testing.T) {
	t.Parallel()
	const raw = `func Added() int { return CapabilityValue() + globalValue }`
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		},
	}
	got, err := runDirectCodingGoFragmentGenerationWorker(
		runtime, "coder", directCodingGoGenerationJob{
			Subject: "channel_union_opaque",
			Input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func Added() int",
				Behavior:         "Return the sum of the direct capability and the in-scope global.",
				Capabilities:     []string{"func CapabilityValue() int"},
				PermittedSymbols: []string{"globalValue"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != raw {
		t.Fatalf("channel-union declaration=%q want %q", got, raw)
	}
}
