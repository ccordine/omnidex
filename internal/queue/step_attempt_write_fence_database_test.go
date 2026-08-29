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
	workspaceRoot := "/tmp/" + marker
	var projectID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES ($1,$2) RETURNING id
	`, workspaceRoot, marker).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	channelID := model.ChannelID(marker)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channels(id,scope,name,tags,project_id,workspace_root)
		VALUES ($1,'user',$2,ARRAY[]::text[],$3,$4)
	`, channelID, marker, projectID, workspaceRoot); err != nil {
		t.Fatal(err)
	}
	scope := model.MemoryScope{ProjectID: projectID, ChannelID: channelID}
	accepted := marker + "-accepted"
	chunk, err := repository.AddMemoryChunkByStepAttempt(
		ctx, claim.Authority, scope, marker, model.MemoryKindReference, accepted, nil, nil,
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
		ctx, claim.Authority, scope, marker, model.MemoryKindReference, rejected, nil, nil,
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
