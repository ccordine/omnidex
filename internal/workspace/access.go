package workspace

import (
	"context"
	"fmt"
)

type Prepared interface {
	ApplyVerified(context.Context, VerifiedChangeObserver) (ReconciliationResult, error)
}

// Access represents one acquired workspace, with code-owned reads, mutation
// preparation, and release. Its transport never grants a different root.
type Access interface {
	Reader
	Reattest(string) error
	Prepare(context.Context, []DesiredFile, []File) (Prepared, error)
	Release() error
}

func (fence *MutationFence) Stat(ctx context.Context, relative string) (Entry, error) {
	return readFencedRoot(fence, func(root RootReader) (Entry, error) { return ReadEntry(ctx, root, relative) })
}

func (fence *MutationFence) ReadFile(ctx context.Context, relative string) (File, error) {
	return readFencedRoot(fence, func(root RootReader) (File, error) { return ReadFile(ctx, root, relative) })
}

func (fence *MutationFence) ReadDirectory(ctx context.Context, relative, after string, limit int) (DirectoryPage, error) {
	return readFencedRoot(fence, func(root RootReader) (DirectoryPage, error) { return ReadDirectory(ctx, root, relative, after, limit) })
}

func readFencedRoot[T any](fence *MutationFence, read func(RootReader) (T, error)) (T, error) {
	var zero T
	if fence == nil {
		return zero, fmt.Errorf("workspace read requires a mutation fence")
	}
	fence.mu.Lock()
	defer fence.mu.Unlock()
	root, err := fence.authoritativeRootLocked()
	if err != nil {
		return zero, err
	}
	value, err := read(root)
	if err != nil {
		return zero, err
	}
	if _, err := fence.authoritativeRootLocked(); err != nil {
		return zero, err
	}
	return value, nil
}
