package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelGauntletCannotRouteProductionWork(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.HasPrefix(relative, filepath.Join("internal", "modelgauntlet")) ||
			strings.HasPrefix(relative, filepath.Join("cmd", "cli")) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "github.com/gryph/omnidex/internal/modelgauntlet") {
			t.Fatalf("production source %s imports the offline model gauntlet", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production source: %v", err)
	}
}
