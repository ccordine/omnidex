package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type repositoryGoModuleView struct {
	mu          sync.Mutex
	root        string
	sourceRoot  string
	buildListID string
	tree        repositoryVerificationTree
	limits      repositoryGoModuleViewLimits
	closed      bool
}

func projectRepositoryGoModuleView(
	ctx context.Context,
	sourceRoot string,
	hostCache string,
	modules []repositoryGoResolvedModule,
) (*repositoryGoModuleView, error) {
	return projectRepositoryGoModuleViewWithLimits(
		ctx, sourceRoot, hostCache, modules, defaultRepositoryGoModuleViewLimits(),
	)
}

func projectRepositoryGoModuleViewWithLimits(
	ctx context.Context,
	sourceRoot string,
	hostCache string,
	modules []repositoryGoResolvedModule,
	limits repositoryGoModuleViewLimits,
) (*repositoryGoModuleView, error) {
	if ctx == nil {
		return nil, fmt.Errorf("project repository Go module view requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("project repository Go module view: %w", err)
	}
	if _, err := exactRepositorySandboxDirectory(sourceRoot, "repository module-view source"); err != nil {
		return nil, err
	}
	if _, err := exactRepositorySandboxDirectory(hostCache, "host Go module cache source"); err != nil {
		return nil, err
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	identity, err := repositoryGoBuildListIdentity(modules)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "omnidex-repository-module-view-*")
	if err != nil {
		return nil, fmt.Errorf("create repository Go module view: %w", err)
	}
	if err := projectRepositoryGoCachedModules(ctx, root, hostCache, modules, limits); err != nil {
		return nil, errors.Join(err, removeRepositoryGoModuleView(root))
	}
	tree, err := captureRepositoryVerificationTreeContextWithLimits(
		ctx, root, limits.MaxEntries, limits.MaxRegularBytes,
	)
	if err != nil {
		return nil, errors.Join(err, removeRepositoryGoModuleView(root))
	}
	return &repositoryGoModuleView{
		root: root, sourceRoot: sourceRoot, buildListID: identity, tree: tree, limits: limits,
	}, nil
}

func (view *repositoryGoModuleView) Root() string {
	if view == nil {
		return ""
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return ""
	}
	return view.root
}

func (view *repositoryGoModuleView) requireSource(root string) error {
	if view == nil {
		return fmt.Errorf("repository Go verification requires one exact module view")
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed || view.root == "" || view.sourceRoot == "" || view.buildListID == "" {
		return fmt.Errorf("repository Go module view is closed or incomplete")
	}
	if root != view.sourceRoot {
		return fmt.Errorf("repository Go module view belongs to a different source projection")
	}
	return nil
}

func (view *repositoryGoModuleView) VerifyExact(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("verify repository Go module view requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify repository Go module view: %w", err)
	}
	if view == nil {
		return fmt.Errorf("verify repository Go module view requires one view")
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed || view.root == "" || view.buildListID == "" || len(view.tree) == 0 {
		return fmt.Errorf("repository Go module view is closed or incomplete")
	}
	if err := assertRepositoryVerificationTreeUnchangedContextWithLimits(
		ctx, view.root, view.tree, view.limits.MaxEntries, view.limits.MaxRegularBytes,
	); err != nil {
		return fmt.Errorf("verify exact repository Go module view: %w", err)
	}
	return nil
}

func (view *repositoryGoModuleView) Cleanup() error {
	if view == nil {
		return nil
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return nil
	}
	if err := removeRepositoryGoModuleView(view.root); err != nil {
		return err
	}
	view.closed = true
	return nil
}

func removeRepositoryGoModuleView(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("repository Go module view has no exact cleanup root")
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("clean repository Go module view: %w", err)
	}
	return nil
}

func repositoryGoBuildListIdentity(modules []repositoryGoResolvedModule) (string, error) {
	for index, item := range modules {
		if item.Path == "" || item.Version == "" || !validRepositoryModuleSum(item.Sum) ||
			!validRepositoryModuleSum(item.GoModSum) ||
			index > 0 && (modules[index-1].Path > item.Path ||
				modules[index-1].Path == item.Path && modules[index-1].Version >= item.Version) {
			return "", fmt.Errorf("repository Go module build list is invalid or non-canonical")
		}
	}
	raw, err := json.Marshal(modules)
	if err != nil {
		return "", fmt.Errorf("encode repository Go module build-list authority: %w", err)
	}
	digest := sha256.Sum256(append([]byte("omnidex.repository-go-module-view.v1\x00"), raw...))
	return hex.EncodeToString(digest[:]), nil
}
