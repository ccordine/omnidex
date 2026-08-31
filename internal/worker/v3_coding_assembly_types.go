package worker

import (
	"fmt"
	"path/filepath"
	"strings"
)

type directCodingFileTask struct {
	Path    string
	Content []byte
	Mode    uint32
}

func (t *directCodingFileTask) normalize() error {
	path, err := normalizeDirectCodingPath(t.Path)
	if err != nil {
		return err
	}
	t.Path = path
	if t.Mode&^uint32(0o777) != 0 {
		return fmt.Errorf("code-owned source %s has invalid permission bits", t.Path)
	}
	return nil
}

func (t directCodingFileTask) validate() error {
	copy := t
	return copy.normalize()
}

type directCodingAssembly struct {
	VersionProfileID string
	Files            []directCodingFileTask
	DeletePaths      []string
}

func (a *directCodingAssembly) normalize() error {
	if a.VersionProfileID != "" && (a.VersionProfileID != strings.TrimSpace(a.VersionProfileID) ||
		strings.ContainsAny(a.VersionProfileID, "\x00\r\n")) {
		return fmt.Errorf("coding assembly version profile ID is not normalized")
	}
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
	if raw == "" || raw != strings.TrimSpace(raw) || strings.Contains(raw, "\\") || filepath.IsAbs(raw) {
		return "", fmt.Errorf("coding path %q must be a non-empty relative slash path", raw)
	}
	path := filepath.ToSlash(filepath.Clean(raw))
	if path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return "", fmt.Errorf("coding path %q escapes the authoritative workspace", raw)
	}
	return path, nil
}
