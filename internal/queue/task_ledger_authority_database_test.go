package queue

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func TestPostgresPublicTaskCommandsCannotMutateCanonicalAuthority(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("canonical-boundary-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	activationID, err := initialLifecycleCommandID(job.ID, 1, 0, "activate-root")
	if err != nil {
		t.Fatal(err)
	}
	resolveID, err := taskstate.NewCommandID(marker, "resolve-instruction")
	if err != nil {
		t.Fatal(err)
	}
	commands := []taskstate.Command{
		taskstate.TransitionNodeCommand{
			CommandID: activationID, ExpectedVersion: initialTaskLedgerVersion,
			Actor: taskstate.AuthorityCode, NodeID: initialTaskRootNodeID,
			To: taskstate.NodeActive,
		},
		taskstate.ResolveEntryCommand{
			CommandID: resolveID, ExpectedVersion: initialTaskLedgerVersion,
			Actor: taskstate.AuthorityCode, EntryID: initialUserInstructionEntryID,
			Reason: "forbidden", Refs: []taskstate.Ref{initialInstructionRef(job)},
		},
	}
	for _, command := range commands {
		if _, err := repository.ApplyTaskCommand(ctx, job.ID, 1, command); !errors.Is(err, taskstate.ErrAuthorityDenied) {
			t.Fatalf("canonical command %T error=%v", command, err)
		}
	}
	state, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != initialTaskLedgerVersion || state.Nodes[0].Status != taskstate.NodeReady ||
		state.Entries[0].Status != taskstate.EntryActive {
		t.Fatalf("forbidden commands mutated canonical authority: %+v", state)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE task_nodes SET status='active' WHERE job_id=$1 AND id=$2
	`, job.ID, initialTaskRootNodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNextStep(ctx, marker+"-worker"); !errors.Is(err, taskstate.ErrInvalidState) || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("uncorroborated active-root claim error=%v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE task_nodes SET status='ready' WHERE job_id=$1 AND id=$2
	`, job.ID, initialTaskRootNodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelJob(ctx, testCancelCommand(
		t, job.ID, "canonical-boundary-cleanup", "finish canonical boundary fixture",
	)); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresInvalidJobInstructionRollsBackWithoutSanitizingAuthority(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("invalid-instruction-%d", time.Now().UnixNano())
	invalid := []string{
		marker + "-utf8-" + string([]byte{0xff}),
		marker + "-nul-\x00authority",
	}
	for _, instruction := range invalid {
		if _, err := repository.EnqueueJob(ctx, instruction, model.PipelineCoding, []byte(`{}`)); err == nil {
			t.Fatalf("invalid instruction %q was accepted", instruction)
		}
	}
	var persisted int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM jobs WHERE instruction LIKE $1
	`, marker+"%").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != 0 {
		t.Fatalf("invalid instruction produced %d persisted jobs", persisted)
	}

	exact := "  " + marker + "-exact\n"
	job, err := repository.EnqueueJob(ctx, exact, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if job.Instruction != exact {
		t.Fatalf("stored instruction=%q want byte-exact %q", job.Instruction, exact)
	}
	assertInitialTaskAuthority(t, mustTaskLedger(t, repository, job.ID), exact, taskstate.NodeReady)
	if _, err := repository.CancelJob(ctx, testCancelCommand(
		t, job.ID, "exact-instruction-cleanup", "finish exact instruction fixture",
	)); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresActiveRootRemainsCanonicalAcrossReplanGeneration(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("active-root-replan-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	first, err := repository.ClaimNextStep(ctx, marker+"-generation-one")
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.Job.ID != job.ID {
		t.Fatalf("generation-one claim=%+v", first)
	}
	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "replace-current", "Replace the remaining current-generation work.")); err != nil {
		t.Fatal(err)
	}
	second, err := repository.ClaimNextStep(ctx, marker+"-generation-two")
	if err != nil {
		t.Fatal(err)
	}
	if second == nil || second.Job.ID != job.ID || second.Step.Generation != 2 {
		t.Fatalf("generation-two claim=%+v", second)
	}
	if _, err := repository.CancelJob(ctx, testCancelCommand(
		t, job.ID, "active-root-replan-cleanup", "finish active-root replan fixture",
	)); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresReplanRollsBackWhenFeedbackLedgerWriteFails(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("replan-feedback-rollback-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	functionName := pgx.Identifier{"reject_replan_feedback_" + suffix}.Sanitize()
	triggerName := pgx.Identifier{"reject_replan_feedback_trigger_" + suffix}.Sanitize()
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			IF NEW.job_id=%d AND NEW.id=%s THEN
				RAISE EXCEPTION 'forced replan feedback failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER %s BEFORE INSERT ON task_entries
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, functionName, job.ID, quoteSQLLiteral(string(replanFeedbackEntryID(2))), triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			"DROP TRIGGER %s ON task_entries; DROP FUNCTION %s()", triggerName, functionName,
		)); err != nil {
			t.Errorf("remove replan feedback failure trigger: %v", err)
		}
	})

	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "rollback-feedback", "This feedback must roll back atomically.")); err == nil || !strings.Contains(err.Error(), "forced replan feedback failure") {
		t.Fatalf("forced feedback failure error=%v", err)
	}
	var generation, generations, ledgerVersion, events, entries int64
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT jobs.current_generation, jobs.status,
		       (SELECT COUNT(*) FROM job_generations WHERE job_id=jobs.id),
		       task_ledgers.version,
		       (SELECT COUNT(*) FROM task_events WHERE job_id=jobs.id),
		       (SELECT COUNT(*) FROM task_entries WHERE job_id=jobs.id)
		FROM jobs JOIN task_ledgers ON task_ledgers.job_id=jobs.id
		WHERE jobs.id=$1
	`, job.ID).Scan(&generation, &status, &generations, &ledgerVersion, &events, &entries); err != nil {
		t.Fatal(err)
	}
	if generation != 1 || status != model.JobStatusPending || generations != 1 ||
		ledgerVersion != int64(initialTaskLedgerVersion) ||
		events != int64(initialTaskLedgerVersion) || entries != 1 {
		t.Fatalf(
			"rollback generation/status/rows/ledger/events/entries=%d/%s/%d/%d/%d/%d",
			generation, status, generations, ledgerVersion, events, entries,
		)
	}
}

func quoteSQLLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func mustTaskLedger(t *testing.T, repository *Repository, jobID int64) taskstate.MaterializedState {
	t.Helper()
	state, err := repository.TaskLedger(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
