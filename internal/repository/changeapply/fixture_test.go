package changeapply_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

type fixtureEntry struct {
	content string
	mode    os.FileMode
}

type fixture struct {
	root     string
	snapshot repositoryfacts.Snapshot
	analysis repositoryfacts.Analysis
}

func basicFixture(t *testing.T) *fixture {
	t.Helper()
	return newFixture(t, map[string]fixtureEntry{
		"go.mod":   {content: "module example.com/basic\n\ngo 1.24\n", mode: 0o600},
		"first.go": {content: "package changeapply\n\nfunc First() int { return 1 }\n", mode: 0o600},
	})
}

func newFixture(t *testing.T, files map[string]fixtureEntry) *fixture {
	t.Helper()
	root := t.TempDir()
	for path, entry := range files {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(entry.content), entry.mode); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "init")
	value := &fixture{root: root}
	value.refresh(t)
	return value
}

func (fixture *fixture) refresh(t *testing.T) {
	t.Helper()
	runGit(t, fixture.root, "add", ".")
	command := exec.Command("git", "-C", fixture.root, "diff", "--cached", "--quiet")
	if err := command.Run(); err != nil {
		runGit(t, fixture.root, "-c", "user.name=Omnidex Test", "-c", "user.email=test@example.com", "commit", "-m", "fixture")
	}
	snapshot, analysis := exactFixtureFacts(t, fixture.root)
	fixture.snapshot = snapshot
	fixture.analysis = analysis
}

func exactFixtureFacts(t *testing.T, root string) (repositoryfacts.Snapshot, repositoryfacts.Analysis) {
	t.Helper()
	snapshot, err := repositoryfacts.BuildGitSnapshot(context.Background(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, analysis
}

func (fixture *fixture) contract(t *testing.T, names ...string) repositoryfacts.ChangeContract {
	t.Helper()
	requests := make([]repositoryfacts.ChangeRequest, len(names))
	for index, name := range names {
		requests[index] = repositoryfacts.ChangeRequest{SymbolID: fixture.symbol(t, name).ID, RequirementQuote: "change " + name}
	}
	contract, err := repositoryfacts.BuildChangeContract(fixture.snapshot, fixture.analysis, requests)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func (fixture *fixture) symbol(t *testing.T, name string) repositoryfacts.Symbol {
	t.Helper()
	for _, symbol := range fixture.analysis.Symbols {
		if symbol.Name == name {
			return symbol
		}
	}
	t.Fatalf("missing symbol %q", name)
	return repositoryfacts.Symbol{}
}

func (fixture *fixture) file(t *testing.T, path string) repositoryfacts.File {
	t.Helper()
	for _, file := range fixture.snapshot.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("missing file %q", path)
	return repositoryfacts.File{}
}

func (fixture *fixture) plan(contract repositoryfacts.ChangeContract, candidates []changeapply.CandidateDeclaration) (*changeapply.StagedChange, error) {
	return changeapply.Plan(context.Background(), changeapply.Input{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, Contract: contract, Candidates: candidates,
	})
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
