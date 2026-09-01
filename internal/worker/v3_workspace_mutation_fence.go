package worker

import (
	"errors"
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (r *nativeRuntimeV3) acquireWorkspaceMutationFence(root string) error {
	if r == nil || r.svc == nil || r.svc.repo == nil || r.claim == nil || r.ctx == nil {
		return fmt.Errorf("acquire workspace mutation fence requires exact runtime authority")
	}
	if r.workspaceFence != nil {
		if r.workspaceFenceRoot != root {
			return fmt.Errorf(
				"workspace mutation runtime already owns root %q, cannot acquire %q",
				r.workspaceFenceRoot, root,
			)
		}
		return nil
	}
	fence, err := workspacefacts.AcquireMutationFence(r.ctx, root)
	if err != nil {
		return err
	}
	if err := r.svc.repo.RequireActiveStepAttempt(r.ctx, r.claim.Authority); err != nil {
		return errors.Join(err, fence.Release())
	}
	r.workspaceFence = fence
	r.workspaceFenceRoot = root
	return nil
}

func (r *nativeRuntimeV3) requireWorkspaceMutationFence() error {
	if r == nil || r.workspaceFence == nil || r.workspaceFenceRoot == "" {
		return fmt.Errorf("workspace completion requires the authoritative root mutation fence")
	}
	return nil
}

func (r *nativeRuntimeV3) releaseWorkspaceMutationFence() error {
	if r == nil || r.workspaceFence == nil {
		return nil
	}
	fence := r.workspaceFence
	r.workspaceFence = nil
	r.workspaceFenceRoot = ""
	return fence.Release()
}
