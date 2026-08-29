package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const omnidexModulePath = "github.com/gryph/omnidex/"

var removedLegacyCognitionPaths = []string{
	"cmd/cognition-gauntlet",
	"internal/cognition",
	"internal/cognitiongauntlet",
	"internal/cognitionpolicy",
	"internal/cognitionreference",
	"internal/cognitionreplay",
	"internal/cognitionruntime",
	"internal/cognitionstate",
	"internal/cognitionstore",
	"internal/cognitiontransport",
	"internal/labyrinth",
	"internal/repository/cognitionenv",
}

var removedLegacyCognitionReleaseArtifacts = []string{
	"docs/LABYRINTH_FIRST_RUN.md",
	"docs/LABYRINTH_GAUNTLET.md",
	"docs/LABYRINTH_PROMOTION_GATES.md",
	"docs/LABYRINTH_REPLAY.md",
}

func TestLegacyCognitionRuntimeIsAbsent(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range removedLegacyCognitionPaths {
		path := filepath.Join(repositoryRoot, filepath.FromSlash(relative))
		if _, err := os.Stat(path); err == nil {
			t.Errorf("rejected legacy cognition path remains: %s", relative)
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect rejected legacy cognition path %s: %v", relative, err)
		}
	}

}

func TestReleaseSurfaceHasNoLegacyCognitionGauntlet(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range removedLegacyCognitionReleaseArtifacts {
		path := filepath.Join(repositoryRoot, filepath.FromSlash(relative))
		if _, err := os.Stat(path); err == nil {
			t.Errorf("rejected legacy cognition release artifact remains: %s", relative)
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect rejected legacy cognition release artifact %s: %v", relative, err)
		}
	}

	for _, relative := range []string{"scripts/build-release.sh", "scripts/build-release-lib.sh"} {
		raw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read release source %s: %v", relative, err)
		}
		for _, forbidden := range []string{
			"cognition-gauntlet",
			"package_release_operator_runbook",
			"LABYRINTH_FIRST_RUN",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("release source %s retains rejected legacy cognition marker %q", relative, forbidden)
			}
		}
	}
}

func TestProductionHasNoLegacyCognitionImports(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, root := range []string{
		filepath.Join(repositoryRoot, "cmd"),
		filepath.Join(repositoryRoot, "internal"),
	} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range parsed.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				for _, relative := range removedLegacyCognitionPaths[1:] {
					forbidden := omnidexModulePath + relative
					if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
						t.Errorf("%s imports rejected legacy cognition package %q", path, importPath)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan production source under %s: %v", root, err)
		}
	}
}
