package changeapply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestDesiredStateAssemblyRejectsMissingAndExtraCandidates(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	contract := fixture.contract(t, "First")
	firstID := fixture.symbol(t, "First").ID
	cases := []struct {
		name       string
		candidates map[string]string
		contains   string
	}{
		{name: "missing", contains: "0 declarations for 1"},
		{name: "extra", candidates: map[string]string{
			firstID:                             "func First() int { return 9 }",
			"symbol_" + strings.Repeat("a", 64): "func Extra() int { return 1 }",
		}, contains: "2 declarations for 1"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := fixture.plan(contract, test.candidates); err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error=%v want %q", err, test.contains)
			}
		})
	}
	secondFixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":    {content: "module example.com/missing\n\ngo 1.24\n", mode: 0o600},
		"first.go":  {content: "package missing\n\nfunc First() int { return 1 }\n", mode: 0o600},
		"second.go": {content: "package missing\n\nfunc Second() int { return 2 }\n", mode: 0o600},
	})
	multiContract := secondFixture.contract(t, "First", "Second")
	_, err := secondFixture.plan(multiContract, map[string]string{
		secondFixture.symbol(t, "First").ID: "func First() int { return 9 }",
		"symbol_" + strings.Repeat("b", 64): "func Extra() int { return 3 }",
	})
	if err == nil || !strings.Contains(err.Error(), "has no candidate declaration") {
		t.Fatalf("equal-count missing target error=%v", err)
	}
}

func TestDesiredStateAssemblyRejectsInvalidOversizedAndUnchangedCandidates(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	contract := fixture.contract(t, "First")
	first := fixture.symbol(t, "First")
	original, err := repositoryfacts.ReadExactSymbolSpan(fixture.snapshot, first, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, declaration, contains string
	}{
		{name: "invalid UTF-8", declaration: string([]byte{0xff}), contains: "UTF-8"},
		{name: "NUL", declaration: "func First() int {\x00return 9 }", contains: "NUL-free"},
		{name: "oversized", declaration: strings.Repeat("x", 64*1024+1), contains: "65536"},
		{name: "blank", declaration: "", contains: "non-empty"},
		{name: "changed signature", declaration: "func First() string { return \"nine\" }", contains: "signature"},
		{name: "compiler directive", declaration: "//go:noinline\nfunc First() int { return 9 }", contains: "comments"},
		{name: "unchanged", declaration: original.Content, contains: "unchanged"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := fixture.plan(contract, map[string]string{first.ID: test.declaration})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error=%v want %q", err, test.contains)
			}
		})
	}
}

func TestDesiredStateAssemblyRejectsStaleAndTamperedAuthority(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	contract := fixture.contract(t, "First")
	first := fixture.symbol(t, "First")
	tampered := contract
	tampered.Targets = append([]repositoryfacts.ChangeTarget(nil), contract.Targets...)
	tampered.Targets[0].ExpectedDeclarationSHA256 = strings.Repeat("0", 64)
	if _, err := fixture.plan(tampered, map[string]string{
		first.ID: "func First() int { return 9 }",
	}); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("tampered contract error=%v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.root, "first.go"), []byte("package changeapply\n\nfunc First() int { return 8 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.plan(contract, map[string]string{
		first.ID: "func First() int { return 9 }",
	}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale source error=%v", err)
	}
}

func TestDesiredStateAssemblyRejectsOverlappingAndProtectedTargets(t *testing.T) {
	t.Parallel()
	overlap := newFixture(t, map[string]fixtureEntry{
		"go.mod":  {content: "module example.com/overlap\n\ngo 1.24\n", mode: 0o600},
		"same.go": {content: "package overlap\n\nfunc Alpha() int { return 1 }\n", mode: 0o600},
	})
	content, err := os.ReadFile(filepath.Join(overlap.root, "same.go"))
	if err != nil {
		t.Fatal(err)
	}
	start := int64(strings.Index(string(content), "func Alpha"))
	end := int64(len(content) - 1)
	file := overlap.file(t, "same.go")
	adapter := repositoryfacts.AdapterIdentity{Name: "overlap-test", Version: "1"}
	analysis := repositoryfacts.Analysis{
		Schema: repositoryfacts.AnalysisSchemaV1, SnapshotID: overlap.snapshot.ID,
		Adapter: adapter, Complete: true, GeneratedAt: time.Now().UTC().Truncate(time.Microsecond),
		Symbols: []repositoryfacts.Symbol{
			repositoryfacts.NewSymbol(overlap.snapshot, file, adapter, "function", "Alpha", "overlap.Alpha", "func Alpha() int", start, end, repositoryfacts.OriginGoAST, 1),
			repositoryfacts.NewSymbol(overlap.snapshot, file, adapter, "function", "Alias", "overlap.Alias", "func Alias() int", start, end, repositoryfacts.OriginGoAST, 1),
		}, Artifacts: []repositoryfacts.Artifact{}, Edges: []repositoryfacts.Edge{}, Diagnostics: []repositoryfacts.AnalysisDiagnostic{},
	}
	if err := repositoryfacts.FinalizeAnalysis(&analysis); err != nil {
		t.Fatal(err)
	}
	contract, err := repositoryfacts.BuildChangeContract(overlap.snapshot, analysis, []repositoryfacts.ChangeRequest{
		{SymbolID: analysis.Symbols[0].ID, RequirementQuote: "change alpha"},
		{SymbolID: analysis.Symbols[1].ID, RequirementQuote: "change alias"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = changeapply.AssembleExistingGoFileStates(
		overlap.snapshot, analysis, contract, map[string]string{
			analysis.Symbols[0].ID: "func Alpha() int { return 2 }",
			analysis.Symbols[1].ID: "func Alias() int { return 2 }",
		},
	)
	if err == nil || (!strings.Contains(err.Error(), "overlap") && !strings.Contains(err.Error(), "duplicate range")) {
		t.Fatalf("overlap error=%v", err)
	}

	protected := newFixture(t, map[string]fixtureEntry{
		"go.mod":             {content: "module example.com/protected\n\ngo 1.24\n", mode: 0o600},
		"value.generated.go": {content: "package protected\n\nfunc Generated() int { return 1 }\n", mode: 0o600},
	})
	protectedContract := protected.contract(t, "Generated")
	_, err = protected.plan(protectedContract, map[string]string{
		protected.symbol(t, "Generated").ID: "func Generated() int { return 2 }",
	})
	if err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("protected target error=%v", err)
	}
}

func TestApplyVerifiedRejectsWorkspaceDriftWithoutPartialApply(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":    {content: "module example.com/rollback\n\ngo 1.24\n", mode: 0o600},
		"first.go":  {content: "package rollback\n\nfunc First() int { return 1 }\n", mode: 0o600},
		"second.go": {content: "package rollback\n\nfunc Second() int { return 2 }\n", mode: 0o600},
	})
	contract := fixture.contract(t, "First", "Second")
	stage, err := fixture.plan(contract, map[string]string{
		fixture.symbol(t, "First").ID:  "func First() int { return 11 }",
		fixture.symbol(t, "Second").ID: "func Second() int { return 22 }",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	if err := os.WriteFile(filepath.Join(fixture.root, "second.go"), []byte("package rollback\n\nfunc Second() int { return 200 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.ApplyVerified(context.Background()); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale apply error=%v", err)
	}
	assertFile(t, filepath.Join(fixture.root, "first.go"), "package rollback\n\nfunc First() int { return 1 }\n", 0o600)
	assertFile(t, filepath.Join(fixture.root, "second.go"), "package rollback\n\nfunc Second() int { return 200 }\n", 0o600)
}
