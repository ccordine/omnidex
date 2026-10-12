package worker

import (
	"context"
	"fmt"
	"os"

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
	if job.Pipeline == model.PipelineChat {
		return service.workspaceConnections.Lifetime(ctx, scope.Root, scope.Identity)
	}
	lifetime, cancel := context.WithCancel(ctx)
	return lifetime, cancel, nil
}

func (service *Service) requireNativeAgentWorkspace(ctx context.Context, job model.Job, scope v3WorkspaceScope, identity string) error {
	if job.Pipeline != model.PipelineChat {
		return service.requireHostWorkspaceRoot(scope.Root)
	}
	configuration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return err
	}
	if configuration.Mode != codingagent.Agents {
		return fmt.Errorf("native client workspace requires explicit Agents mode")
	}
	current, err := service.workspaceScopeForV3Job(ctx, job)
	if err != nil {
		return err
	}
	if current != scope {
		return fmt.Errorf("native workspace differs from its immutable job binding")
	}
	clientID, directoryID, err := projectroot.ParseClientWorkspaceIdentity(scope.Identity)
	if err != nil {
		return err
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	localID, err := projectroot.ReadClientInstallationIdentity(configRoot)
	if err != nil {
		return fmt.Errorf("native agent requires the existing local client installation: %w", err)
	}
	physicalIdentity, err := projectroot.DirectoryIdentity(scope.Root)
	if err != nil {
		return err
	}
	if clientID != localID || directoryID != physicalIdentity || identity != physicalIdentity {
		return fmt.Errorf("selected workspace belongs to another environment; use an agent registered in that environment")
	}
	// The connected local CLI grants only this physical directory. Direct jobs
	// retain the configured host boundary, and the CLI still owns task writes
	// through its existing transport mutation fence.
	return workspace.NewHostDirectoryAccess(scope.Root).ValidateWorkspaceRoot(scope.Root)
}
