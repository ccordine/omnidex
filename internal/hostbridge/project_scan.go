package hostbridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultScanMaxFiles = 1200

type ProjectWalkFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
	SHA256  string `json:"sha256"`
}

type ProjectWalkResult struct {
	Root      string            `json:"root"`
	Files     []ProjectWalkFile `json:"files"`
	Manifests map[string]string `json:"manifests,omitempty"`
}

func WalkProjectTree(path string, maxFiles int) (ProjectWalkResult, error) {
	abs, err := resolveScannableProjectPath(path)
	if err != nil {
		return ProjectWalkResult{}, err
	}
	if maxFiles <= 0 {
		maxFiles = defaultScanMaxFiles
	}

	result := ProjectWalkResult{
		Root:      abs,
		Files:     []ProjectWalkFile{},
		Manifests: map[string]string{},
	}

	err = filepath.WalkDir(abs, func(walkPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || walkPath == abs {
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
		if maxFiles > 0 && len(result.Files) >= maxFiles {
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(abs, walkPath)
		if err != nil {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		sum, err := hashProjectFile(walkPath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		result.Files = append(result.Files, ProjectWalkFile{
			Path:    rel,
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339Nano),
			SHA256:  sum,
		})
		if isProjectWalkManifest(rel) {
			result.Manifests[rel] = sum
		}
		return nil
	})
	if err != nil {
		return ProjectWalkResult{}, err
	}
	return result, nil
}

func WriteProjectArtifacts(root string, indexJSON, mapJSON []byte) (string, string, error) {
	abs, err := resolveScannableProjectPath(root)
	if err != nil {
		return "", "", err
	}
	omniDir := filepath.Join(abs, ".omni")
	if err := os.MkdirAll(omniDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create .omni directory: %w", err)
	}
	indexPath := filepath.Join(omniDir, "index.json")
	mapPath := filepath.Join(omniDir, "codebase-map.json")
	if err := os.WriteFile(indexPath, append(indexJSON, '\n'), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(mapPath, append(mapJSON, '\n'), 0o644); err != nil {
		return "", "", err
	}
	return indexPath, mapPath, nil
}

func ReadProjectMapFile(path string) ([]byte, string, error) {
	abs, err := resolveScannableProjectPath(path)
	if err != nil {
		return nil, "", err
	}
	mapPath := filepath.Join(abs, ".omni", "codebase-map.json")
	blob, err := os.ReadFile(mapPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, mapPath, nil
		}
		return nil, "", err
	}
	return blob, mapPath, nil
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
	if name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key") {
		return true
	}
	return false
}

func isProjectWalkManifest(path string) bool {
	switch filepath.Base(path) {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.mod", "go.sum", "Cargo.toml", "pyproject.toml":
		return true
	default:
		return false
	}
}

func hashProjectFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func decodeProjectMapBlob(blob []byte) (map[string]any, error) {
	if len(blob) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(blob, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func resolveScannableProjectPath(path string) (string, error) {
	workspace, err := resolveHostWorkspace(path)
	if err != nil {
		return "", err
	}
	if err := ensureBrowseAllowed(workspace, BrowseOptions{ExtraRoots: []string{workspace}}); err != nil {
		return "", err
	}
	return workspace, nil
}
