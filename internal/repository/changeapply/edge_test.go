package changeapply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestPlanReplacesMultipleTargetsInOneFileByExactDescendingRanges(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":  {content: "module example.com/samefile\n\ngo 1.24\n", mode: 0o600},
		"both.go": {content: "package samefile\n\nfunc First() int { return 1 }\n\nconst Retained = 7\n\nfunc Second() int { return 2 }\n", mode: 0o640},
	})
	contract := fixture.contract(t, "First", "Second")
	stage, err := fixture.plan(contract, []changeapply.CandidateDeclaration{
		{SymbolID: fixture.symbol(t, "Second").ID, Declaration: "func Second() int {\n\treturn 222\n}"},
		{SymbolID: fixture.symbol(t, "First").ID, Declaration: "func First() int { return 111 }"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	assertFile(
		t,
		filepath.Join(stage.Workspace(), "both.go"),
		"package samefile\n\nfunc First() int { return 111 }\n\nconst Retained = 7\n\nfunc Second() int {\n\treturn 222\n}\n",
		0o640,
	)
	if strings.Count(stage.Patch(), "diff --git") != 1 || strings.Count(stage.Patch(), "@@ ") != 2 {
		t.Fatalf("same-file patch did not contain one file and two bounded hunks:\n%s", stage.Patch())
	}
}

func TestApplyVerifiedRejectsTamperedStagingWorkspace(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	contract := fixture.contract(t, "First")
	stage, err := fixture.plan(contract, []changeapply.CandidateDeclaration{{
		SymbolID: fixture.symbol(t, "First").ID, Declaration: "func First() int { return 9 }",
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	if err := os.WriteFile(
		filepath.Join(stage.Workspace(), "first.go"),
		[]byte("package changeapply\n\nfunc First() int { return 900 }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.ApplyVerified(context.Background()); err == nil || !strings.Contains(err.Error(), "tampered") {
		t.Fatalf("tampered stage error=%v", err)
	}
	assertFile(t, filepath.Join(fixture.root, "first.go"), "package changeapply\n\nfunc First() int { return 1 }\n", 0o600)
}

func TestApplyVerifiedRejectsUnexpectedStagingInventory(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	contract := fixture.contract(t, "First")
	stage, err := fixture.plan(contract, []changeapply.CandidateDeclaration{{
		SymbolID: fixture.symbol(t, "First").ID, Declaration: "func First() int { return 9 }",
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	if err := os.Mkdir(filepath.Join(stage.Workspace(), "unexpected"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(stage.Workspace(), "unexpected", "generated.go"),
		[]byte("package unexpected\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.ApplyVerified(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "unexpected inventory") {
		t.Fatalf("unexpected stage inventory error=%v", err)
	}
	assertFile(t, filepath.Join(fixture.root, "first.go"), "package changeapply\n\nfunc First() int { return 1 }\n", 0o600)
}

func TestPlanRejectsSourceLayoutsTheTransactionalPatchEngineCannotPreserve(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, source, contains string
	}{
		{name: "CRLF", source: "package textlayout\r\n\r\nfunc Value() int { return 1 }\r\n", contains: "carriage-return"},
		{name: "no final LF", source: "package textlayout\n\nfunc Value() int { return 1 }", contains: "end with one LF"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newFixture(t, map[string]fixtureEntry{
				"go.mod":   {content: "module example.com/textlayout\n\ngo 1.24\n", mode: 0o600},
				"value.go": {content: test.source, mode: 0o600},
			})
			contract := fixture.contract(t, "Value")
			_, err := fixture.plan(contract, []changeapply.CandidateDeclaration{{
				SymbolID: fixture.symbol(t, "Value").ID, Declaration: "func Value() int { return 2 }",
			}})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("unsupported layout error=%v want %q", err, test.contains)
			}
		})
	}
}

func TestPlanRejectsSymlinkThatEscapesIsolatedStaging(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(fixture.root, "outside-link")); err != nil {
		t.Fatal(err)
	}
	fixture.refresh(t)
	contract := fixture.contract(t, "First")
	_, err := fixture.plan(contract, []changeapply.CandidateDeclaration{{
		SymbolID: fixture.symbol(t, "First").ID, Declaration: "func First() int { return 9 }",
	}})
	if err == nil || !strings.Contains(err.Error(), "absolute target") {
		t.Fatalf("escaping symlink error=%v", err)
	}
}
