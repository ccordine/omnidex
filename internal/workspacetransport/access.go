package workspacetransport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/workspace"
)

type access struct {
	remote   *remote
	ctx      context.Context
	lease    uint64
	mu       sync.Mutex
	released bool
}

func (hub *Hub) Acquire(ctx context.Context, root, identity string) (workspace.Access, error) {
	r, err := hub.connection(ctx, root, identity)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// An acquisition round must finish so code knows whether it owns a
		// lease. Canceling a waiter must not close another owner's connection.
		round, cancel := context.WithTimeout(context.WithoutCancel(ctx), operationTimeout)
		reply, err := r.call(round, requestData{request: request{Kind: opAcquire}}, nil)
		cancel()
		if err == nil {
			acquired := &access{remote: r, ctx: ctx, lease: reply.ID}
			if err := ctx.Err(); err != nil {
				return nil, errors.Join(err, acquired.Release())
			}
			return acquired, nil
		}
		if ctx.Err() != nil {
			if errors.Is(err, workspace.ErrWorkspaceBusy) {
				return nil, ctx.Err()
			}
			return nil, errors.Join(ctx.Err(), err)
		}
		if !errors.Is(err, workspace.ErrWorkspaceBusy) {
			return nil, err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-r.done:
			timer.Stop()
			return nil, r.failure()
		case <-timer.C:
		}
	}
}

func (a *access) call(ctx context.Context, data requestData, observe func(workspace.Change) error) (responseData, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.released {
		return responseData{}, fmt.Errorf("client workspace access was released")
	}
	data.Lease = a.lease
	return a.remote.call(ctx, data, observe)
}

func (a *access) Reattest(root string) error {
	if root != a.remote.binding.Root {
		return fmt.Errorf("workspace reattestation root differs from its exact binding")
	}
	_, err := a.call(a.ctx, requestData{request: request{Kind: opAttest}}, nil)
	return err
}

func (a *access) Stat(ctx context.Context, path string) (workspace.Entry, error) {
	reply, err := a.call(ctx, requestData{request: request{Kind: opStat, Path: path}}, nil)
	if err != nil {
		return workspace.Entry{}, err
	}
	return *reply.Entry, nil
}

func (a *access) ReadFile(ctx context.Context, path string) (workspace.File, error) {
	reply, err := a.call(ctx, requestData{request: request{Kind: opReadFile, Path: path}}, nil)
	if err != nil {
		return workspace.File{}, err
	}
	return workspace.File{Entry: *reply.Entry, Content: reply.content}, nil
}

func (a *access) ReadDirectory(ctx context.Context, path, after string, limit int) (workspace.DirectoryPage, error) {
	reply, err := a.call(ctx, requestData{request: request{Kind: opReadDirectory, Path: path, After: after, Limit: limit}}, nil)
	if err != nil {
		return workspace.DirectoryPage{}, err
	}
	return *reply.Page, nil
}

func (a *access) Release() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.released {
		return fmt.Errorf("client workspace access was already released")
	}
	a.released = true
	_, err := a.remote.call(context.Background(), requestData{request: request{Kind: opRelease, Lease: a.lease}}, nil)
	return err
}
