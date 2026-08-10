package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func waitForReplacementClaim(
	ctx context.Context,
	database *offlinePromotionDatabase,
	original model.StepAttemptAuthority,
) (model.StepAttemptAuthority, error) {
	if ctx == nil || database == nil || database.repository == nil {
		return model.StepAttemptAuthority{}, fmt.Errorf("cognition takeover claim requires PostgreSQL and context")
	}
	worker, err := randomProcessIdentity("gauntlet-replacement-")
	if err != nil {
		return model.StepAttemptAuthority{}, err
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return model.StepAttemptAuthority{}, ctx.Err()
		case <-ticker.C:
			claim, err := database.repository.ClaimNextStep(ctx, worker)
			if err != nil {
				return model.StepAttemptAuthority{}, err
			}
			if claim == nil {
				continue
			}
			replacement := claim.Authority
			if replacement.JobID != original.JobID || replacement.Generation != original.Generation ||
				replacement.StepID != original.StepID || replacement.Attempt != original.Attempt+1 ||
				replacement.WorkerID == original.WorkerID {
				return model.StepAttemptAuthority{}, fmt.Errorf("lease reclaim returned another cognition step or attempt")
			}
			return replacement, nil
		}
	}
}
