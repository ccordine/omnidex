package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextKernelsDoNotImportRepositoryMemoryOrRuntimeAuthority(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"github.com/gryph/omnidex/internal/memory",
		"github.com/gryph/omnidex/internal/repository",
		"github.com/gryph/omnidex/internal/worker",
		"github.com/gryph/omnidex/internal/llm",
		"github.com/gryph/omnidex/internal/queue",
	}
	for _, packageName := range []string{"taskstate", "workingset", "contextbuilder"} {
		root := filepath.Clean(filepath.Join("..", packageName))
		if err := scanContextKernelImports(t, root, forbidden); err != nil {
			t.Fatalf("scan %s source: %v", packageName, err)
		}
	}
}

func scanContextKernelImports(t *testing.T, root string, forbidden []string) error {
	t.Helper()
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, dependency := range forbidden {
			if strings.Contains(string(raw), dependency) {
				t.Fatalf("context kernel source %s imports separate authority %s", path, dependency)
			}
		}
		return nil
	})
}

func TestBuildCodenamesDoNotBecomeRuntimePackages(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "..", "internal"))
	for _, name := range []string{"charmander", "charmeleon"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("build codename became runtime package %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect forbidden codename package %s: %v", path, err)
		}
	}
}
