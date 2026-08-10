package queue

import "testing"

func TestPostgresCognitionLifecycleSealSetRejectsPostCommitAppend(t *testing.T) {
	repository, pool, ctx := lifecycleRetirementTestRepository(t)
	first := startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx, "append-first")
	firstCommand := testCancelCommand(t, first.Job.ID, "append-first", "Seal the first episode set.")
	if _, err := repository.CancelJob(ctx, firstCommand); err != nil {
		t.Fatal(err)
	}
	second := startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx, "append-second")
	secondCommand := testCancelCommand(t, second.Job.ID, "append-second", "Seal the second episode set.")
	if _, err := repository.CancelJob(ctx, secondCommand); err != nil {
		t.Fatal(err)
	}
	var retirementID, retirementSHA, traceSHA string
	if err := pool.QueryRow(ctx, `
		SELECT retirements.retirement_id,retirements.retirement_sha256,seals.trace_sha256
		FROM cognition_lifecycle_retirements retirements
		JOIN cognition_terminal_seals seals ON seals.episode_id=retirements.episode_id
		WHERE retirements.operation_id=$1
	`, secondCommand.OperationID).Scan(&retirementID, &retirementSHA, &traceSHA); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_lifecycle_operation_seal_episodes (
		 operation_id,position,episode_id,retirement_id,retirement_sha256,trace_sha256
		) VALUES ($1,1,$2,$3,$4,$5)
	`, firstCommand.OperationID, second.EpisodeID, retirementID, retirementSHA, traceSHA); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append setup failed before deferred authority check: %v", err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("post-commit lifecycle seal-set append was accepted")
	}
	var childCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM cognition_lifecycle_operation_seal_episodes WHERE operation_id=$1
	`, firstCommand.OperationID).Scan(&childCount); err != nil {
		t.Fatal(err)
	}
	if childCount != 1 {
		t.Fatalf("rejected append changed immutable child count=%d", childCount)
	}
}
