package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/workspace"
)

type v3WorkspaceScopeContextKey struct{}

type v3WorkspaceScope struct {
	Scanner *workspace.Service
	Root    string
	Source  string
}

func codingWorkspaceForJob(job model.Job) string {
	if cwd := clientCWDForJob(job); strings.TrimSpace(cwd) != "" {
		return cwd
	}
	return metadataString(job.Metadata, "workspace")
}

func (s *Service) workspaceScopeForV3Job(job model.Job) (v3WorkspaceScope, error) {
	if s == nil || s.workspace == nil || !s.workspace.Enabled() {
		return v3WorkspaceScope{}, fmt.Errorf("workspace.research is disabled")
	}
	root := strings.TrimSpace(codingWorkspaceForJob(job))
	source := "job_metadata"
	if root == "" {
		root = strings.TrimSpace(s.workspace.Root())
		source = "worker_config"
	}
	if root == "" {
		return v3WorkspaceScope{}, fmt.Errorf("workspace.research requires an authoritative workspace root")
	}
	resolvedRoot, err := resolveV3WorkspaceRoot(s.workspace.Root(), s.workspaceHostRoot, root)
	if err != nil {
		return v3WorkspaceScope{}, fmt.Errorf("bind %s workspace %q: %w", source, root, err)
	}
	scanner, err := s.workspace.Scoped(resolvedRoot)
	if err != nil {
		return v3WorkspaceScope{}, fmt.Errorf("bind %s workspace %q: %w", source, root, err)
	}
	return v3WorkspaceScope{Scanner: scanner, Root: scanner.Root(), Source: source}, nil
}

func withV3WorkspaceScope(ctx context.Context, scope v3WorkspaceScope) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("workspace tool context is required")
	}
	if scope.Scanner == nil || strings.TrimSpace(scope.Root) == "" || strings.TrimSpace(scope.Source) == "" {
		return nil, fmt.Errorf("complete workspace scope is required")
	}
	return context.WithValue(ctx, v3WorkspaceScopeContextKey{}, scope), nil
}

func v3WorkspaceScopeFromContext(ctx context.Context) (v3WorkspaceScope, error) {
	if ctx == nil {
		return v3WorkspaceScope{}, fmt.Errorf("workspace tool context is required")
	}
	scope, ok := ctx.Value(v3WorkspaceScopeContextKey{}).(v3WorkspaceScope)
	if !ok || scope.Scanner == nil || strings.TrimSpace(scope.Root) == "" {
		return v3WorkspaceScope{}, fmt.Errorf("workspace.research requires a server-authoritative job workspace scope")
	}
	return scope, nil
}
