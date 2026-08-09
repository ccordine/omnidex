package worker

import (
	"context"
	"errors"
	"fmt"

	toolruntime "github.com/gryph/omnidex/internal/tools"
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
	call toolruntime.Call,
) (toolruntime.Result, error) {
	if environment == nil || environment.modules == nil || environment.root == "" {
		return toolruntime.Result{}, fmt.Errorf("repository Go verification environment is incomplete")
	}
	if root != environment.root {
		return toolruntime.Result{}, fmt.Errorf("repository Go verification environment belongs to a different source")
	}
	return executeRepositoryGoVerificationWithConfig(
		ctx, root, call, environment.config, environment.modules,
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
	call toolruntime.Call,
) (result toolruntime.Result, resultErr error) {
	environment, err := newRepositoryGoVerificationEnvironment(ctx, root)
	if err != nil {
		return toolruntime.Result{}, err
	}
	defer func() {
		if cleanupErr := environment.Cleanup(); cleanupErr != nil {
			result = toolruntime.Result{}
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()
	return environment.executeRepositoryGoVerification(ctx, root, call)
}
