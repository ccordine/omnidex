package queue

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresStepClaimRollsBackWhenContextProjectionExceedsBudget(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("context-projection-budget-%d", time.Now().UnixNano())

	itemJob, err := repository.EnqueueJob(ctx, marker+"-items", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	itemStepID := taskGenerationStepID(t, ctx, pool, itemJob.ID, 1)
	if _, err := pool.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		SELECT $1, 'item-' || value::text, 'x'
		FROM generate_series(1, $2) AS value
	`, itemStepID, maxClaimedStepContextItems+1); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNextStep(ctx, marker+"-item-worker"); !errors.Is(err, ErrContextProjectionBudget) {
		t.Fatalf("item projection error=%v", err)
	}
	assertProjectionClaimRolledBack(t, repository, itemJob.ID, itemStepID)
	if _, err := repository.CancelJob(ctx, testCancelCommand(
		t, itemJob.ID, "oversized-item-cleanup", "release oversized item fixture",
	)); err != nil {
		t.Fatal(err)
	}

	byteJob, err := repository.EnqueueJob(ctx, marker+"-bytes", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	byteStepID := taskGenerationStepID(t, ctx, pool, byteJob.ID, 1)
	if _, err := pool.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value) VALUES ($1, 'oversized', $2)
	`, byteStepID, strings.Repeat("x", maxClaimedStepContextBytes+1)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNextStep(ctx, marker+"-byte-worker"); !errors.Is(err, ErrContextProjectionBudget) {
		t.Fatalf("byte projection error=%v", err)
	}
	assertProjectionClaimRolledBack(t, repository, byteJob.ID, byteStepID)
	if _, err := repository.CancelJob(ctx, testCancelCommand(
		t, byteJob.ID, "oversized-byte-cleanup", "release oversized byte fixture",
	)); err != nil {
		t.Fatal(err)
	}
}

func assertProjectionClaimRolledBack(t *testing.T, repository *Repository, jobID, stepID int64) {
	t.Helper()
	jobStatus, stepStatus, err := repository.GetStepRuntimeState(t.Context(), jobID, stepID)
	if err != nil {
		t.Fatal(err)
	}
	if jobStatus != model.JobStatusPending || stepStatus != model.StepStatusPending {
		t.Fatalf("oversized projection retained claim state job=%q step=%q", jobStatus, stepStatus)
	}
}
