package hostbridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveHostWorkspace(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("workspace is required")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if mapped, ok := mapWorkspacePathForHost(abs); ok {
		if stat, err := os.Stat(mapped); err == nil && stat.IsDir() {
			return filepath.Clean(mapped), nil
		}
	}
	if stat, err := os.Stat(abs); err == nil && stat.IsDir() {
		return abs, nil
	}
	return "", fmt.Errorf("workspace %q is not an existing directory on the host", raw)
}

func mapWorkspacePathForHost(location string) (string, bool) {
	workspaceRoot := strings.TrimSpace(os.Getenv("WORKSPACE_ROOT"))
	hostRoot := strings.TrimSpace(os.Getenv("HOST_WORKSPACE_PATH"))
	if workspaceRoot == "" || hostRoot == "" || !filepath.IsAbs(hostRoot) {
		return "", false
	}
	workspaceRoot = filepath.Clean(workspaceRoot)
	location = filepath.Clean(location)
	hostRoot = filepath.Clean(hostRoot)
	rel, err := filepath.Rel(workspaceRoot, location)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	if rel == "." {
		return hostRoot, true
	}
	return filepath.Join(hostRoot, rel), true
}

func validateHostWorkspace(raw string) (string, error) {
	workspace, err := resolveHostWorkspace(raw)
	if err != nil {
		return "", err
	}
	if err := ensureBrowseAllowed(workspace, BrowseOptions{}); err != nil {
		return "", err
	}
	stat, err := os.Stat(workspace)
	if err != nil || !stat.IsDir() {
		return "", fmt.Errorf("workspace must be an existing directory on the host")
	}
	return workspace, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
