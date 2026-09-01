package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptBrowserTargetTreeNeverInfersOwnershipFromExistingNames(t *testing.T) {
	fixtures := []struct {
		name    string
		entries []string
		want    []string
	}{
		{
			name:    "complete ordinary pair",
			entries: []string{"src/feature001.tsx", "src/feature001.test.tsx"},
			want:    []string{"src/feature002.test.tsx", "src/feature002.tsx"},
		},
		{
			name:    "partial ordinary pair",
			entries: []string{"src/feature001.tsx"},
			want:    []string{"src/feature002.test.tsx", "src/feature002.tsx"},
		},
		{
			name: "multiple ordinary pairs",
			entries: []string{
				"src/feature001.tsx", "src/feature001.test.tsx",
				"src/feature002.tsx", "src/feature002.test.tsx",
			},
			want: []string{"src/feature003.test.tsx", "src/feature003.tsx"},
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			for _, artifactPath := range fixture.entries {
				target := filepath.Join(root, filepath.FromSlash(artifactPath))
				if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
					t.Fatalf("create fixture directory: %v", err)
				}
				if err := os.WriteFile(target, []byte("fixture"), 0o600); err != nil {
					t.Fatalf("write fixture path %s: %v", artifactPath, err)
				}
			}
			target, err := testResolveBrowserTargetAtRoot(t, root)
			if err != nil {
				t.Fatalf("allocate beyond ordinary existing paths: %v", err)
			}
			if !sameExactStrings(target.Paths, fixture.want) {
				t.Fatalf("target=%v; want collision-free pair %v", target.Paths, fixture.want)
			}
			for _, artifactPath := range fixture.entries {
				content, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifactPath)))
				if readErr != nil || string(content) != "fixture" {
					t.Fatalf("ordinary existing path %s changed: %q, %v", artifactPath, content, readErr)
				}
			}
		})
	}
}

func TestTypeScriptBrowserTargetTreeAdvancesAtomicPairPastOccupiedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "feature001.tsx"), 0o700); err != nil {
		t.Fatalf("create occupied target directory: %v", err)
	}
	target, err := testResolveBrowserTargetAtRoot(t, root)
	if err != nil {
		t.Fatalf("resolve target after occupied half: %v", err)
	}
	want := []string{"src/feature002.test.tsx", "src/feature002.tsx"}
	if !sameExactStrings(target.Paths, want) {
		t.Fatalf("target=%v; want atomic advanced pair %v", target.Paths, want)
	}
}

func TestTypeScriptBrowserGreenfieldAuthorityPreservesOrdinaryExistingFiles(t *testing.T) {
	fixtures := []struct {
		name        string
		product     string
		requirement string
		existing    []string
		wantTarget  []string
	}{
		{
			name:        "maintenance tracker",
			product:     "A maintenance tracker",
			requirement: "Expose the current status of one scheduled maintenance task.",
			existing:    []string{"src/feature001.tsx", "src/feature001.test.tsx"},
			wantTarget:  []string{"src/feature002.test.tsx", "src/feature002.tsx"},
		},
		{
			name:        "text summarizer",
			product:     "A text summarizer",
			requirement: "Accept supplied text and expose one resulting summary.",
			existing:    []string{"src/reference.txt"},
			wantTarget:  []string{"src/feature001.test.tsx", "src/feature001.tsx"},
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			for _, relative := range fixture.existing {
				target := filepath.Join(root, filepath.FromSlash(relative))
				if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
					t.Fatalf("create ordinary parent: %v", err)
				}
				if err := os.WriteFile(target, []byte("user-owned"), 0o600); err != nil {
					t.Fatalf("write ordinary path %s: %v", relative, err)
				}
			}
			program := testTypeScriptBrowserProgramAtRoot(
				t, fixture.name, fixture.product, fixture.requirement, root,
			)
			if !sameExactStrings(program.TargetTree.Paths, fixture.wantTarget) {
				t.Fatalf("target=%v; want %v", program.TargetTree.Paths, fixture.wantTarget)
			}
			if err := validateDirectCodingTypeScriptGreenfieldProgramRoot(root, program); err != nil {
				t.Fatalf("ordinary unrelated state blocked greenfield allocation: %v", err)
			}
			for _, relative := range fixture.existing {
				content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
				if err != nil || string(content) != "user-owned" {
					t.Fatalf("ordinary path %s changed: %q, %v", relative, content, err)
				}
			}
		})
	}
}

func TestTypeScriptBrowserGreenfieldAuthorityRejectsUnownedCollisions(t *testing.T) {
	for _, relative := range []string{"package.json", "src/App.tsx", "node_modules/marker.txt"} {
		relative := relative
		t.Run(relative, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatalf("create collision parent: %v", err)
			}
			if err := os.WriteFile(target, []byte("retain exactly"), 0o600); err != nil {
				t.Fatalf("write collision: %v", err)
			}
			program := testTypeScriptBrowserProgramAtRoot(
				t,
				"greenfield collision",
				"A neutral browser utility",
				"Expose one observable status.",
				root,
			)
			if err := validateDirectCodingTypeScriptGreenfieldProgramRoot(root, program); err == nil {
				t.Fatal("unowned existing path unexpectedly received replacement authority")
			}
			content, err := os.ReadFile(target)
			if err != nil || string(content) != "retain exactly" {
				t.Fatalf("unowned collision changed: %q, %v", content, err)
			}
		})
	}
}

func testResolveBrowserTargetAtRoot(
	t *testing.T,
	root string,
) (assemblyline.TargetTree, error) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "Fixture product",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Expose one observable fixture result.",
		}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatalf("freeze fixture workload: %v", err)
	}
	stack, _ := testTypeScriptBrowserProject(t)
	occupation, err := snapshotDirectCodingTargetTreeOccupation(root, stack)
	if err != nil {
		return assemblyline.TargetTree{}, err
	}
	target, _, err := resolveDirectCodingTargetTree(
		specification, workload, stack, nil, occupation,
	)
	return target, err
}
