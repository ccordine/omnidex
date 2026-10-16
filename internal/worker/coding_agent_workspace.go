package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspace"
)

func (service *Service) nativeAgentWorkspaceContext(ctx context.Context, job model.Job, scope v3WorkspaceScope, identity string) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("native agent workspace requires an execution context")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := service.requireNativeAgentWorkspace(ctx, job, scope, identity); err != nil {
		return nil, nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	return lifetime, cancel, nil
}

func (service *Service) requireNativeAgentWorkspace(ctx context.Context, job model.Job, scope v3WorkspaceScope, identity string) error {
	if job.Pipeline != model.PipelineChat {
		current, err := service.workspaceScopeForV3Job(ctx, job)
		if err != nil {
			return err
		}
		if current != scope {
			return fmt.Errorf("native workspace differs from its immutable job binding")
		}
		return nil
	}
	configuration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return err
	}
	if configuration.Mode != codingagent.Agents {
		return fmt.Errorf("native client workspace requires explicit Agents mode")
	}
	current, err := workspaceAuthorityForV3Job(job)
	if err != nil {
		return err
	}
	if current != scope {
		return fmt.Errorf("native workspace differs from its immutable job binding")
	}
	_, directoryID, err := projectroot.ParseClientWorkspaceIdentity(scope.Identity)
	if err != nil {
		return err
	}
	physicalIdentity, err := projectroot.DirectoryIdentity(scope.Root)
	if err != nil {
		return err
	}
	if directoryID != physicalIdentity || identity != physicalIdentity {
		return fmt.Errorf("selected workspace belongs to another environment; use an agent registered in that environment")
	}
	// The approved job retains its exact physical-directory binding independently
	// of a connected CLI or a local CLI installation's configuration files.
	return workspace.NewHostDirectoryAccess(scope.Root).ValidateWorkspaceRoot(scope.Root)
}
