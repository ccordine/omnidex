package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelGauntletCannotRouteProductionWork(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, sourceRoot := range []string{"internal", "cmd"} {
		root := filepath.Join(repositoryRoot, sourceRoot)
		err := walkProductionGo(root, func(path string, raw []byte) error {
			relative, err := filepath.Rel(repositoryRoot, path)
			if err != nil {
				return err
			}
			if strings.HasPrefix(relative, filepath.Join("internal", "modelgauntlet")) ||
				strings.HasPrefix(relative, filepath.Join("cmd", "cli")) {
				return nil
			}
			if strings.Contains(string(raw), "github.com/gryph/omnidex/internal/modelgauntlet") {
				t.Fatalf("production source %s imports the offline model gauntlet", relative)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s production source: %v", sourceRoot, err)
		}
	}
}

func walkProductionGo(root string, inspect func(string, []byte) error) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return inspect(path, raw)
	})
}
