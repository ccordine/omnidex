package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestResolveAmbiguousArtifactDeletionSelectsSemanticCandidateIndependentOfPathOrder(t *testing.T) {
	t.Parallel()
	snapshot, analysis := ambiguousDeletionFixture(t)
	quote := "Remove whichever of ARTIFACT_1 or ARTIFACT_2 owns LegacyAdapter."
	var prompt string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			var err error
			prompt, err = assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: "ARTIFACT_CANDIDATE_2",
			}, nil
		},
	}
	directives, err := resolveAmbiguousArtifactDeletion(
		runtime, "semantic", []string{quote},
		[]assemblyline.ArtifactDirective{
			{Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactAbsenceCandidate},
			{Token: "ARTIFACT_2", Disposition: assemblyline.ArtifactAbsenceCandidate},
		},
		[]assemblyline.ArtifactIdentity{
			{Token: "ARTIFACT_1", Value: "legacy_adapter.go"},
			{Token: "ARTIFACT_2", Value: "current_adapter.go"},
		},
		snapshot, analysis,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(directives) != 2 || directives[0].Disposition != assemblyline.ArtifactForbid ||
		directives[1].Disposition != assemblyline.ArtifactReference {
		t.Fatalf("resolved directives=%+v", directives)
	}
	if !strings.Contains(prompt, `"candidate_id":"ARTIFACT_CANDIDATE_2"`) ||
		!strings.Contains(prompt, "LegacyAdapter") {
		t.Fatalf("semantic candidate did not sort second behind code-owned path order: %s", prompt)
	}
	for _, physical := range []string{"legacy_adapter.go", "current_adapter.go"} {
		if strings.Contains(prompt, physical) {
			t.Fatalf("candidate selector leaked physical identity %q: %s", physical, prompt)
		}
	}
	if !strings.Contains(prompt, "LegacyAdapter") || !strings.Contains(prompt, "CurrentAdapter") {
		t.Fatalf("candidate selector omitted bounded declaration evidence: %s", prompt)
	}
	graph, err := compileDesiredArtifactDeletion(
		"immutable front-door authority", []string{quote}, directives,
		[]assemblyline.ArtifactIdentity{
			{Token: "ARTIFACT_1", Value: "legacy_adapter.go"},
			{Token: "ARTIFACT_2", Value: "current_adapter.go"},
		},
		snapshot, analysis,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Artifacts) != 1 || graph.Artifacts[0].MustExist ||
		graph.Artifacts[0].RequirementQuote != quote {
		t.Fatalf("desired graph=%+v", graph)
	}
	legacy := existingRepositoryVerificationSymbol(t, analysis, "LegacyAdapter")
	if len(graph.Artifacts[0].ExistingSymbolIDs) != 1 ||
		graph.Artifacts[0].ExistingSymbolIDs[0] != legacy.ID {
		t.Fatalf("desired graph selected symbols=%v", graph.Artifacts[0].ExistingSymbolIDs)
	}
}

func TestResolveAmbiguousArtifactDeletionNoneFailsWithoutMutationFallback(t *testing.T) {
	t.Parallel()
	snapshot, analysis := ambiguousDeletionFixture(t)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: assemblyline.ArtifactCandidateSelectionNone,
			}, nil
		},
	}
	_, err := resolveAmbiguousArtifactDeletion(
		runtime, "semantic",
		[]string{"Remove whichever of ARTIFACT_1 or ARTIFACT_2 is obsolete."},
		ambiguousDeletionDirectives(), ambiguousDeletionIdentities(), snapshot, analysis,
	)
	if err == nil || !strings.Contains(err.Error(), "NONE") || calls != 1 {
		t.Fatalf("NONE selection error=%v calls=%d", err, calls)
	}
}

func TestResolveAmbiguousArtifactDeletionRejectsUnsafeSetBeforeInference(t *testing.T) {
	t.Parallel()
	snapshot, analysis := ambiguousDeletionFixture(t)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{}, nil
		},
	}
	for name, directives := range map[string][]assemblyline.ArtifactDirective{
		"one candidate": {
			{Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactAbsenceCandidate},
			{Token: "ARTIFACT_2", Disposition: assemblyline.ArtifactReference},
		},
		"mixed resolved deletion": {
			{Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactAbsenceCandidate},
			{Token: "ARTIFACT_2", Disposition: assemblyline.ArtifactForbid},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveAmbiguousArtifactDeletion(
				runtime, "semantic",
				[]string{"Remove whichever of ARTIFACT_1 or ARTIFACT_2 is obsolete."},
				directives, ambiguousDeletionIdentities(), snapshot, analysis,
			)
			if err == nil {
				t.Fatalf("unsafe set accepted")
			}
		})
	}
	if calls != 0 {
		t.Fatalf("unsafe candidate set reached inference %d times", calls)
	}
}

func TestResolveAmbiguousArtifactDeletionRejectsModelPhysicalAuthority(t *testing.T) {
	t.Parallel()
	snapshot, analysis := ambiguousDeletionFixture(t)
	for name, candidate := range map[string]string{
		"path":      `{"schema":"omnidex.artifact-candidate-selection.v1","candidate_id":"legacy_adapter.go"}`,
		"operation": `{"schema":"omnidex.artifact-candidate-selection.v1","candidate_id":"ARTIFACT_CANDIDATE_1","operation":"delete_file"}`,
	} {
		t.Run(name, func(t *testing.T) {
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			_, err := resolveAmbiguousArtifactDeletion(
				runtime, "semantic",
				[]string{"Remove whichever of ARTIFACT_1 or ARTIFACT_2 is obsolete."},
				ambiguousDeletionDirectives(), ambiguousDeletionIdentities(), snapshot, analysis,
			)
			if err == nil {
				t.Fatalf("model physical authority accepted: %s", candidate)
			}
		})
	}
}

func TestArtifactHandlingMapsOnlyPossibleAbsenceTruthToCandidateDisposition(t *testing.T) {
	t.Parallel()
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: string(assemblyline.ArtifactPossibleAbsenceCandidate),
			}, nil
		},
	}
	directives, err := classifyArtifactHandling(
		runtime, "semantic", "Remove either ARTIFACT_1 or ARTIFACT_2.",
		[]assemblyline.ArtifactIdentity{{Token: "ARTIFACT_1", Value: "legacy_adapter.go"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(directives) != 1 || directives[0].Disposition != assemblyline.ArtifactAbsenceCandidate {
		t.Fatalf("directives=%+v", directives)
	}
}

func ambiguousDeletionFixture(t *testing.T) (repositoryfacts.Snapshot, repositoryfacts.Analysis) {
	t.Helper()
	snapshot, _ := existingRepositoryVerificationFixture(t)
	for name, source := range map[string]string{
		"legacy_adapter.go":  "package verification\n\nfunc LegacyAdapter() int { return 1 }\n",
		"current_adapter.go": "package verification\n\nfunc CurrentAdapter() int { return 2 }\n",
	} {
		if err := os.WriteFile(filepath.Join(snapshot.Root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	desiredStateGit(t, snapshot.Root, "add", "legacy_adapter.go", "current_adapter.go")
	desiredStateGit(t, snapshot.Root, "commit", "-m", "ambiguous deletion candidates")
	return desiredStateReindex(t, t.Context(), snapshot.Root)
}

func ambiguousDeletionDirectives() []assemblyline.ArtifactDirective {
	return []assemblyline.ArtifactDirective{
		{Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactAbsenceCandidate},
		{Token: "ARTIFACT_2", Disposition: assemblyline.ArtifactAbsenceCandidate},
	}
}

func ambiguousDeletionIdentities() []assemblyline.ArtifactIdentity {
	return []assemblyline.ArtifactIdentity{
		{Token: "ARTIFACT_1", Value: "legacy_adapter.go"},
		{Token: "ARTIFACT_2", Value: "current_adapter.go"},
	}
}
