package worker

import (
	"fmt"
	"path/filepath"
	"strings"
)

type directCodingFileTask struct {
	Path     string
	Content  []byte
	Mode     uint32
	MoveFrom string
}

type directCodingAssembly struct {
	Files         []directCodingFileTask
	RequiredPaths []string
	DeletePaths   []string
}

func requireExactDirectCodingPath(raw string) (string, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || strings.Contains(raw, "\\") || filepath.IsAbs(raw) {
		return "", fmt.Errorf("coding path %q must be a non-empty relative slash path", raw)
	}
	normalized := filepath.ToSlash(filepath.Clean(raw))
	if normalized != raw || normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("coding path %q must be one exact canonical relative path", raw)
	}
	return raw, nil
}
