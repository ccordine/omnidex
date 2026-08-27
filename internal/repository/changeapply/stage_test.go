package changeapply_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanStagesOnlyChangedPostImagesAndApplyVerifiedCommitsExactMultiFilePatch(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":       {content: "module example.com/changeapply\n\ngo 1.24\n", mode: 0o600},
		"first.go":     {content: "package changeapply\n\n// retained first\nfunc First() int { return 1 }\n", mode: 0o640},
		"second.go":    {content: "package changeapply\n\nfunc Second() int { return 2 }\n\n// retained second\n", mode: 0o600},
		"unchanged.md": {content: "unchanged bytes\n", mode: 0o644},
	})
	if err := os.Symlink("unchanged.md", filepath.Join(fixture.root, "unchanged-link")); err != nil {
		t.Fatal(err)
	}
	fixture.refresh(t)
	contract := fixture.contract(t, "First", "Second")
	candidates := map[string]string{
		fixture.symbol(t, "Second").ID: "func Second() int { return 22 }",
		fixture.symbol(t, "First").ID:  "func First() int { return 11 }",
	}
	stage, err := fixture.plan(contract, candidates)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })

	second, err := fixture.plan(contract, map[string]string{
		fixture.symbol(t, "First").ID:  candidates[fixture.symbol(t, "First").ID],
		fixture.symbol(t, "Second").ID: candidates[fixture.symbol(t, "Second").ID],
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Cleanup() })
	if stage.Patch() != second.Patch() {
		t.Fatalf("patch is not deterministic\nfirst:\n%s\nsecond:\n%s", stage.Patch(), second.Patch())
	}
	expectedFiles := stage.ExpectedFiles()
	if len(expectedFiles) != 2 {
		t.Fatalf("expected post-patch files=%+v", expectedFiles)
	}
	for _, expected := range expectedFiles {
		if expected.FileID == "" || len(expected.SHA256) != 64 || expected.Size < 1 {
			t.Fatalf("invalid expected post-patch file=%+v", expected)
		}
	}
	if _, err := os.Stat(filepath.Join(stage.DeltaRoot(), ".git")); !os.IsNotExist(err) {
		t.Fatalf("delta contains Git state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage.DeltaRoot(), ".omni")); !os.IsNotExist(err) {
		t.Fatalf("delta contains Omnidex state: %v", err)
	}
	for _, unchanged := range []string{"go.mod", "unchanged-link", "unchanged.md"} {
		if _, err := os.Lstat(filepath.Join(stage.DeltaRoot(), unchanged)); !os.IsNotExist(err) {
			t.Fatalf("delta copied unchanged path %q: %v", unchanged, err)
		}
	}
	assertFile(t, filepath.Join(stage.DeltaRoot(), "first.go"), "package changeapply\n\n// retained first\nfunc First() int { return 11 }\n", 0o640)
	assertFile(t, filepath.Join(stage.DeltaRoot(), "second.go"), "package changeapply\n\nfunc Second() int { return 22 }\n\n// retained second\n", 0o600)

	raw, err := json.Marshal(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), fixture.root) || strings.Contains(string(raw), "second.go") {
		t.Fatalf("model candidate leaked path authority: %s", raw)
	}
	result, err := stage.ApplyVerified(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("applied files=%+v", result.Files)
	}
	assertFile(t, filepath.Join(fixture.root, "first.go"), "package changeapply\n\n// retained first\nfunc First() int { return 11 }\n", 0o640)
	assertFile(t, filepath.Join(fixture.root, "second.go"), "package changeapply\n\nfunc Second() int { return 22 }\n\n// retained second\n", 0o600)
	assertFile(t, filepath.Join(fixture.root, "unchanged.md"), "unchanged bytes\n", 0o644)
	if _, err := stage.ApplyVerified(context.Background()); err == nil || !strings.Contains(err.Error(), "already applied") {
		t.Fatalf("second authoritative apply error=%v", err)
	}
}

func TestCleanupIsExplicitAndIdempotent(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	contract := fixture.contract(t, "First")
	stage, err := fixture.plan(contract, map[string]string{
		fixture.symbol(t, "First").ID: "func First() int { return 9 }",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := stage.DeltaRoot()
	if err := stage.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := stage.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("staging workspace survived cleanup: %v", err)
	}
	if _, err := stage.ApplyVerified(context.Background()); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("apply after cleanup error=%v", err)
	}
}

func assertFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != content {
		t.Fatalf("%s content=%q want %q", path, raw, content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode.Perm() {
		t.Fatalf("%s mode=%o want=%o", path, info.Mode().Perm(), mode.Perm())
	}
}
