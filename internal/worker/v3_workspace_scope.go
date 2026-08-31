package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type v3WorkspaceScope struct {
	Root   string
	Source string
}

func resolveV3WorkspaceFile(root, relative string) (string, error) {
	normalized, err := normalizeDirectCodingPath(relative)
	if err != nil {
		return "", err
	}
	if normalized != relative {
		return "", fmt.Errorf("workspace path %q must be exactly normalized as %q", relative, normalized)
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", fmt.Errorf("workspace file requires one canonical absolute root")
	}
	return filepath.Join(root, filepath.FromSlash(normalized)), nil
}

func codingWorkspaceForJob(job model.Job) string {
	return clientCWDForJob(job)
}

func (s *Service) workspaceScopeForV3Job(job model.Job) (v3WorkspaceScope, error) {
	if s == nil || strings.TrimSpace(s.workspaceRoot) == "" {
		return v3WorkspaceScope{}, fmt.Errorf("workspace boundary is not configured")
	}
	root := strings.TrimSpace(codingWorkspaceForJob(job))
	if root == "" {
		return v3WorkspaceScope{}, fmt.Errorf("workspace boundary requires an authoritative job root")
	}
	source := "job_metadata"
	resolvedRoot, err := resolveV3WorkspaceRoot(s.workspaceRoot, s.workspaceHostRoot, root)
	if err != nil {
		return v3WorkspaceScope{}, fmt.Errorf("bind %s workspace %q: %w", source, root, err)
	}
	return v3WorkspaceScope{Root: resolvedRoot, Source: source}, nil
}
