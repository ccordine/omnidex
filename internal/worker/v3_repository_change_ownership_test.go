package worker

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestRepositoryGoCorrectionOwnershipUsesStagedAST(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	stage, err := changeapply.Plan(context.Background(), changeapply.Input{
		Snapshot: snapshot, Analysis: analysis, Contract: contract,
		Candidates: repositoryCorrectionDeclarations(t, contract, candidates),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	ownership, err := buildRepositoryGoCorrectionOwnership(
		snapshot, contract, candidates, stage.Workspace(),
	)
	if err != nil {
		t.Fatal(err)
	}
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	output := goTestJSONLines(
		goTestEvent{Action: "start", Package: "example.com/verification"},
		goTestEvent{Action: "output", Package: "example.com/verification", Output: "./first.go:3:29: undefined: Missing\n"},
		goTestEvent{Action: "fail", Package: "example.com/verification"},
	)
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	failure, err := classifyRepositoryGoVerificationFailure(
		commands[len(commands)-1], output, ownership,
	)
	if err != nil {
		t.Fatal(err)
	}
	if failure.targetSymbolID != first.ID || failure.diagnostic != "undefined: Missing" {
		t.Fatalf("failure=%+v", failure)
	}
	if strings.ContainsAny(failure.diagnostic, "/\\") || strings.Contains(failure.diagnostic, ".go:") {
		t.Fatalf("path-bearing diagnostic=%q", failure.diagnostic)
	}
}

func TestRepositoryGoFailureClassificationAcceptsRealCompilerJSON(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	contract, err := repositoryfacts.BuildChangeContract(
		snapshot, analysis, []repositoryfacts.ChangeRequest{{
			SymbolID: first.ID, RequirementQuote: "change First",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	candidates := map[string]string{first.ID: `func First() int { return "wrong" }`}
	stage, err := changeapply.Plan(context.Background(), changeapply.Input{
		Snapshot: snapshot, Analysis: analysis, Contract: contract,
		Candidates: repositoryCorrectionDeclarations(t, contract, candidates),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	ownership, err := buildRepositoryGoCorrectionOwnership(snapshot, contract, candidates, stage.Workspace())
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	broad := commands[len(commands)-1]
	command := exec.Command("go", broad.Args...)
	command.Dir = stage.Workspace()
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err == nil {
		t.Fatal("invalid staged Go declaration unexpectedly compiled")
	}
	failure, err := classifyRepositoryGoVerificationFailure(broad, stdout.String(), ownership)
	if err != nil {
		t.Fatalf("classify real compiler JSON: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if failure.targetSymbolID != first.ID || !strings.Contains(failure.diagnostic, "cannot use") {
		t.Fatalf("real compiler failure=%+v", failure)
	}
}

func TestRepositoryGoCorrectionOwnershipRejectsAmbiguousStagedDeclaration(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	contract.Targets = []repositoryfacts.ChangeTarget{repositoryCorrectionTarget(t, contract, first.ID)}
	candidates = map[string]string{first.ID: "func First() int { return 11 }"}
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "first.go"),
		[]byte("package verification\n\nfunc First() int { return 11 }\n\nfunc First() int { return 11 }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := buildRepositoryGoCorrectionOwnership(snapshot, contract, candidates, root); err == nil ||
		!strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous staged declaration error=%v", err)
	}
}

func TestRepositoryGoFailedFocusedTestUsesOnlyBoundTargetAndPathFreeOutput(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	stage, err := changeapply.Plan(context.Background(), changeapply.Input{
		Snapshot: snapshot, Analysis: analysis, Contract: contract,
		Candidates: repositoryCorrectionDeclarations(t, contract, candidates),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	ownership, err := buildRepositoryGoCorrectionOwnership(snapshot, contract, candidates, stage.Workspace())
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	output := goTestJSONLines(
		goTestEvent{Action: "start", Package: "example.com/verification"},
		goTestEvent{Action: "run", Package: "example.com/verification", Test: "TestFirst"},
		goTestEvent{Action: "output", Package: "example.com/verification", Test: "TestFirst", Output: "    first_test.go:5: got 11, want 1\n"},
		goTestEvent{Action: "fail", Package: "example.com/verification", Test: "TestFirst"},
		goTestEvent{Action: "fail", Package: "example.com/verification"},
	)
	failure, err := classifyRepositoryGoVerificationFailure(commands[0], output, ownership)
	if err != nil {
		t.Fatal(err)
	}
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	if failure.targetSymbolID != first.ID || failure.diagnostic != "got 11, want 1" {
		t.Fatalf("failure=%+v", failure)
	}
}

func TestRepositoryGoFailureClassificationRejectsMultipleExactDiagnostics(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	stage, err := changeapply.Plan(context.Background(), changeapply.Input{
		Snapshot: snapshot, Analysis: analysis, Contract: contract,
		Candidates: repositoryCorrectionDeclarations(t, contract, candidates),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	ownership, err := buildRepositoryGoCorrectionOwnership(snapshot, contract, candidates, stage.Workspace())
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	output := goTestJSONLines(
		goTestEvent{Action: "start", Package: "example.com/verification"},
		goTestEvent{Action: "output", Package: "example.com/verification", Output: "./first.go:3:29: undefined: Missing\n"},
		goTestEvent{Action: "output", Package: "example.com/verification", Output: "./first.go:3:37: undefined: Other\n"},
		goTestEvent{Action: "fail", Package: "example.com/verification"},
	)
	if _, err := classifyRepositoryGoVerificationFailure(
		commands[len(commands)-1], output, ownership,
	); err == nil || !strings.Contains(err.Error(), "multiple exact diagnostics") {
		t.Fatalf("multiple diagnostic error=%v", err)
	}
}

func TestRepositoryGoFailureClassificationRejectsUnownedAndAmbiguousFailures(t *testing.T) {
	t.Parallel()
	snapshot, analysis, contract, candidates := repositoryCorrectionFixture(t)
	stage, err := changeapply.Plan(context.Background(), changeapply.Input{
		Snapshot: snapshot, Analysis: analysis, Contract: contract,
		Candidates: repositoryCorrectionDeclarations(t, contract, candidates),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	ownership, err := buildRepositoryGoCorrectionOwnership(snapshot, contract, candidates, stage.Workspace())
	if err != nil {
		t.Fatal(err)
	}
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	second := existingRepositoryVerificationSymbol(t, analysis, "Second")
	targetIDs := []string{first.ID, second.ID}
	sort.Strings(targetIDs)
	ambiguous := testCommand{
		Family: "go", Name: "go",
		Args: []string{"test", "-json", "-count=1", "-run", "^TestBoth$", "."},
		RepositoryProof: &repositoryGoTestProof{
			Mode: repositoryGoProofFocused, Package: ".",
			Expected: []repositoryGoExpectedTest{{
				SymbolID: "test-both", Name: "TestBoth",
				TargetSymbolIDs: targetIDs,
			}},
		},
	}
	output := goTestJSONLines(
		goTestEvent{Action: "start", Package: "example.com/verification"},
		goTestEvent{Action: "run", Package: "example.com/verification", Test: "TestBoth"},
		goTestEvent{Action: "output", Package: "example.com/verification", Test: "TestBoth", Output: "    both_test.go:9: wrong value\n"},
		goTestEvent{Action: "fail", Package: "example.com/verification", Test: "TestBoth"},
		goTestEvent{Action: "fail", Package: "example.com/verification"},
	)
	if _, err := classifyRepositoryGoVerificationFailure(ambiguous, output, ownership); err == nil ||
		!strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguous failure error=%v", err)
	}
	broad := testCommand{
		Family: "go", Name: "go", Args: []string{"test", "-json", "-count=1", "./..."},
		RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofBroad, Package: "./..."},
	}
	if _, err := classifyRepositoryGoVerificationFailure(broad, output, ownership); err == nil ||
		!strings.Contains(err.Error(), "no exact contract target") {
		t.Fatalf("unowned failure error=%v", err)
	}
}

func repositoryCorrectionFixture(
	t *testing.T,
) (repositoryfacts.Snapshot, repositoryfacts.Analysis, repositoryfacts.ChangeContract, map[string]string) {
	t.Helper()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	requests := make([]repositoryfacts.ChangeRequest, 0, 2)
	candidates := make(map[string]string)
	for index, name := range []string{"First", "Second"} {
		symbol := existingRepositoryVerificationSymbol(t, analysis, name)
		requests = append(requests, repositoryfacts.ChangeRequest{SymbolID: symbol.ID, RequirementQuote: "change " + name})
		candidates[symbol.ID] = "func " + name + "() int { return " + strconv.Itoa(11+index) + " }"
	}
	contract, err := repositoryfacts.BuildChangeContract(snapshot, analysis, requests)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, analysis, contract, candidates
}

func repositoryCorrectionDeclarations(
	t *testing.T, contract repositoryfacts.ChangeContract, candidates map[string]string,
) []changeapply.CandidateDeclaration {
	t.Helper()
	declarations, err := exactRepositoryCandidateDeclarations(contract, candidates)
	if err != nil {
		t.Fatal(err)
	}
	return declarations
}

func repositoryCorrectionTarget(
	t *testing.T, contract repositoryfacts.ChangeContract, symbolID string,
) repositoryfacts.ChangeTarget {
	t.Helper()
	for _, target := range contract.Targets {
		if target.SymbolID == symbolID {
			return target
		}
	}
	t.Fatalf("contract lacks target %q", symbolID)
	return repositoryfacts.ChangeTarget{}
}
