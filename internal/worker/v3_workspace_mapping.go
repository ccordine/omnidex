package worker

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveV3WorkspaceRoot maps the client-visible host root onto the one
// configured runtime mount. It never probes an arbitrary container path.
func resolveV3WorkspaceRoot(runtimeRoot, hostRoot, requested string) (string, error) {
	runtimeRoot = strings.TrimSpace(runtimeRoot)
	hostRoot = strings.TrimSpace(hostRoot)
	requested = strings.TrimSpace(requested)
	if runtimeRoot == "" || requested == "" {
		return "", fmt.Errorf("configured and requested workspace roots are required")
	}
	if !filepath.IsAbs(runtimeRoot) || !filepath.IsAbs(requested) {
		return "", fmt.Errorf("configured and requested workspace roots must be absolute")
	}

	runtimeRoot = filepath.Clean(runtimeRoot)
	requested = filepath.Clean(requested)
	candidate := ""
	if relative, inside := relativeWithin(runtimeRoot, requested); inside {
		candidate = filepath.Join(runtimeRoot, relative)
	} else if hostRoot != "" {
		if !filepath.IsAbs(hostRoot) {
			return "", fmt.Errorf("configured host workspace root must be absolute")
		}
		hostRoot = filepath.Clean(hostRoot)
		relative, inside := relativeWithin(hostRoot, requested)
		if inside {
			candidate = filepath.Join(runtimeRoot, relative)
		}
	}
	if candidate == "" {
		return "", fmt.Errorf("requested path is outside the configured workspace boundary")
	}

	realRuntimeRoot, err := filepath.EvalSymlinks(runtimeRoot)
	if err != nil {
		return "", fmt.Errorf("resolve configured workspace boundary: %w", err)
	}
	realCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve requested workspace: %w", err)
	}
	if _, inside := relativeWithin(realRuntimeRoot, realCandidate); !inside {
		return "", fmt.Errorf("requested path escapes the configured workspace boundary through a symlink")
	}
	return realCandidate, nil
}

func relativeWithin(root, candidate string) (string, bool) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}
