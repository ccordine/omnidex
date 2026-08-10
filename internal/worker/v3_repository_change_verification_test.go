package worker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
)

func TestExistingRepositoryGoVerificationCommandsUseExactTargetAndTestPackages(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	requests := make([]repositoryfacts.ChangeRequest, 0, 2)
	for _, name := range []string{"First", "Second"} {
		requests = append(requests, repositoryfacts.ChangeRequest{
			SymbolID:         existingRepositoryVerificationSymbol(t, analysis, name).ID,
			RequirementQuote: "change " + name,
		})
	}
	contract, err := repositoryfacts.BuildChangeContract(snapshot, analysis, requests)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	firstTarget := existingRepositoryVerificationSymbol(t, analysis, "First").ID
	secondTarget := existingRepositoryVerificationSymbol(t, analysis, "Second").ID
	want := []testCommand{
		{
			Family: "go", Name: "go",
			Args: []string{"test", "-json", "-count=1", "-run", "^TestFirst$", "."},
			RepositoryProof: &repositoryGoTestProof{
				Mode: repositoryGoProofFocused, Package: ".",
				Expected: []repositoryGoExpectedTest{{
					SymbolID:        existingRepositoryVerificationSymbol(t, analysis, "TestFirst").ID,
					Name:            "TestFirst",
					TargetSymbolIDs: []string{firstTarget},
				}},
			},
		},
		{
			Family: "go", Name: "go",
			Args: []string{"test", "-json", "-count=1", "-run", "^TestSecond$", "./sub"},
			RepositoryProof: &repositoryGoTestProof{
				Mode: repositoryGoProofFocused, Package: "./sub",
				Expected: []repositoryGoExpectedTest{{
					SymbolID:        existingRepositoryVerificationSymbol(t, analysis, "TestSecond").ID,
					Name:            "TestSecond",
					TargetSymbolIDs: []string{secondTarget},
				}},
			},
		},
		{
			Family: "go", Name: "go",
			Args:            []string{"test", "-json", "-count=1", "./..."},
			RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofBroad, Package: "./..."},
		},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("verification commands=%+v want=%+v", commands, want)
	}
}

func TestExistingRepositoryGoVerificationCommandsRejectTargetWithoutDirectTest(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	analysis.Edges = removeRepositoryTestEdgesTo(analysis.Edges, first.ID)
	if err := repositoryfacts.FinalizeAnalysis(&analysis); err != nil {
		t.Fatal(err)
	}
	contract, err := repositoryfacts.BuildChangeContract(snapshot, analysis, []repositoryfacts.ChangeRequest{{
		SymbolID: first.ID, RequirementQuote: "change First",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract); err == nil ||
		!strings.Contains(err.Error(), "no direct verification") {
		t.Fatalf("missing direct verification error=%v", err)
	}
}

func TestFocusedGoVerificationProofRequiresEveryExactTestPass(t *testing.T) {
	t.Parallel()
	proof := repositoryGoTestProof{
		Mode: repositoryGoProofFocused, Package: ".",
		Expected: []repositoryGoExpectedTest{
			{SymbolID: "symbol-one", Name: "TestOne"},
			{SymbolID: "symbol-two", Name: "TestTwo"},
		},
	}
	valid := goTestJSONLines(
		goTestEvent{Action: "start", Package: "example.test/proof"},
		goTestEvent{Action: "run", Package: "example.test/proof", Test: "TestOne"},
		goTestEvent{Action: "pass", Package: "example.test/proof", Test: "TestOne"},
		goTestEvent{Action: "run", Package: "example.test/proof", Test: "TestTwo"},
		goTestEvent{Action: "run", Package: "example.test/proof", Test: "TestTwo/subcase"},
		goTestEvent{Action: "pass", Package: "example.test/proof", Test: "TestTwo/subcase"},
		goTestEvent{Action: "pass", Package: "example.test/proof", Test: "TestTwo"},
		goTestEvent{Action: "pass", Package: "example.test/proof"},
	)
	if err := validateRepositoryGoTestProof(proof, valid); err != nil {
		t.Fatal(err)
	}
	runOne := `{"Action":"run","Package":"example.test/proof","Test":"TestOne"}`
	duplicateRun := strings.Replace(valid, runOne, runOne+"\n"+runOne, 1)
	tests := []struct {
		name, output, contains string
	}{
		{name: "missing", output: goTestJSONLines(
			goTestEvent{Action: "start", Package: "example.test/proof"},
			goTestEvent{Action: "run", Package: "example.test/proof", Test: "TestOne"},
			goTestEvent{Action: "pass", Package: "example.test/proof", Test: "TestOne"},
			goTestEvent{Action: "pass", Package: "example.test/proof"},
		), contains: "TestTwo"},
		{name: "skipped", output: strings.Replace(valid, `"Action":"pass","Package":"example.test/proof","Test":"TestTwo"`, `"Action":"skip","Package":"example.test/proof","Test":"TestTwo"`, 1), contains: "skipped"},
		{name: "unexpected", output: strings.Replace(valid, `"Test":"TestTwo/subcase"`, `"Test":"TestOther"`, 1), contains: "unexpected"},
		{name: "malformed", output: valid + "not-json\n", contains: "malformed"},
		{name: "duplicate run", output: duplicateRun, contains: "more than once"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRepositoryGoTestProof(proof, test.output); err == nil ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("proof error=%v want %q", err, test.contains)
			}
		})
	}
}

func TestBroadGoVerificationProofRequiresStructuredPassingPackages(t *testing.T) {
	t.Parallel()
	proof := repositoryGoTestProof{Mode: repositoryGoProofBroad, Package: "./..."}
	valid := goTestJSONLines(
		goTestEvent{Action: "start", Package: "example.test/one"},
		goTestEvent{Action: "pass", Package: "example.test/one"},
		goTestEvent{Action: "start", Package: "example.test/two"},
		goTestEvent{Action: "skip", Package: "example.test/two"},
	)
	if err := validateRepositoryGoTestProof(proof, valid); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{
		"", "not-json\n",
		goTestJSONLines(goTestEvent{Action: "start", Package: "example.test/one"}),
		goTestJSONLines(
			goTestEvent{Action: "start", Package: "example.test/one"},
			goTestEvent{Action: "fail", Package: "example.test/one"},
		),
	} {
		if err := validateRepositoryGoTestProof(proof, output); err == nil {
			t.Fatalf("invalid broad output was accepted: %q", output)
		}
	}
}

func TestGoVerificationProofRejectsOutputPastExactEvidenceBound(t *testing.T) {
	t.Parallel()
	proof := repositoryGoTestProof{Mode: repositoryGoProofBroad, Package: "./..."}
	if err := validateRepositoryGoTestProof(
		proof, strings.Repeat("x", maxRepositoryGoVerificationStdoutBytes+1),
	); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("overflow proof error=%v", err)
	}
}

func removeRepositoryTestEdgesTo(edges []repositoryfacts.Edge, targetID string) []repositoryfacts.Edge {
	result := make([]repositoryfacts.Edge, 0, len(edges))
	for _, edge := range edges {
		if edge.Kind != "tests" || edge.ToID != targetID {
			result = append(result, edge)
		}
	}
	return result
}

func goTestJSONLines(events ...goTestEvent) string {
	var output strings.Builder
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			panic(err)
		}
		output.Write(raw)
		output.WriteByte('\n')
	}
	return output.String()
}

func existingRepositoryVerificationFixture(t *testing.T) (repositoryfacts.Snapshot, repositoryfacts.Analysis) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"empty/empty.go":     "package empty\n\nfunc Value() int { return 1 }\n",
		"go.mod":             "module example.com/verification\n\ngo 1.24\n",
		"first.go":           "package verification\n\nfunc First() int { return 1 }\n",
		"first_test.go":      "package verification\n\nimport \"testing\"\n\nfunc TestFirst(t *testing.T) { if First() != 1 { t.Fatal() } }\n",
		"sub/second.go":      "package sub\n\nfunc Second() int { return 2 }\n",
		"sub/second_test.go": "package sub\n\nimport \"testing\"\n\nfunc TestSecond(t *testing.T) { if Second() != 2 { t.Fatal() } }\n",
	}
	for relative, content := range files {
		absolute := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "verification@example.test"},
		{"config", "user.name", "Omnidex Test"}, {"add", "."}, {"commit", "-m", "fixture"},
	} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
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

func existingRepositoryVerificationSymbol(
	t *testing.T,
	analysis repositoryfacts.Analysis,
	name string,
) repositoryfacts.Symbol {
	t.Helper()
	for _, symbol := range analysis.Symbols {
		if symbol.Name == name {
			return symbol
		}
	}
	t.Fatalf("repository verification fixture lacks symbol %q", name)
	return repositoryfacts.Symbol{}
}
