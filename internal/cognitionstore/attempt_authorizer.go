package cognitionstore

import (
	"context"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
)

func (store *Store) AuthorizeAttempt(
	ctx context.Context,
	actor cognition.AttemptRef,
) error {
	if store == nil || store.repository == nil {
		return fmt.Errorf("cognition attempt authorizer is uninitialized")
	}
	if err := actor.Validate(); err != nil {
		return err
	}
	if actor.Attempt > math.MaxInt64 {
		return fmt.Errorf("cognition attempt exceeds PostgreSQL BIGINT")
	}
	return store.repository.AuthorizeStepAttempt(ctx, model.StepAttemptAuthority{
		JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
		Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
	})
}
