package queue

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresWorkerWriteFenceRejectsTerminalAttemptBeforeMutation(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("attempt-worker-write-%d", time.Now().UnixNano())
	job := enqueueWorkingSetTestJob(t, ctx, repository, marker)
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	accepted := marker + "-accepted"
	chunk, err := repository.AddMemoryChunkByStepAttempt(
		ctx, claim.Authority, marker, model.MemoryKindReference, accepted, nil, nil,
	)
	if err != nil || chunk.ID <= 0 {
		t.Fatalf("accepted worker memory=%+v error=%v", chunk, err)
	}
	if _, err := repository.CancelJob(ctx, testCancelCommand(
		t, job.ID, "worker-write-fence", "terminalize the exact attempt",
	)); err != nil {
		t.Fatal(err)
	}
	rejected := marker + "-rejected"
	if _, err := repository.AddMemoryChunkByStepAttempt(
		ctx, claim.Authority, marker, model.MemoryKindReference, rejected, nil, nil,
	); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("terminal worker write error=%v want ErrStaleStepAttempt", err)
	}
	var rejectedCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM memory_chunks WHERE content=$1`, rejected).Scan(&rejectedCount); err != nil {
		t.Fatal(err)
	}
	if rejectedCount != 0 {
		t.Fatalf("terminal attempt persisted %d rejected worker writes", rejectedCount)
	}
}
