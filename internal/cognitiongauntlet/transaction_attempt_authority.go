package cognitiongauntlet

import (
	"context"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

func transactionAttemptAuthorizer(
	repository *queue.Repository,
) labyrinthhost.TransactionAttemptAuthorizer {
	return func(ctx context.Context, tx pgx.Tx, actor cognition.AttemptRef) error {
		if err := actor.Validate(); err != nil {
			return err
		}
		if actor.Attempt > math.MaxInt64 {
			return fmt.Errorf("cognition attempt exceeds PostgreSQL BIGINT")
		}
		return repository.AuthorizeStepAttemptTransaction(ctx, tx, model.StepAttemptAuthority{
			JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
			Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
		})
	}
}
