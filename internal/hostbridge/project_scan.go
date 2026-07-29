package hostbridge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultScanMaxFiles = 1200

type ProjectWalkFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type ProjectWalkResult struct {
	Root      string            `json:"root"`
	Files     []ProjectWalkFile `json:"files"`
	Manifests []string          `json:"manifests,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

func WalkProjectTree(path string, maxFiles int) (ProjectWalkResult, error) {
	abs, err := resolveScannableProjectPath(path)
	if err != nil {
		return ProjectWalkResult{}, err
	}
	if maxFiles <= 0 {
		maxFiles = defaultScanMaxFiles
	}
	result := ProjectWalkResult{Root: abs, Files: []ProjectWalkFile{}, Manifests: []string{}}
	err = filepath.WalkDir(abs, func(walkPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk project path %q: %w", walkPath, walkErr)
		}
		if walkPath == abs {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if shouldSkipProjectWalkDir(name) || name == ".omni" {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipProjectWalkFile(name) {
			return nil
		}
		if len(result.Files) >= maxFiles {
			result.Truncated = true
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(abs, walkPath)
		if err != nil {
			return fmt.Errorf("resolve project-relative path for %q: %w", walkPath, err)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect project file %q: %w", walkPath, err)
		}
		rel = filepath.ToSlash(rel)
		result.Files = append(result.Files, ProjectWalkFile{Path: rel, Size: info.Size()})
		if isProjectWalkManifest(rel) {
			result.Manifests = append(result.Manifests, rel)
		}
		return nil
	})
	if err != nil {
		return ProjectWalkResult{}, err
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	sort.Strings(result.Manifests)
	return result, nil
}

func shouldSkipProjectWalkDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", "target", ".next", ".cache", "__pycache__":
		return true
	default:
		return strings.HasPrefix(name, ".") && name != "."
	}
}

func shouldSkipProjectWalkFile(name string) bool {
	return name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key")
}

func isProjectWalkManifest(path string) bool {
	switch filepath.Base(path) {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.mod", "go.sum", "Cargo.toml", "pyproject.toml":
		return true
	default:
		return false
	}
}

func resolveScannableProjectPath(path string) (string, error) {
	workspace, err := resolveHostWorkspace(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory")
	}
	return workspace, nil
}
