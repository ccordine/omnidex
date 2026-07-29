package worker

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func directCodingWorkspaceHasImplementation(
	root string,
	protected map[string]directCodingProtectedPath,
) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor", "dist", "build", "target":
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, isProtected := protected[relative]; isProtected {
			return nil
		}
		if isDirectCodingSourcePath(relative) || directCodingImplementationManifest(relative) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("inspect existing coding workspace: %w", err)
	}
	return found, nil
}

func directCodingImplementationManifest(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "go.mod", "cargo.toml", "package.json", "pyproject.toml", "requirements.txt", "composer.json", "pom.xml", "build.gradle", "build.gradle.kts":
		return true
	default:
		return strings.HasSuffix(base, ".csproj")
	}
}
