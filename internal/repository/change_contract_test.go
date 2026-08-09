package repository_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
)

func TestChangeContractBindsTargetsCapabilitiesAndTestsWithoutPaths(t *testing.T) {
	t.Parallel()
	root, snapshot, analysis := changeContractFixture(t)
	value := changeContractSymbol(t, analysis, "Value")
	contract, err := repositoryfacts.BuildChangeContract(snapshot, analysis, []repositoryfacts.ChangeRequest{
		{SymbolID: value.ID, RequirementQuote: "return two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.Validate(snapshot, analysis); err != nil {
		t.Fatal(err)
	}
	if len(contract.Targets) != 1 || len(contract.Targets[0].DirectCapabilities) != 1 ||
		contract.Targets[0].DirectCapabilities[0].Name != "Helper" ||
		len(contract.Targets[0].VerificationSymbolIDs) != 1 {
		t.Fatalf("change contract=%+v", contract)
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{root, "value.go", "value_test.go"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("change contract leaked filesystem path %q: %s", forbidden, raw)
		}
	}
}

func TestChangeContractRejectsStaleSourceAndUnsupportedTarget(t *testing.T) {
	t.Parallel()
	root, snapshot, analysis := changeContractFixture(t)
	value := changeContractSymbol(t, analysis, "Value")
	contract, err := repositoryfacts.BuildChangeContract(snapshot, analysis, []repositoryfacts.ChangeRequest{
		{SymbolID: value.ID, RequirementQuote: "return two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "value.go"), []byte("package sample\n\nfunc Value() int { return 3 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := contract.Validate(snapshot, analysis); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale contract error=%v", err)
	}

	_, cleanSnapshot, cleanAnalysis := changeContractFixture(t)
	variable := changeContractSymbol(t, cleanAnalysis, "Count")
	if _, err := repositoryfacts.BuildChangeContract(cleanSnapshot, cleanAnalysis, []repositoryfacts.ChangeRequest{
		{SymbolID: variable.ID, RequirementQuote: "increase count"},
	}); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("variable target error=%v", err)
	}
	cleanValue := changeContractSymbol(t, cleanAnalysis, "Value")
	if _, err := repositoryfacts.BuildChangeContract(cleanSnapshot, cleanAnalysis, []repositoryfacts.ChangeRequest{
		{SymbolID: cleanValue.ID, RequirementQuote: strings.Repeat("x", 513)},
	}); err == nil || !strings.Contains(err.Error(), "at most 512 bytes") {
		t.Fatalf("oversized requirement error=%v", err)
	}
}

func TestChangeContractFailsWhenDirectCapabilitiesExceedModelBoundary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/overflow\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	source.WriteString("package overflow\n\n")
	for index := 0; index < 33; index++ {
		source.WriteString("func Helper" + strconv.Itoa(index) + "() int { return 1 }\n")
	}
	source.WriteString("\nfunc Value() int { return ")
	for index := 0; index < 33; index++ {
		if index > 0 {
			source.WriteString(" + ")
		}
		source.WriteString("Helper" + strconv.Itoa(index) + "()")
	}
	source.WriteString(" }\n")
	if err := os.WriteFile(filepath.Join(root, "overflow.go"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	changeContractGit(t, root, "init")
	changeContractGit(t, root, "add", ".")
	changeContractGit(t, root, "-c", "user.name=Omnidex Test", "-c", "user.email=test@example.com", "commit", "-m", "fixture")
	snapshot, err := repositoryfacts.BuildGitSnapshot(context.Background(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	value := changeContractSymbol(t, analysis, "Value")
	if _, err := repositoryfacts.BuildChangeContract(snapshot, analysis, []repositoryfacts.ChangeRequest{
		{SymbolID: value.ID, RequirementQuote: "return the combined value"},
	}); err == nil || !strings.Contains(err.Error(), "direct capabilities exceed") {
		t.Fatalf("capability overflow error=%v", err)
	}
}

func changeContractFixture(t *testing.T) (string, repositoryfacts.Snapshot, repositoryfacts.Analysis) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/sample\n\ngo 1.24\n",
		"value.go": `package sample

var Count = 1

func Helper() int { return Count }

func Value() int { return Helper() }
`,
		"value_test.go": `package sample

import "testing"

func TestValue(t *testing.T) {
	if Value() != 1 { t.Fatal("unexpected value") }
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	changeContractGit(t, root, "init")
	changeContractGit(t, root, "add", ".")
	changeContractGit(t, root, "-c", "user.name=Omnidex Test", "-c", "user.email=test@example.com", "commit", "-m", "fixture")
	snapshot, err := repositoryfacts.BuildGitSnapshot(context.Background(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return root, snapshot, analysis
}

func changeContractSymbol(t *testing.T, analysis repositoryfacts.Analysis, name string) repositoryfacts.Symbol {
	t.Helper()
	for _, symbol := range analysis.Symbols {
		if symbol.Name == name {
			return symbol
		}
	}
	t.Fatalf("analysis has no symbol named %q", name)
	return repositoryfacts.Symbol{}
}

func changeContractGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
