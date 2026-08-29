package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestMalformedDeclarationInitialFailureLeavesRepositoryUnchanged(t *testing.T) {
	t.Parallel()
	before, _ := existingRepositoryVerificationFixture(t)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			if calls == 0 && (job.Kind != assemblyline.WorkFragmentGeneration || model != "coder") {
				t.Fatalf("initial generation call=%q/%q", job.Kind, model)
			}
			calls++
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: `func Added() string { return "wrong" }`,
			}, nil
		},
	}
	_, err := runDirectCodingGoFragmentGenerationWorker(
		runtime, "coder", directCodingGoGenerationJob{
			Subject: "desired_artifact_opaque",
			Input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func Added() int", Behavior: "return two",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "initial candidate rejected") || calls != 1 {
		t.Fatalf("malformed generation error=%v calls=%d", err, calls)
	}
	after, err := repositoryfacts.BuildGitSnapshot(t.Context(), before.Root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID {
		t.Fatalf("generation exhaustion changed live inventory: before=%s after=%s", before.ID, after.ID)
	}
	if _, err := os.Stat(filepath.Join(before.Root, "omni_added_artifact.go")); !os.IsNotExist(err) {
		t.Fatalf("generation exhaustion created code-owned target: %v", err)
	}
}

func TestFailedStagedDesiredStateProofLeavesLiveRepositoryUnchanged(t *testing.T) {
	t.Parallel()
	before, analysis := existingRepositoryVerificationFixture(t)
	graph, err := repositoryfacts.NewDesiredArtifactGraph(
		before, analysis, "objective_typecheck_failure",
		[]repositoryfacts.DesiredGoArtifact{{
			RequirementQuote:  "Add func Added() int.",
			PackageArtifactID: desiredStatePackageID(t, analysis, "verification"),
			Signature:         "func Added() int", MustExist: true,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compileDesiredRepositoryFileStates(
		graph, before, analysis,
		map[string]string{graph.Artifacts[0].ID: `func Added() int { return "not an int" }`},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: before, Analysis: analysis, OwnerID: graph.ID, Desired: compiled.States,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	if output, err := desiredStateGoTestProjected(t.Context(), before.Root, stage); err == nil ||
		!strings.Contains(string(output), "not an int") {
		t.Fatalf("invalid staged source proof error=%v output=%s", err, output)
	}
	after, err := repositoryfacts.BuildGitSnapshot(t.Context(), before.Root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID {
		t.Fatalf("failed staged proof changed live repository: before=%s after=%s", before.ID, after.ID)
	}
	if _, err := os.Stat(filepath.Join(before.Root, compiled.States[0].Path)); !os.IsNotExist(err) {
		t.Fatalf("failed staged proof created authoritative target: %v", err)
	}
}

func TestExactDesiredPostRejectsUnexpectedUnrelatedDeletion(t *testing.T) {
	t.Parallel()
	before, analysis := existingRepositoryVerificationFixture(t)
	stage := desiredVerificationCreateStage(t, before, analysis)
	t.Cleanup(func() { _ = stage.Cleanup() })
	if _, err := stage.ApplyVerified(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(before.Root, "sub", "second.go")); err != nil {
		t.Fatal(err)
	}
	after, err := repositoryfacts.BuildGitSnapshot(t.Context(), before.Root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateExactRepositoryPostInventory(before, after, stage.ExpectedFiles()); err == nil ||
		!strings.Contains(err.Error(), "inventory") {
		t.Fatalf("unexpected unrelated deletion error=%v", err)
	}
}
