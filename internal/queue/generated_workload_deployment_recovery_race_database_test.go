package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestDeploymentRecoveryParentLockSerializesTerminalAndExecution(t *testing.T) {
	t.Run("terminal wins", func(t *testing.T) {
		fixture := generatedDeploymentApplyingFixture(t, "race-terminal-first")
		generatedDeploymentQualifyProtectedExecution(
			t, fixture, fixture.authority, fixture.manifest.Commands[0],
		)
		winner := beginGeneratedDeploymentRaceTx(t, fixture)
		defer winner.Rollback(fixture.ctx)
		if err := failGeneratedDeploymentAndReleaseTx(fixture.ctx, winner, fixture); err != nil {
			t.Fatal(err)
		}
		loser := beginGeneratedDeploymentRaceTx(t, fixture)
		defer loser.Rollback(fixture.ctx)
		err := blockedGeneratedDeploymentMutation(t, fixture, loser, func() error {
			return insertGeneratedDeploymentExecutionTx(fixture.ctx, loser, fixture)
		}, func() error { return winner.Commit(fixture.ctx) })
		if err == nil || !strings.Contains(
			err.Error(), "protected deployment execution lacks exact current-attempt namespace requalification",
		) {
			t.Fatalf("execution loser error=%v", err)
		}
		assertGeneratedDeploymentRaceState(t, fixture, GeneratedWorkloadDeploymentFailed, 0, 0, false)
	})

	t.Run("execution wins", func(t *testing.T) {
		fixture := generatedDeploymentApplyingFixture(t, "race-execution-first")
		generatedDeploymentQualifyProtectedExecution(
			t, fixture, fixture.authority, fixture.manifest.Commands[0],
		)
		winner := beginGeneratedDeploymentRaceTx(t, fixture)
		defer winner.Rollback(fixture.ctx)
		if err := insertGeneratedDeploymentExecutionTx(fixture.ctx, winner, fixture); err != nil {
			t.Fatal(err)
		}
		loser := beginGeneratedDeploymentRaceTx(t, fixture)
		defer loser.Rollback(fixture.ctx)
		err := blockedGeneratedDeploymentMutation(t, fixture, loser, func() error {
			_, err := loser.Exec(fixture.ctx, `
				UPDATE generated_workload_deployments
				SET status='failed',terminal_code='pre_side_effect_failure',terminal_detail_sha256=$2,
				    terminal_at=clock_timestamp(),updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
				WHERE id=$1
			`, generatedDeploymentOperationID(t, fixture.command), generatedDeploymentSHA("race failure"))
			return err
		}, func() error { return winner.Commit(fixture.ctx) })
		if err == nil || !strings.Contains(err.Error(), "observe-first cleanup") {
			t.Fatalf("terminal loser error=%v", err)
		}
		assertGeneratedDeploymentRaceState(t, fixture, GeneratedWorkloadDeploymentApplying, 1, 0, true)
	})
}

func TestDeploymentRecoveryParentLockSerializesExecutionAndRollback(t *testing.T) {
	for _, rollbackFirst := range []bool{false, true} {
		rollbackFirst := rollbackFirst
		name := "execution wins"
		if rollbackFirst {
			name = "rollback wins"
		}
		t.Run(name, func(t *testing.T) {
			fixture := generatedDeploymentApplyingFixtureAtAuthorityHardening(
				t, "race-child-"+strings.ReplaceAll(name, " ", "-"),
			)
			generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[0])
			generatedDeploymentQualifyAndCompleteProtected(t, fixture, fixture.manifest.Commands[1])
			winner := beginGeneratedDeploymentRaceTx(t, fixture)
			defer winner.Rollback(fixture.ctx)
			winnerMutation := insertGeneratedDeploymentExecutionTx
			loserMutation := insertGeneratedDeploymentRollbackAttemptTx
			wantExecution, wantRollback := 3, 0
			if rollbackFirst {
				winnerMutation, loserMutation = loserMutation, winnerMutation
				wantExecution, wantRollback = 2, 1
			}
			if err := winnerMutation(fixture.ctx, winner, fixture); err != nil {
				t.Fatal(err)
			}
			loser := beginGeneratedDeploymentRaceTx(t, fixture)
			defer loser.Rollback(fixture.ctx)
			err := blockedGeneratedDeploymentMutation(t, fixture, loser, func() error {
				return loserMutation(fixture.ctx, loser, fixture)
			}, func() error { return winner.Commit(fixture.ctx) })
			if err == nil {
				t.Fatal("conflicting child mutation committed after parent lock winner")
			}
			if rollbackFirst && !strings.Contains(err.Error(), "execution start authority is invalid") {
				t.Fatalf("execution loser error=%v", err)
			}
			if !rollbackFirst && !strings.Contains(err.Error(), "rollback attempt authority is invalid") {
				t.Fatalf("rollback loser error=%v", err)
			}
			assertGeneratedDeploymentRaceState(
				t, fixture, GeneratedWorkloadDeploymentApplying, wantExecution, wantRollback, true,
			)
		})
	}
}

type generatedDeploymentRaceMutation func(
	context.Context, pgx.Tx, generatedDeploymentDatabaseFixture,
) error

func beginGeneratedDeploymentRaceTx(
	t *testing.T, fixture generatedDeploymentDatabaseFixture,
) pgx.Tx {
	t.Helper()
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func blockedGeneratedDeploymentMutation(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	loser pgx.Tx,
	mutation func() error,
	release func() error,
) error {
	t.Helper()
	var pid int32
	if err := loser.QueryRow(fixture.ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		done <- mutation()
	}()
	<-started
	deadline := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("loser mutation returned before winner commit: %v", err)
		case <-deadline.C:
			t.Fatal("loser mutation never reported a PostgreSQL lock wait")
		case <-ticker.C:
			var waitType string
			if err := fixture.pool.QueryRow(fixture.ctx, `
				SELECT COALESCE(wait_event_type,'') FROM pg_stat_activity WHERE pid=$1
			`, pid).Scan(&waitType); err != nil {
				t.Fatal(err)
			}
			if waitType == "Lock" {
				if err := release(); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-done:
					return err
				case <-time.After(5 * time.Second):
					t.Fatal("loser mutation remained blocked after winner commit")
				}
			}
		}
	}
}

func insertGeneratedDeploymentExecutionTx(
	ctx context.Context, tx pgx.Tx, fixture generatedDeploymentDatabaseFixture,
) error {
	var executionCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM generated_workload_deployment_executions WHERE operation_id=$1
	`, generatedDeploymentOperationID(nil, fixture.command)).Scan(&executionCount); err != nil {
		return err
	}
	if executionCount < 0 || executionCount >= len(fixture.manifest.Commands) {
		return fmt.Errorf("deployment race has no next lifecycle execution")
	}
	command := fixture.manifest.Commands[executionCount]
	_, err := tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_executions(
		 operation_id,slot_name,slot_ordinal,step_attempt,worker_id,
		 command_sha256,workspace_sha256,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,'started')
	`, generatedDeploymentOperationID(nil, fixture.command), command.Slot.Name, command.Slot.Ordinal,
		fixture.authority.Attempt, fixture.authority.WorkerID,
		command.CommandSHA256, command.WorkspaceSHA256)
	return err
}

func insertGeneratedDeploymentRollbackAttemptTx(
	ctx context.Context, tx pgx.Tx, fixture generatedDeploymentDatabaseFixture,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_rollback_attempts(
		 operation_id,job_id,generation,step_id,step_attempt,worker_id,
		 command_sha256,workspace_sha256,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'started')
	`, generatedDeploymentOperationID(nil, fixture.command), fixture.authority.JobID,
		fixture.authority.Generation, fixture.authority.StepID, fixture.authority.Attempt,
		fixture.authority.WorkerID, fixture.rollback.Execution.CommandSHA256,
		fixture.rollback.Execution.WorkspaceSHA256)
	return err
}

func failGeneratedDeploymentAndReleaseTx(
	ctx context.Context, tx pgx.Tx, fixture generatedDeploymentDatabaseFixture,
) error {
	operationID := generatedDeploymentOperationID(nil, fixture.command)
	if _, err := tx.Exec(ctx, `
		UPDATE generated_workload_deployments
		SET status='failed',terminal_code='pre_side_effect_failure',terminal_detail_sha256=$2,
		    terminal_at=clock_timestamp(),updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
		WHERE id=$1
	`, operationID, generatedDeploymentSHA("race failure")); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE generated_workload_project_deployment_heads
		SET fence=fence+1,candidate_deployment_id=NULL,candidate_job_id=NULL,
		    candidate_generation=NULL,candidate_step_id=NULL,candidate_step_attempt=NULL,
		    candidate_worker_id=NULL,updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
		WHERE project_id=$1
	`, fixture.projectID)
	return err
}

func assertGeneratedDeploymentRaceState(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	wantState GeneratedWorkloadDeploymentState,
	wantExecutions, wantRollbacks int,
	wantCandidate bool,
) {
	t.Helper()
	var state GeneratedWorkloadDeploymentState
	var executions, rollbacks int
	var candidate bool
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT deployment.status,
		 (SELECT count(*) FROM generated_workload_deployment_executions WHERE operation_id=deployment.id),
		 (SELECT count(*) FROM generated_workload_deployment_rollback_attempts WHERE operation_id=deployment.id),
		 head.candidate_deployment_id IS NOT NULL
		FROM generated_workload_deployments AS deployment
		JOIN generated_workload_project_deployment_heads AS head ON head.project_id=deployment.project_id
		WHERE deployment.id=$1
	`, generatedDeploymentOperationID(t, fixture.command)).Scan(
		&state, &executions, &rollbacks, &candidate,
	); err != nil {
		t.Fatal(err)
	}
	if state != wantState || executions != wantExecutions || rollbacks != wantRollbacks ||
		candidate != wantCandidate {
		t.Fatalf("race state=%s executions=%d rollbacks=%d candidate=%t want %s/%d/%d/%t",
			state, executions, rollbacks, candidate, wantState, wantExecutions, wantRollbacks, wantCandidate)
	}
}

func generatedDeploymentOperationID(
	t *testing.T, command GeneratedWorkloadDeploymentCommand,
) string {
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		panic(err)
	}
	return identity.OperationID
}
