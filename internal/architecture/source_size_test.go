package architecture

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const maximumProductionSourceLines = 800

func TestProductionSourceFilesDoNotBecomeGodFiles(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, sourceRoot := range []string{"cmd", "internal"} {
		root := filepath.Join(repositoryRoot, sourceRoot)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != root && ignoredSourceDirectory(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !isProductionSourceFile(entry.Name()) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			lines := bytes.Count(content, []byte{'\n'}) + 1
			if lines > maximumProductionSourceLines {
				relative, err := filepath.Rel(repositoryRoot, path)
				if err != nil {
					return err
				}
				t.Errorf("%s has %d lines; production source files must stay at or below %d", relative, lines, maximumProductionSourceLines)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s production source: %v", sourceRoot, err)
		}
	}
}

func ignoredSourceDirectory(name string) bool {
	return name == "dist" || name == "node_modules" || name == "testdata" || strings.HasPrefix(name, ".")
}

func isProductionSourceFile(name string) bool {
	if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.tsx") {
		return false
	}
	extension := filepath.Ext(name)
	return extension == ".go" || extension == ".ts" || extension == ".tsx"
}
