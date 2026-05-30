package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
	"github.com/gryph/omnidex/internal/queue"
)

func mapWorkspacePathForHostBridge(location string) (string, bool) {
	workspaceRoot := strings.TrimSpace(os.Getenv("WORKSPACE_ROOT"))
	hostRoot := strings.TrimSpace(os.Getenv("HOST_WORKSPACE_PATH"))
	if workspaceRoot == "" || hostRoot == "" {
		return "", false
	}
	if !filepath.IsAbs(hostRoot) {
		return "", false
	}

	workspaceRoot = filepath.Clean(workspaceRoot)
	location = filepath.Clean(strings.TrimSpace(location))
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

func hostBridgeBrowseCandidates(location string) []string {
	location = filepath.Clean(strings.TrimSpace(location))
	if location == "" {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	add := func(path string) {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}

	add(location)
	if mapped, ok := mapWorkspacePathForHostBridge(location); ok {
		// Prefer the host path first when core is using a container workspace mount.
		out = append([]string{mapped}, out...)
		return dedupePaths(out)
	}
	return out
}

func dedupePaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func resolveHostBridgeProjectPath(ctx context.Context, client *hostbridge.Client, raw string) (string, error) {
	location, err := queue.NormalizeProjectLocation(raw)
	if err != nil {
		return "", err
	}
	if client == nil {
		return "", fmt.Errorf("host bridge unavailable")
	}

	candidates := hostBridgeBrowseCandidates(location)
	if len(candidates) == 0 {
		return "", fmt.Errorf("project location is required")
	}

	var attempts []string
	var lastErr error
	for _, candidate := range candidates {
		result, err := client.Browse(ctx, candidate)
		if err == nil && result != nil && strings.TrimSpace(result.Path) != "" {
			return filepath.Clean(result.Path), nil
		}
		if err != nil {
			lastErr = err
			attempts = append(attempts, fmt.Sprintf("%s: %v", candidate, err))
		} else {
			attempts = append(attempts, fmt.Sprintf("%s: empty browse result", candidate))
		}
	}

	if strings.HasPrefix(location, filepath.Clean(strings.TrimSpace(os.Getenv("WORKSPACE_ROOT")))) &&
		strings.TrimSpace(os.Getenv("HOST_WORKSPACE_PATH")) == "" {
		return "", fmt.Errorf("project directory %q is mounted at WORKSPACE_ROOT in core but HOST_WORKSPACE_PATH is not set; set HOST_WORKSPACE_PATH to the host path mounted at /workspace", location)
	}

	if lastErr != nil {
		return "", fmt.Errorf("project directory is not reachable on the host (%s)", strings.Join(attempts, " | "))
	}
	return "", fmt.Errorf("project directory is not reachable on the host")
}
