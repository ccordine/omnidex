package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/operation"
)

type repositoryGoVerificationEnvironment struct {
	root    string
	config  repositoryGoSandboxConfig
	modules *repositoryGoModuleView
}

func newRepositoryGoVerificationEnvironment(
	ctx context.Context,
	root string,
) (*repositoryGoVerificationEnvironment, error) {
	config, err := discoverRepositoryGoSandbox()
	if err != nil {
		return nil, err
	}
	return newRepositoryGoVerificationEnvironmentWithConfig(ctx, root, config)
}

func newRepositoryGoVerificationEnvironmentWithConfig(
	ctx context.Context,
	root string,
	config repositoryGoSandboxConfig,
) (*repositoryGoVerificationEnvironment, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	modules, err := resolveRepositoryGoBuildList(ctx, root, config)
	if err != nil {
		return nil, err
	}
	view, err := projectRepositoryGoModuleView(ctx, root, config.ModuleCache, modules)
	if err != nil {
		return nil, err
	}
	return &repositoryGoVerificationEnvironment{root: root, config: config, modules: view}, nil
}

func (environment *repositoryGoVerificationEnvironment) executeRepositoryGoVerification(
	ctx context.Context,
	root string,
	request repositoryGoVerificationRequest,
) (operation.Result, error) {
	if environment == nil || environment.modules == nil || environment.root == "" {
		return operation.Result{}, fmt.Errorf("repository Go verification environment is incomplete")
	}
	if root != environment.root {
		return operation.Result{}, fmt.Errorf("repository Go verification environment belongs to a different source")
	}
	return executeRepositoryGoVerificationWithConfig(
		ctx, root, request, environment.config, environment.modules,
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
	root string,
	request repositoryGoVerificationRequest,
) (result operation.Result, resultErr error) {
	environment, err := newRepositoryGoVerificationEnvironment(ctx, root)
	if err != nil {
		return operation.Result{}, err
	}
	defer func() {
		if cleanupErr := environment.Cleanup(); cleanupErr != nil {
			result = operation.Result{}
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()
	return environment.executeRepositoryGoVerification(ctx, root, request)
}
