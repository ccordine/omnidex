package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestDesiredRepositoryGenerationRejectsForbiddenEnvelopeBeforeInference(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		prompt string
	}{
		{name: "physical operation", prompt: "choose create_file"},
		{name: "target path", prompt: "write omni_added_artifact.go"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
					called = true
					return assemblyline.PortableResult{}, nil
				},
			}
			paths := []string{"omni_added_artifact.go"}
			wrapped := runtime.Execute
			runtime.Execute = func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
				if err := validateDesiredRepositoryModelEnvelope(test.prompt, paths); err != nil {
					return assemblyline.PortableResult{}, err
				}
				return wrapped(job, model)
			}
			_, err := runtime.Execute(assemblyline.PortableJob{}, "model")
			if err == nil || called {
				t.Fatalf("forbidden envelope error=%v provider_called=%t", err, called)
			}
		})
	}
}

func TestDesiredRepositoryModelResponseCannotSelectOperationOrPath(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		[]string{"omni_added_artifact.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Added() string",
		Behavior: "Return one stable semantic value.",
	}
	for _, candidate := range []string{
		`{"create_file":"omni_added_artifact.go"}`,
		`func Added() string { return "omni_added_artifact.go" }`,
	} {
		_, err := runDirectCodingGoFragmentGenerationWorker(
			typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			},
			"model", directCodingGoGenerationJob{Subject: "DECLARATION_1", Input: input},
		)
		if err == nil {
			t.Fatalf("operation/path-bearing candidate was accepted: %s", candidate)
		}
	}
}

func TestCodeOwnedFilenameNeverSelectsGoBuildMembership(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"AddedTest", "AddedLinux", "AddedAmd64"} {
		value, err := deterministicGoSourceName(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(value, "_test.go") || strings.HasSuffix(value, "_linux.go") ||
			strings.HasSuffix(value, "_amd64.go") {
			t.Fatalf("declaration %q selected build membership through %q", name, value)
		}
	}
}

func TestUniqueGoPackagePlacementIgnoresImportOnlyPackageArtifacts(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	placement, err := repositoryfacts.UniqueGoPackagePlacement(snapshot, analysis)
	if err == nil {
		// This fixture has three local packages, so the only valid result is an
		// ambiguity over those packages rather than imported package artifacts.
		t.Fatalf("unexpected placement=%+v", placement)
	}
	if !strings.Contains(err.Error(), "across 3 opaque candidates") {
		t.Fatalf("local package ambiguity error=%v", err)
	}
}
