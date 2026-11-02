package worker

import (
	"context"
	"fmt"
	"log"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/queue"
)

func (runtime *nativeRuntimeV3) runCodingTaskLoop(ctx context.Context, codex *codingagent.Controller) error {
	for {
		task, err := runtime.svc.repo.NextIncompleteCodingLedgerTask(ctx, runtime.claim.Authority)
		if err != nil {
			return err
		}
		if task == nil {
			request, err := runtime.codingLedgerRequest()
			if err != nil {
				return err
			}
			err = runtime.runWithSessionMessages(ctx, func(reviewCtx context.Context) error {
				return codex.ReviewGoal(reviewCtx, codingagent.Objective{Instruction: request.Instruction, Clarifications: request.Feedback})
			})
			if err == nil {
				runtime.cleanupCodingWorkflow(context.WithoutCancel(ctx), codex, []string{"goal-review"})
			}
			return err
		}
		if err := runtime.runWithSessionMessages(ctx, func(taskCtx context.Context) error {
			return codex.RunTask(taskCtx, *task)
		}); err != nil {
			return err
		}
		if err := runtime.svc.repo.AcceptCodingLedgerTask(ctx, runtime.claim.Authority, task.ID); err != nil {
			return err
		}
		codex.CompletedTaskIDs = append(codex.CompletedTaskIDs, task.ID)
		runtime.cleanupCodingWorkflow(context.WithoutCancel(ctx), codex, []string{task.ID})
		replanned, err := runtime.svc.repo.ReplanPendingCodingAmendments(ctx, runtime.claim.Authority)
		if err != nil {
			return err
		}
		if replanned {
			return fmt.Errorf("%w: queued amendments entered planning after task acceptance", queue.ErrStaleStepAttempt)
		}
	}
}

// Cleanup is code-owned, never another model stage or an acceptance gate.
// Retained ownership receipts make failed cleanup retryable on recovery.
func (runtime *nativeRuntimeV3) cleanupCodingWorkflow(ctx context.Context, controller *codingagent.Controller, accepted []string) {
	for _, provider := range controller.Providers {
		if lifecycle, ok := provider.(interface {
			CleanupWorkflow(context.Context, string, int64, []string) error
		}); ok {
			if err := lifecycle.CleanupWorkflow(ctx, controller.Root, controller.JobID, accepted); err != nil {
				log.Printf("job %d: owned resource cleanup pending; will retry: %v", controller.JobID, err)
			}
		}
	}
}
