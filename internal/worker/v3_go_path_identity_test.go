package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestGoFragmentGenerationRejectsPathIdentityBeforeInferenceOrAcceptance(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		[]string{"internal/transport.go"},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("known input basename before inference", func(t *testing.T) {
		calls := 0
		runtime := typedWorkerRuntime{
			Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
			Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
				calls++
				return assemblyline.PortableResult{}, nil
			},
		}
		_, err := runDirectCodingGoFragmentGenerationWorker(
			runtime, "coder", directCodingGoGenerationJob{
				Subject: "opaque",
				Input: assemblyline.FragmentGenerationInput{
					Language: "go", Dialect: "Go 1.24", Signature: "func Added() string",
					Behavior: "return the contents of transport.go",
				},
			},
		)
		if err == nil || !strings.Contains(err.Error(), "transport.go") || calls != 0 {
			t.Fatalf("known input path error=%v calls=%d", err, calls)
		}
	})

	for name, candidate := range map[string]string{
		"known basename":      `func Added() string { return "transport.go" }`,
		"arbitrary suffix":    `func Added() string { return "foo/value.unregistered" }`,
		"unix absolute":       `func Added() string { return "/workspace/generated" }`,
		"tilde":               "func Added() string { return `~/private/value` }",
		"drive relative":      "func Added() string { return `C:private\\value` }",
		"windows absolute":    "func Added() string { return `C:\\private\\value` }",
		"unc":                 "func Added() string { return `\\\\server\\share\\value` }",
		"windows device path": "func Added() string { return `\\\\?\\C:\\private\\value` }",
	} {
		name, candidate := name, candidate
		t.Run(name, func(t *testing.T) {
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			_, err := runDirectCodingGoFragmentGenerationWorker(
				runtime, "coder", directCodingGoGenerationJob{
					Subject: "opaque",
					Input: assemblyline.FragmentGenerationInput{
						Language: "go", Dialect: "Go 1.24", Signature: "func Added() string", Behavior: "return a label",
					},
				},
			)
			if err == nil || !strings.Contains(err.Error(), "filesystem identity") || calls != 1 {
				t.Fatalf("path-bearing candidate error=%v calls=%d", err, calls)
			}
		})
	}
}

func TestGoFragmentGenerationRetainsUnprovenNodeJSLiteral(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		[]string{"internal/transport.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: `func Added() string { return "Node.js" }`,
			}, nil
		},
	}
	got, err := runDirectCodingGoFragmentGenerationWorker(
		runtime, "coder", directCodingGoGenerationJob{
			Subject: "opaque",
			Input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func Added() string", Behavior: "return Node.js",
			},
		},
	)
	if err != nil || !strings.Contains(got, `"Node.js"`) {
		t.Fatalf("Node.js result=%q error=%v", got, err)
	}
}

func TestGoFragmentModificationUsesSameProvenanceBoundary(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		[]string{"internal/transport.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	base := assemblyline.FragmentModificationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Value() string",
		CurrentDeclaration: `func Value() string { return "old" }`,
		RequirementQuote:   "return a new label",
	}

	t.Run("known requirement basename before inference", func(t *testing.T) {
		input := base
		input.RequirementQuote = "replace the value using transport.go"
		calls := 0
		runtime := typedWorkerRuntime{
			Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
			Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
				calls++
				return assemblyline.PortableResult{}, nil
			},
		}
		_, err := runDirectCodingGoFragmentModificationWorker(
			runtime, "coder", directCodingGoModificationJob{Subject: "opaque", Input: input},
		)
		if err == nil || !strings.Contains(err.Error(), "transport.go") || calls != 0 {
			t.Fatalf("known input path error=%v calls=%d", err, calls)
		}
	})

	t.Run("qualified candidate before acceptance", func(t *testing.T) {
		runtime := typedWorkerRuntime{
			Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
			Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: `func Value() string { return "../private/value.anything" }`,
				}, nil
			},
		}
		_, err := runDirectCodingGoFragmentModificationWorker(
			runtime, "coder", directCodingGoModificationJob{Subject: "opaque", Input: base},
		)
		if err == nil || !strings.Contains(err.Error(), "filesystem identity") {
			t.Fatalf("path-bearing modification error=%v", err)
		}
	})
}
