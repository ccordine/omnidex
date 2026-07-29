package omni

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// WorkspaceIndex is an ephemeral view of the current filesystem. It is never
// persisted or compared to an older scan; Git owns source history.
type WorkspaceIndex struct {
	Workspace    string                    `json:"workspace"`
	GeneratedAt  string                    `json:"generated_at"`
	Files        []WorkspaceIndexFile      `json:"files"`
	Manifests    []string                  `json:"manifests,omitempty"`
	PackageProbe DeterministicProjectProbe `json:"package_probe"`
	Truncated    bool                      `json:"truncated,omitempty"`
}

type WorkspaceIndexFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func BuildWorkspaceIndex(workspace string, maxFiles int) (WorkspaceIndex, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return WorkspaceIndex{}, fmt.Errorf("workspace path is required")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return WorkspaceIndex{}, fmt.Errorf("resolve workspace path %q: %w", workspace, err)
	}
	rootInfo, err := os.Stat(abs)
	if err != nil {
		return WorkspaceIndex{}, fmt.Errorf("inspect workspace %q: %w", abs, err)
	}
	if !rootInfo.IsDir() {
		return WorkspaceIndex{}, fmt.Errorf("workspace %q must be a directory", abs)
	}
	index := WorkspaceIndex{
		Workspace:   abs,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Files:       []WorkspaceIndexFile{},
		Manifests:   []string{},
	}
	err = filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk workspace path %q: %w", path, walkErr)
		}
		if path == abs {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if shouldSkipSnapshotDir(name) || name == ".omni" {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipIndexFile(name) {
			return nil
		}
		if maxFiles > 0 && len(index.Files) >= maxFiles {
			index.Truncated = true
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return fmt.Errorf("resolve workspace-relative path for %q: %w", path, err)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect workspace file %q: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		index.Files = append(index.Files, WorkspaceIndexFile{Path: rel, Size: info.Size()})
		if isWorkspaceManifest(rel) {
			index.Manifests = append(index.Manifests, rel)
		}
		return nil
	})
	if err != nil {
		return WorkspaceIndex{}, err
	}
	sort.Slice(index.Files, func(i, j int) bool { return index.Files[i].Path < index.Files[j].Path })
	sort.Strings(index.Manifests)
	probe, err := deterministicProjectProbe(abs)
	if err != nil {
		return WorkspaceIndex{}, err
	}
	index.PackageProbe = probe
	return index, nil
}

func shouldSkipIndexFile(name string) bool {
	return name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key")
}

func isWorkspaceManifest(path string) bool {
	switch filepath.Base(path) {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.mod", "go.sum", "Cargo.toml", "pyproject.toml":
		return true
	default:
		return false
	}
}
