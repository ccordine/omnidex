package worker

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	maxDirectCodingAssemblyUnits = 32
)

type directCodingFileTask struct {
	Path    string
	Content string
}

func (t *directCodingFileTask) normalize() error {
	path, err := normalizeDirectCodingPath(t.Path)
	if err != nil {
		return err
	}
	t.Path = path
	if t.Content == "" {
		return fmt.Errorf("code-owned source %s is empty", t.Path)
	}
	if len(t.Content) > maxV3WriteBytes {
		return fmt.Errorf("server-owned content for %s exceeds the %d-byte limit", t.Path, maxV3WriteBytes)
	}
	return nil
}

func (t directCodingFileTask) validate() error {
	copy := t
	return copy.normalize()
}

type directCodingAssembly struct {
	VersionProfileID string
	Directories      []string
	Files            []directCodingFileTask
	DeletePaths      []string
}

func (a *directCodingAssembly) normalize() error {
	if a.VersionProfileID == "" || a.VersionProfileID != strings.TrimSpace(a.VersionProfileID) ||
		strings.ContainsAny(a.VersionProfileID, "\x00\r\n") {
		return fmt.Errorf("coding assembly requires one normalized version profile ID")
	}
	if len(a.Files) > maxDirectCodingAssemblyUnits {
		return fmt.Errorf("coding assembly exceeds the %d-source-unit limit", maxDirectCodingAssemblyUnits)
	}
	if len(a.Directories) == 0 && len(a.Files) == 0 && len(a.DeletePaths) == 0 {
		return fmt.Errorf("coding assembly requires at least one mutation")
	}
	directories := make([]string, 0, len(a.Directories))
	seenDirectories := make(map[string]struct{}, len(a.Directories))
	for index, raw := range a.Directories {
		path, err := normalizeDirectCodingPath(raw)
		if err != nil {
			return fmt.Errorf("coding directory %d: %w", index, err)
		}
		if _, duplicate := seenDirectories[path]; duplicate {
			return fmt.Errorf("coding assembly repeats directory %s", path)
		}
		seenDirectories[path] = struct{}{}
		directories = append(directories, path)
	}
	a.Directories = directories
	seen := map[string]string{}
	for index := range a.Files {
		if err := a.Files[index].normalize(); err != nil {
			return fmt.Errorf("coding assembly unit %d: %w", index, err)
		}
		path := a.Files[index].Path
		if previous, exists := seen[path]; exists {
			return fmt.Errorf("coding assembly repeats path %s as %s and source", path, previous)
		}
		seen[path] = "source"
	}
	if len(a.DeletePaths) > maxDirectCodingAssemblyUnits {
		return fmt.Errorf("coding assembly exceeds the %d-delete limit", maxDirectCodingAssemblyUnits)
	}
	deletes := make([]string, 0, len(a.DeletePaths))
	for index, raw := range a.DeletePaths {
		path, err := normalizeDirectCodingPath(raw)
		if err != nil {
			return fmt.Errorf("coding delete path %d: %w", index, err)
		}
		if previous, exists := seen[path]; exists {
			return fmt.Errorf("coding assembly repeats path %s as %s and delete", path, previous)
		}
		seen[path] = "delete"
		deletes = append(deletes, path)
	}
	a.DeletePaths = deletes
	return nil
}

func (a directCodingAssembly) validate() error {
	copy := a
	return copy.normalize()
}

func normalizeDirectCodingPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "\\") || filepath.IsAbs(raw) {
		return "", fmt.Errorf("coding path %q must be a non-empty relative slash path", raw)
	}
	path := filepath.ToSlash(filepath.Clean(raw))
	if path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return "", fmt.Errorf("coding path %q escapes the authoritative workspace", raw)
	}
	return path, nil
}
