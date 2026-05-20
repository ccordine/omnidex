package omni

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type WorkspaceIndex struct {
	Version      string                    `json:"version"`
	Workspace    string                    `json:"workspace"`
	GeneratedAt  string                    `json:"generated_at"`
	Files        []WorkspaceIndexFile      `json:"files"`
	Manifests    map[string]string         `json:"manifests,omitempty"`
	PackageProbe DeterministicProjectProbe `json:"package_probe"`
	Update       WorkspaceIndexUpdate      `json:"update,omitempty"`
}

type WorkspaceIndexFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
	SHA256  string `json:"sha256"`
}

type WorkspaceIndexUpdate struct {
	PreviousFiles int `json:"previous_files,omitempty"`
	CurrentFiles  int `json:"current_files,omitempty"`
	ReusedHashes  int `json:"reused_hashes,omitempty"`
	RehashedFiles int `json:"rehashed_files,omitempty"`
	AddedFiles    int `json:"added_files,omitempty"`
	RemovedFiles  int `json:"removed_files,omitempty"`
}

func BuildWorkspaceIndex(workspace string, maxFiles int) (WorkspaceIndex, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = workspacePathOrCurrentDir()
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return WorkspaceIndex{}, err
	}
	index := WorkspaceIndex{
		Version:      "1.0",
		Workspace:    abs,
		GeneratedAt:  nowUTC(),
		Manifests:    map[string]string{},
		PackageProbe: deterministicProjectProbe(abs),
	}
	err = filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || path == abs {
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
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		index.Files = append(index.Files, WorkspaceIndexFile{Path: rel, Size: info.Size(), ModTime: info.ModTime().UTC().Format(time.RFC3339Nano), SHA256: sum})
		if isWorkspaceManifest(rel) {
			index.Manifests[rel] = sum
		}
		return nil
	})
	if err != nil {
		return WorkspaceIndex{}, err
	}
	sort.Slice(index.Files, func(i, j int) bool { return index.Files[i].Path < index.Files[j].Path })
	return index, nil
}

func UpdateWorkspaceIndex(workspace, existingPath string, maxFiles int) (WorkspaceIndex, error) {
	previous, err := ReadWorkspaceIndex(existingPath)
	if err != nil {
		return BuildWorkspaceIndex(workspace, maxFiles)
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = previous.Workspace
	}
	if workspace == "" {
		workspace = workspacePathOrCurrentDir()
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return WorkspaceIndex{}, err
	}
	previousByPath := map[string]WorkspaceIndexFile{}
	for _, file := range previous.Files {
		previousByPath[file.Path] = file
	}
	index := WorkspaceIndex{
		Version:      "1.0",
		Workspace:    abs,
		GeneratedAt:  nowUTC(),
		Manifests:    map[string]string{},
		PackageProbe: deterministicProjectProbe(abs),
		Update: WorkspaceIndexUpdate{
			PreviousFiles: len(previous.Files),
		},
	}
	seen := map[string]struct{}{}
	err = filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || path == abs {
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
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = struct{}{}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		modTime := info.ModTime().UTC().Format(time.RFC3339Nano)
		file := WorkspaceIndexFile{Path: rel, Size: info.Size(), ModTime: modTime}
		if previousFile, ok := previousByPath[rel]; ok && previousFile.Size == file.Size && previousFile.ModTime == file.ModTime && previousFile.SHA256 != "" {
			file.SHA256 = previousFile.SHA256
			index.Update.ReusedHashes++
		} else {
			sum, err := fileSHA256(path)
			if err != nil {
				return nil
			}
			file.SHA256 = sum
			index.Update.RehashedFiles++
			if _, existed := previousByPath[rel]; !existed {
				index.Update.AddedFiles++
			}
		}
		index.Files = append(index.Files, file)
		if isWorkspaceManifest(rel) {
			index.Manifests[rel] = file.SHA256
		}
		return nil
	})
	if err != nil {
		return WorkspaceIndex{}, err
	}
	for path := range previousByPath {
		if _, ok := seen[path]; !ok {
			index.Update.RemovedFiles++
		}
	}
	index.Update.CurrentFiles = len(index.Files)
	sort.Slice(index.Files, func(i, j int) bool { return index.Files[i].Path < index.Files[j].Path })
	return index, nil
}

func shouldSkipIndexFile(name string) bool {
	if name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key") {
		return true
	}
	return false
}

func WriteWorkspaceIndex(index WorkspaceIndex, outputPath string) error {
	target := strings.TrimSpace(outputPath)
	if target == "" {
		target = filepath.Join(index.Workspace, ".omni", "index.json")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create index directory: %w", err)
	}
	blob, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspace index: %w", err)
	}
	return os.WriteFile(target, append(blob, '\n'), 0o644)
}

func ReadWorkspaceIndex(path string) (WorkspaceIndex, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceIndex{}, err
	}
	var index WorkspaceIndex
	if err := json.Unmarshal(blob, &index); err != nil {
		return WorkspaceIndex{}, fmt.Errorf("decode workspace index: %w", err)
	}
	return index, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func isWorkspaceManifest(path string) bool {
	switch filepath.Base(path) {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.mod", "go.sum", "Cargo.toml", "pyproject.toml":
		return true
	default:
		return false
	}
}
