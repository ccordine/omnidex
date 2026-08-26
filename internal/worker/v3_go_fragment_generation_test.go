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
	var prompt string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			var err error
			prompt, _, err = assemblyline.RenderPortableJob(job)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: "func Added() int { return 2 }"}, err
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
	if got != "func Added() int {\n\treturn 2\n}" {
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
