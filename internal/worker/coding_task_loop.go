package worker

import (
	"context"
	"fmt"

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
			return runtime.runWithSessionMessages(ctx, func(reviewCtx context.Context) error {
				return codex.ReviewGoal(reviewCtx, codingagent.Objective{Instruction: request.Instruction, Clarifications: request.Feedback})
			})
		}
		if err := runtime.runWithSessionMessages(ctx, func(taskCtx context.Context) error {
			return codex.RunTask(taskCtx, *task)
		}); err != nil {
			return err
		}
		if err := runtime.svc.repo.AcceptCodingLedgerTask(ctx, runtime.claim.Authority, task.ID); err != nil {
			return err
		}
		replanned, err := runtime.svc.repo.ReplanPendingCodingAmendments(ctx, runtime.claim.Authority)
		if err != nil {
			return err
		}
		if replanned {
			return fmt.Errorf("%w: queued amendments entered planning after task acceptance", queue.ErrStaleStepAttempt)
		}
	}
}
