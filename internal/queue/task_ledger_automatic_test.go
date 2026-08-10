package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresJobLifecycleAutomaticallyRecordsRootTaskAuthority(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	instruction := fmt.Sprintf("Preserve exact user authority %d\nacross restarts.", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, instruction, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	state, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertInitialTaskAuthority(t, state, instruction, taskstate.NodeReady)

	claimed, err := repository.ClaimNextStep(ctx, fmt.Sprintf("root-authority-worker-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Job.ID != job.ID {
		t.Fatalf("claimed step=%+v, want job %d", claimed, job.ID)
	}
	state, err = repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertInitialTaskAuthority(t, state, instruction, taskstate.NodeActive)

	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "automatic-complete", claimed.Step.ID),
		Authority:   claimed.Authority,
		StepID:      claimed.Step.ID, Output: "verified completion",
	}); err != nil {
		t.Fatal(err)
	}
	state, err = repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertInitialTaskAuthority(t, state, instruction, taskstate.NodeDone)
	if state.Status != taskstate.LedgerClosed || len(state.Nodes[0].VerificationRefs) != 1 {
		t.Fatalf("terminal state=%+v", state)
	}
	if state.Nodes[0].VerificationRefs[0].Relation != taskstate.RefVerifies {
		t.Fatalf("completion ref=%+v", state.Nodes[0].VerificationRefs[0])
	}

	var jobStatus, stepStatus string
	if err := pool.QueryRow(ctx, `
		SELECT jobs.status, job_steps.status
		FROM jobs JOIN job_steps ON job_steps.job_id=jobs.id
		WHERE jobs.id=$1 AND job_steps.id=$2
	`, job.ID, claimed.Step.ID).Scan(&jobStatus, &stepStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != model.JobStatusCompleted || stepStatus != model.StepStatusCompleted {
		t.Fatalf("job/step status=%q/%q", jobStatus, stepStatus)
	}
}

func assertInitialTaskAuthority(
	t *testing.T,
	state taskstate.MaterializedState,
	instruction string,
	wantStatus taskstate.NodeStatus,
) {
	t.Helper()
	if len(state.Nodes) != 1 || state.Nodes[0].ID != initialTaskRootNodeID ||
		state.Nodes[0].Kind != taskstate.NodeGoal || state.Nodes[0].Status != wantStatus {
		t.Fatalf("root nodes=%+v, want one %q goal in %q", state.Nodes, initialTaskRootNodeID, wantStatus)
	}
	if len(state.Entries) != 1 || state.Entries[0].ID != initialUserInstructionEntryID ||
		state.Entries[0].Authority != taskstate.AuthorityUser || len(state.Entries[0].Refs) != 1 {
		t.Fatalf("instruction entries=%+v", state.Entries)
	}
	wantHashBytes := sha256.Sum256([]byte(instruction))
	wantHash := hex.EncodeToString(wantHashBytes[:])
	ref := state.Entries[0].Refs[0]
	if ref.Hash != wantHash || ref.Relation != taskstate.RefSource {
		t.Fatalf("instruction ref=%+v, want hash %s", ref, wantHash)
	}
}
