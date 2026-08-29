package worker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnexpectedProgramSourcesPreserveOnlyUnchangedInitialLeaves(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "data.json")
	if err := os.WriteFile(dataPath, []byte("{\"value\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	initial, err := snapshotDirectCodingInitialPaths(root, []string{"data.json"})
	if err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{StackID: genericTypeScriptBrowserAdapter}
	unexpected := func() []string {
		t.Helper()
		paths, err := directCodingUnexpectedProgramSources(
			root, map[string]string{}, map[string]struct{}{}, program, initial,
		)
		if err != nil {
			t.Fatal(err)
		}
		return paths
	}

	if paths := unexpected(); len(paths) != 0 {
		t.Fatalf("unchanged initial source paths=%v", paths)
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	extraPath := filepath.Join(root, "src", "extra.tsx")
	if err := os.WriteFile(extraPath, []byte("export const extra = true;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if paths := unexpected(); len(paths) != 1 || paths[0] != "src/extra.tsx" {
		t.Fatalf("new undeclared source paths=%v", paths)
	}
	if err := os.Remove(extraPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("{\"value\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if paths := unexpected(); len(paths) != 1 || paths[0] != "data.json" {
		t.Fatalf("modified omitted source paths=%v", paths)
	}
	if err := os.Remove(dataPath); err != nil {
		t.Fatal(err)
	}
	if paths := unexpected(); len(paths) != 1 || paths[0] != "data.json" {
		t.Fatalf("deleted omitted source paths=%v", paths)
	}
	paths, err := directCodingUnexpectedProgramSources(
		root,
		map[string]string{},
		map[string]struct{}{"data.json": {}},
		program,
		initial,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("code-authorized deleted source paths=%v", paths)
	}
}

func TestInitialPathSnapshotRequiresExactNormalizedUniquePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name    string
		paths   []string
		wantErr string
	}{
		{name: "not normalized", paths: []string{"./data.json"}, wantErr: "exactly normalized"},
		{name: "duplicate", paths: []string{"data.json", "data.json"}, wantErr: "duplicates"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := snapshotDirectCodingInitialPaths(root, testCase.paths)
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error=%v want substring %q", err, testCase.wantErr)
			}
		})
	}
}

func TestInitialPathAuthorityIsCapturedBeforeSemanticGenerationAndUsedAtVerification(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve initial-path test source")
	}
	directory := filepath.Dir(current)
	plan, err := os.ReadFile(filepath.Join(directory, "v3_coding_driver_plan.go"))
	if err != nil {
		t.Fatal(err)
	}
	planSource := string(plan)
	snapshotIndex := strings.Index(planSource, "snapshotDirectCodingInitialPaths(s.root, existingPaths)")
	semanticIndex := strings.Index(planSource, "runDirectCodingApplicationInterpreter(")
	if snapshotIndex < 0 || semanticIndex < 0 || snapshotIndex >= semanticIndex {
		t.Fatal("initial-path authority must be captured from the initial tree before semantic generation")
	}
	verification, err := os.ReadFile(filepath.Join(directory, "v3_coding_driver_verification.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(verification), "s.initialPaths") {
		t.Fatal("program workspace verification omitted captured initial-path authority")
	}
}
