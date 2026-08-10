package cognitionenv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

type recordingBuilder struct {
	mu       sync.Mutex
	pack     repositoryretrieval.EvidencePack
	build    func(repositoryretrieval.Request) (repositoryretrieval.EvidencePack, error)
	err      error
	requests []repositoryretrieval.Request
}

func (builder *recordingBuilder) Build(
	_ context.Context,
	request repositoryretrieval.Request,
) (repositoryretrieval.EvidencePack, error) {
	builder.mu.Lock()
	defer builder.mu.Unlock()
	builder.requests = append(builder.requests, request)
	if builder.build != nil {
		return builder.build(request)
	}
	return builder.pack, builder.err
}

func testEnvironment(
	t *testing.T,
	investigation Investigation,
	builder EvidenceBuilder,
) (*Environment, cognition.EpisodeRef, cognition.AttemptRef) {
	t.Helper()
	episode, err := cognition.NewEpisodeRef("repository-episode-test")
	if err != nil {
		t.Fatal(err)
	}
	actor := cognition.AttemptRef{JobID: 1, Generation: 1, StepID: 2, Attempt: 1, WorkerID: "worker-test"}
	environment, err := NewEnvironment(
		investigation, episode, builder,
		func(_ context.Context, candidate cognition.AttemptRef) error {
			if candidate != actor {
				return fmt.Errorf("stale actor")
			}
			return nil
		},
		&memoryJournal{receipts: make(map[cognition.ActionID]cognition.EnvironmentReceipt)},
	)
	if err != nil {
		t.Fatal(err)
	}
	return environment, episode, actor
}

func testAction(
	t *testing.T,
	investigation Investigation,
	actor cognition.AttemptRef,
	evidence ...cognition.EvidenceRef,
) cognition.RegisteredAction {
	t.Helper()
	schema, exists := investigation.Catalog().Schema(ActionSearch)
	if !exists {
		t.Fatal("test investigation has no search action")
	}
	request, err := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{})
	if err != nil {
		t.Fatal(err)
	}
	action, err := cognition.NewRegisteredAction(
		"repository-action-test", actor, schema, request, evidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func testSymbolAction(
	t *testing.T,
	investigation Investigation,
	actor cognition.AttemptRef,
	kind cognition.ActionKind,
	symbolRef string,
	evidence ...cognition.EvidenceRef,
) cognition.RegisteredAction {
	t.Helper()
	schema, exists := investigation.Catalog().Schema(kind)
	if !exists {
		t.Fatalf("test investigation has no %s action", kind)
	}
	request, err := cognition.NewActionRequest(kind, []cognition.ActionArgument{{
		Name: ArgumentSymbolRef, Value: symbolRef,
	}})
	if err != nil {
		t.Fatal(err)
	}
	action, err := cognition.NewRegisteredAction(
		cognition.ActionID("repository-action-"+string(kind)), actor, schema, request, evidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func activeObligation(t *testing.T, investigation Investigation, generation int64) cognition.Obligation {
	t.Helper()
	graph, err := cognition.NewObligationGraph(1, "repository-root", []cognition.ObligationSpec{{
		ID: "repository-root", Desired: investigation.Goal(),
		DependsOn: []cognition.ObligationID{}, SupportingRefs: []cognition.EvidenceRef{},
		CompletionCheck: investigation.Completion().Check,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(uint64(generation)); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition("repository-root", uint64(generation), cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	value, found := graph.Obligation("repository-root")
	if !found {
		t.Fatal("active obligation is absent")
	}
	return value
}

func testInvestigation(
	t *testing.T,
	operation repositoryretrieval.Operation,
) (Investigation, repositoryfacts.Analysis, repositoryfacts.Snapshot) {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.invalid")
	runGit(t, root, "config", "user.name", "Omnidex Test")
	source := "package sample\n\nfunc Target() int { return 1 }\nfunc Caller() int { return Target() }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.invalid/sample\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "sample.go", "go.mod")
	runGit(t, root, "commit", "-m", "fixture")
	snapshot, err := repositoryfacts.BuildGitSnapshot(t.Context(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	need, err := NewNeedAuthority("Determine the declaration responsible for the target behavior.")
	if err != nil {
		t.Fatal(err)
	}
	investigation, err := NewInvestigation(41, snapshot, analysis, need, operation, "Target")
	if err != nil {
		t.Fatal(err)
	}
	return investigation, analysis, snapshot
}

func testPack(
	t *testing.T,
	investigation Investigation,
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
) repositoryretrieval.EvidencePack {
	t.Helper()
	var symbol repositoryfacts.Symbol
	for _, candidate := range analysis.Symbols {
		if candidate.Name == "Target" {
			symbol = candidate
			break
		}
	}
	if symbol.ID == "" {
		t.Fatal("Target symbol is absent")
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: snapshot.ID, AnalysisID: analysis.ID, Operation: investigation.operation,
		QueryBinding: investigation.queryBinding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: symbol.ID, Kind: symbol.Kind, Name: symbol.Name, Signature: symbol.Signature,
			SourceSHA256: symbol.SourceSHA256, Source: "func Target() int { return 1 }",
		}},
		Relations:       []repositoryretrieval.EvidenceRelation{},
		SourceOmissions: []repositoryretrieval.SourceOmission{}, OmittedSymbolIDs: []string{},
		MaxBytes: fixedLimits.MaxPackBytes,
	}
	if investigation.operation != repositoryretrieval.OperationSemanticExcerpts {
		pack.SubjectSymbolID = symbol.ID
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	return pack
}

func evidenceSymbolFileSize(
	t *testing.T,
	symbolID string,
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
) int64 {
	t.Helper()
	for _, symbol := range analysis.Symbols {
		if symbol.ID != symbolID {
			continue
		}
		for _, file := range snapshot.Files {
			if file.ID == symbol.FileID && file.Size > 0 {
				return file.Size
			}
		}
	}
	t.Fatal("test could not resolve the evidence symbol's exact file")
	return 0
}

func assertNoRepositoryPaths(t *testing.T, value any, snapshot repositoryfacts.Snapshot) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	forbiddenValues := []string{snapshot.Root, `"file_id"`, `"path"`}
	for _, file := range snapshot.Files {
		forbiddenValues = append(forbiddenValues, file.Path, file.ID)
	}
	for _, forbidden := range forbiddenValues {
		if strings.Contains(text, forbidden) {
			t.Fatalf("model-visible repository cognition payload leaked %q: %s", forbidden, text)
		}
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}
