package worker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestDesiredArtifactGraphGoVerificationUsesExactPackagesThenBroadProof(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	graph := desiredVerificationGraph(t, snapshot, analysis, []repositoryfacts.DesiredGoArtifact{
		{
			RequirementQuote:  "The package must contain func Added() int.",
			PackageArtifactID: desiredVerificationPackageID(t, analysis, "verification"),
			Signature:         "func Added() int",
			MustExist:         true,
		},
		{
			RequirementQuote:  "The obsolete declaration must be absent.",
			PackageArtifactID: desiredVerificationPackageID(t, analysis, "sub"),
			MustExist:         false,
			ExistingSymbolIDs: []string{existingRepositoryVerificationSymbol(t, analysis, "Second").ID},
		},
	})
	commands, err := desiredArtifactGraphGoVerificationCommands(snapshot, analysis, graph)
	if err != nil {
		t.Fatal(err)
	}
	want := []testCommand{
		{
			Family: "go", Name: "go", Args: []string{"test", "-json", "-count=1", "-run", "^$", "."},
			Purpose:         verificationTest,
			RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofPackage, Package: "."},
		},
		{
			Family: "go", Name: "go", Args: []string{"test", "-json", "-count=1", "-run", "^$", "./sub"},
			Purpose:         verificationTest,
			RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofPackage, Package: "./sub"},
		},
		{
			Family: "go", Name: "go", Args: []string{"test", "-json", "-count=1", "./..."},
			Purpose:         verificationTest,
			RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofBroad, Package: "./..."},
		},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("desired verification commands=%+v want=%+v", commands, want)
	}
	baseline, err := newRepositoryBaselineVerificationAuthority(snapshot.ID, graph.ID, commands)
	if err != nil {
		t.Fatalf("desired graph baseline owner was rejected: %v", err)
	}
	metadata := baseline.metadata()
	if metadata["repository_desired_artifact_graph_id"] != graph.ID ||
		metadata["repository_mutation_owner_id"] != graph.ID {
		t.Fatalf("desired graph baseline metadata=%#v", metadata)
	}
}

func TestPackageGoVerificationProofRequiresExactPackagePass(t *testing.T) {
	t.Parallel()
	proof := repositoryGoTestProof{Mode: repositoryGoProofPackage, Package: "./sub"}
	valid := goTestJSONLines(
		goTestEvent{Action: "start", Package: "example.com/verification/sub"},
		goTestEvent{Action: "run", Package: "example.com/verification/sub", Test: "TestSecond"},
		goTestEvent{Action: "pass", Package: "example.com/verification/sub", Test: "TestSecond"},
		goTestEvent{Action: "pass", Package: "example.com/verification/sub"},
	)
	if err := validateRepositoryGoTestProof(proof, valid); err != nil {
		t.Fatal(err)
	}
	for name, output := range map[string]string{
		"skip": goTestJSONLines(
			goTestEvent{Action: "start", Package: "example.com/verification/sub"},
			goTestEvent{Action: "skip", Package: "example.com/verification/sub"},
		),
		"failure": goTestJSONLines(
			goTestEvent{Action: "start", Package: "example.com/verification/sub"},
			goTestEvent{Action: "fail", Package: "example.com/verification/sub", Test: "TestSecond"},
		),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRepositoryGoTestProof(proof, output); err == nil {
				t.Fatalf("invalid package proof was accepted: %s", output)
			}
		})
	}
}

func TestPackageGoVerificationCommandIsRegisteredCompileOnlyProof(t *testing.T) {
	t.Parallel()
	valid := testCommand{
		Family: "go", Name: "go",
		Args:    []string{"test", "-json", "-count=1", "-run", "^$", "./sub"},
		Purpose: verificationTest,
		RepositoryProof: &repositoryGoTestProof{
			Mode: repositoryGoProofPackage, Package: "./sub",
		},
	}
	if err := validateRepositoryGoTestCommand(valid); err != nil {
		t.Fatal(err)
	}
	if !registeredRepositoryGoTestArguments(valid.Args) {
		t.Fatalf("sandbox rejected exact package compile/typecheck args: %v", valid.Args)
	}
	for _, args := range [][]string{
		{"test", "-json", "-count=1", "./sub"},
		{"test", "-json", "-count=1", "-run", ".*", "./sub"},
		{"test", "-json", "-count=1", "-run", "^$", "./..."},
	} {
		candidate := valid
		candidate.Args = args
		if err := validateRepositoryGoTestCommand(candidate); err == nil {
			t.Fatalf("package proof accepted divergent args: %v", args)
		}
	}
}

func TestRepositoryExpectedPostAndStageAcceptExactPresenceTransitions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		fixture func(*testing.T) (repositoryfacts.Snapshot, repositoryfacts.Analysis)
		stage   func(*testing.T, repositoryfacts.Snapshot, repositoryfacts.Analysis) *changeapply.StagedChange
	}{
		{name: "create", fixture: existingRepositoryVerificationFixture, stage: desiredVerificationCreateStage},
		{name: "delete", fixture: desiredVerificationDeletionFixture, stage: desiredVerificationDeleteStage},
	} {
		name := test.name
		t.Run(name, func(t *testing.T) {
			snapshot, analysis := test.fixture(t)
			stage := test.stage(t, snapshot, analysis)
			t.Cleanup(func() { _ = stage.Cleanup() })
			if _, err := repositoryExpectedPostID(snapshot.ID, stage.ChangedFileIDs(), stage.ExpectedFiles()); err != nil {
				t.Fatal(err)
			}
			graph := desiredVerificationGraph(t, snapshot, analysis, []repositoryfacts.DesiredGoArtifact{{
				RequirementQuote:  "The repository inventory must reach its accepted desired state.",
				PackageArtifactID: desiredVerificationPackageID(t, analysis, "verification"),
				Signature:         "func AuthorityOnly() int",
				MustExist:         true,
			}})
			commands, err := desiredArtifactGraphGoVerificationCommands(snapshot, analysis, graph)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := newVerifiedRepositoryChangeStage(graph.ID, commands, stage)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateVerifiedRepositoryChangeStage(prepared); err != nil {
				t.Fatal(err)
			}
			if _, err := prepared.verificationAuthority(snapshot.ID, graph.ID, commands); err != nil {
				t.Fatalf("desired graph owner was rejected: %v", err)
			}
		})
	}
}

func TestRepositoryExpectedPostRejectsMalformedPresenceAuthority(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	stage := desiredVerificationCreateStage(t, snapshot, analysis)
	t.Cleanup(func() { _ = stage.Cleanup() })
	valid := stage.ExpectedFiles()[0]
	for name, mutate := range map[string]func(*changeapply.ExpectedFileState){
		"path": func(state *changeapply.ExpectedFileState) { state.Path = "../escape.go" },
		"absent content": func(state *changeapply.ExpectedFileState) {
			state.Present = false
		},
		"mode": func(state *changeapply.ExpectedFileState) { state.Mode = 0o1777 },
	} {
		t.Run(name, func(t *testing.T) {
			state := valid
			mutate(&state)
			if _, err := repositoryExpectedPostID(
				snapshot.ID, []string{state.FileID}, []changeapply.ExpectedFileState{state},
			); err == nil {
				t.Fatalf("malformed %s presence authority was accepted: %+v", name, state)
			}
		})
	}
}

func TestRefreshedRepositoryChangeAcceptsExactCreateAndDeleteOnly(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		fixture func(*testing.T) (repositoryfacts.Snapshot, repositoryfacts.Analysis)
		stage   func(*testing.T, repositoryfacts.Snapshot, repositoryfacts.Analysis) *changeapply.StagedChange
		delta   int
	}{
		{name: "create", fixture: existingRepositoryVerificationFixture, stage: desiredVerificationCreateStage, delta: 1},
		{name: "delete", fixture: desiredVerificationDeletionFixture, stage: desiredVerificationDeleteStage, delta: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			before, analysis := test.fixture(t)
			stage := test.stage(t, before, analysis)
			t.Cleanup(func() { _ = stage.Cleanup() })
			if _, err := stage.ApplyVerified(context.Background()); err != nil {
				t.Fatal(err)
			}
			after := existingRepositoryRefreshedIndex(t, before.Root)
			if got := len(after.Snapshot.Files) - len(before.Files); got != test.delta {
				t.Fatalf("inventory delta=%d want=%d", got, test.delta)
			}
			if err := validateRefreshedRepositoryChange(before, after, stage.ExpectedFiles()); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(before.Root, "unauthorized.txt"), []byte("extra\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			drifted := existingRepositoryRefreshedIndex(t, before.Root)
			if err := validateRefreshedRepositoryChange(before, drifted, stage.ExpectedFiles()); err == nil ||
				!strings.Contains(err.Error(), "inventory") {
				t.Fatalf("unauthorized inventory error=%v", err)
			}
		})
	}
}

func desiredVerificationCreateStage(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) *changeapply.StagedChange {
	t.Helper()
	stage, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: snapshot, Analysis: analysis,
		OwnerID: "desired_graph_" + strings.Repeat("1", 64),
		Desired: []changeapply.DesiredFileState{{
			Path: "added.go", Present: true, Mode: 0o644,
			Content:           []byte("package verification\n\nfunc Added() int { return 3 }\n"),
			PackageArtifactID: desiredVerificationPackageID(t, analysis, "verification"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stage
}

func desiredVerificationDeleteStage(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) *changeapply.StagedChange {
	t.Helper()
	value := existingRepositoryVerificationSymbol(t, analysis, "ObsoleteForVerification")
	file := exactRepositorySnapshotFile(t, snapshot, value.FileID)
	stage, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: snapshot, Analysis: analysis,
		OwnerID: "desired_graph_" + strings.Repeat("2", 64),
		Desired: []changeapply.DesiredFileState{{
			Path: file.Path, Present: false,
			Source: changeapply.ExactSourceFile{
				FileID: file.ID, SHA256: file.SHA256, Size: file.Size, Mode: file.Mode,
			},
			RemovedSymbolIDs: []string{value.ID},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stage
}

func desiredVerificationDeletionFixture(
	t *testing.T,
) (repositoryfacts.Snapshot, repositoryfacts.Analysis) {
	t.Helper()
	snapshot, _ := existingRepositoryVerificationFixture(t)
	if err := os.WriteFile(
		filepath.Join(snapshot.Root, "obsolete_verification.go"),
		[]byte("package verification\n\nfunc ObsoleteForVerification() int { return 9 }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	desiredStateGit(t, snapshot.Root, "add", "obsolete_verification.go")
	desiredStateGit(t, snapshot.Root, "commit", "-m", "obsolete verification fixture")
	return desiredStateReindex(t, t.Context(), snapshot.Root)
}

func desiredVerificationGraph(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	artifacts []repositoryfacts.DesiredGoArtifact,
) repositoryfacts.DesiredArtifactGraph {
	t.Helper()
	graph, err := repositoryfacts.NewDesiredArtifactGraph(
		snapshot, analysis, "objective-verification", artifacts,
	)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func desiredVerificationPackageID(
	t *testing.T,
	analysis repositoryfacts.Analysis,
	name string,
) string {
	t.Helper()
	for _, artifact := range analysis.Artifacts {
		if artifact.Kind == "go_package" && artifact.Detail["package_name"] == name {
			return artifact.ID
		}
	}
	t.Fatalf("package artifact %q not found", name)
	return ""
}
