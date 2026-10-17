package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/codingagent"
)

func (runtime *nativeRuntimeV3) runCodingTaskLoop(ctx context.Context, codex *codingagent.Controller) error {
	for {
		task, err := runtime.svc.repo.NextIncompleteCodingLedgerTask(ctx, runtime.claim.Authority)
		if err != nil {
			return err
		}
		if task == nil {
			return nil
		}
		if err := codex.RunTask(ctx, *task); err != nil {
			return err
		}
		if err := runtime.svc.repo.AcceptCodingLedgerTask(ctx, runtime.claim.Authority, task.ID); err != nil {
			return err
		}
	}
}
