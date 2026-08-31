package worker

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/gryph/omnidex/internal/model"
)

type v3WorkspaceScope struct {
	Root string
}

func codingWorkspaceForJob(job model.Job) (string, error) {
	if len(job.Metadata) == 0 {
		return "", fmt.Errorf("workspace boundary requires job metadata")
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return "", fmt.Errorf("decode workspace job metadata: %w", err)
	}
	raw, exists := metadata["client_cwd"]
	if !exists {
		return "", fmt.Errorf("workspace boundary requires client_cwd")
	}
	var root string
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", fmt.Errorf("workspace boundary client_cwd must be a string: %w", err)
	}
	return root, nil
}

func (s *Service) workspaceScopeForV3Job(job model.Job) (v3WorkspaceScope, error) {
	if s == nil {
		return v3WorkspaceScope{}, fmt.Errorf("workspace service is unavailable")
	}
	root, err := codingWorkspaceForJob(job)
	if err != nil {
		return v3WorkspaceScope{}, err
	}
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return v3WorkspaceScope{}, fmt.Errorf(
			"bind job workspace %q: root must be one canonical absolute path", root,
		)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return v3WorkspaceScope{}, fmt.Errorf("resolve authoritative workspace root %q: %w", root, err)
	}
	if !filepath.IsAbs(resolvedRoot) || filepath.Clean(resolvedRoot) != resolvedRoot {
		return v3WorkspaceScope{}, fmt.Errorf(
			"resolved workspace root %q is not one canonical absolute path", resolvedRoot,
		)
	}
	return v3WorkspaceScope{Root: resolvedRoot}, nil
}
