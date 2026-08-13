package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type v3WorkspaceScope struct {
	Root   string
	Source string
}

func codingWorkspaceForJob(job model.Job) string {
	if cwd := clientCWDForJob(job); strings.TrimSpace(cwd) != "" {
		return cwd
	}
	return metadataString(job.Metadata, "workspace")
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
