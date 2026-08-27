package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/operation"
)

type repositoryGoVerificationEnvironment struct {
	projectionID string
	sourceRoot   string
	config       repositoryGoSandboxConfig
	modules      *repositoryGoModuleView
}

func newRepositoryGoVerificationEnvironment(
	ctx context.Context,
	projection repositoryWorkspaceProjection,
) (*repositoryGoVerificationEnvironment, error) {
	config, err := discoverRepositoryGoSandbox()
	if err != nil {
		return nil, err
	}
	return newRepositoryGoVerificationEnvironmentWithConfig(ctx, projection, config)
}

func newRepositoryGoVerificationEnvironmentWithConfig(
	ctx context.Context,
	projection repositoryWorkspaceProjection,
	config repositoryGoSandboxConfig,
) (*repositoryGoVerificationEnvironment, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := projection.VerifyExact(ctx); err != nil {
		return nil, err
	}
	root := projection.source.Root
	modules, err := resolveRepositoryGoBuildList(ctx, root, config)
	if err != nil {
		return nil, err
	}
	view, err := projectRepositoryGoModuleView(ctx, root, config.ModuleCache, modules)
	if err != nil {
		return nil, err
	}
	return &repositoryGoVerificationEnvironment{
		projectionID: projection.id, sourceRoot: root, config: config, modules: view,
	}, nil
}

func (environment *repositoryGoVerificationEnvironment) executeRepositoryGoVerification(
	ctx context.Context,
	projection repositoryWorkspaceProjection,
	request repositoryGoVerificationRequest,
) (operation.Result, error) {
	if environment == nil || environment.modules == nil || environment.projectionID == "" || environment.sourceRoot == "" {
		return operation.Result{}, fmt.Errorf("repository Go verification environment is incomplete")
	}
	if projection.id != environment.projectionID || projection.source.Root != environment.sourceRoot {
		return operation.Result{}, fmt.Errorf("repository Go verification environment belongs to a different projection")
	}
	return executeRepositoryGoVerificationWithConfig(
		ctx, projection, request, environment.config, environment.modules,
	)
}

func (environment *repositoryGoVerificationEnvironment) Cleanup() error {
	if environment == nil {
		return nil
	}
	return environment.modules.Cleanup()
}

func executeRepositoryGoVerification(
	ctx context.Context,
	projection repositoryWorkspaceProjection,
	request repositoryGoVerificationRequest,
) (result operation.Result, resultErr error) {
	environment, err := newRepositoryGoVerificationEnvironment(ctx, projection)
	if err != nil {
		return operation.Result{}, err
	}
	defer func() {
		if cleanupErr := environment.Cleanup(); cleanupErr != nil {
			result = operation.Result{}
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()
	return environment.executeRepositoryGoVerification(ctx, projection, request)
}
