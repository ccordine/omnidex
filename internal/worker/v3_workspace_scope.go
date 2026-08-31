package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type v3WorkspaceScope struct {
	Root string
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
	if s == nil {
		return v3WorkspaceScope{}, fmt.Errorf("workspace service is unavailable")
	}
	root := strings.TrimSpace(codingWorkspaceForJob(job))
	if root == "" {
		return v3WorkspaceScope{}, fmt.Errorf("workspace boundary requires an authoritative job root")
	}
	resolvedRoot, err := resolveV3WorkspaceRoot(root)
	if err != nil {
		return v3WorkspaceScope{}, fmt.Errorf("bind job workspace %q: %w", root, err)
	}
	return v3WorkspaceScope{Root: resolvedRoot}, nil
}
