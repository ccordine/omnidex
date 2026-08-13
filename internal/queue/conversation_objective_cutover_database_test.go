package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresConversationObjectiveCutoverPreservesOnlyCurrentAuthority(t *testing.T) {
	ctx, pool, repository := installConversationSchemaThrough065(t)
	marker := fmt.Sprintf("conversation-cutover-success-%d", time.Now().UnixNano())
	liveJobID, liveStepID := enqueueConversationCutoverJob(t, ctx, repository, marker+"-live")
	channelID := marker + "-channel"
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channels (id,name,tags) VALUES ($1,'Current channel',ARRAY['current'])
	`, channelID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channel_messages (channel_id,role,content)
		VALUES ($1,'user','Exact retained message')
	`, channelID); err != nil {
		t.Fatal(err)
	}
	historicalJobID := seedCompletedHistoricalPlanningJob(t, ctx, pool, marker+"-history")
	ledgerBefore := conversationCutoverLedgerSnapshot(t, ctx, pool)

	if err := applyConversationCutover(t, ctx, repository); err != nil {
		t.Fatal(err)
	}
	if got := conversationCutoverLedgerSnapshot(t, ctx, pool); got != ledgerBefore {
		t.Fatalf("066 rewrote historical migration authority\nbefore=%s\nafter=%s", ledgerBefore, got)
	}
	assertAppliedMigrationCount(t, pool, conversationObjectiveCutoverMigration, 1)
	assertConversationCutoverCatalog(t, ctx, pool)
	assertConversationCutoverData(t, ctx, pool, liveJobID, liveStepID, historicalJobID, channelID)

	assertCurrentGenerationBoundary(t, ctx, pool, liveJobID)
}

func seedCompletedHistoricalPlanningJob(
	t *testing.T,
	ctx context.Context,
	pool interface {
		BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	},
	instruction string,
) int64 {
	t.Helper()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var jobID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO jobs (instruction,pipeline,status,metadata)
		VALUES ($1,'assistant','completed','{}'::jsonb)
		RETURNING id
	`, instruction).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	const feedback = "Immutable historical planning boundary."
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (job_id,generation,purpose)
		VALUES ($1,1,'initial')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (
			job_id,generation,purpose,predecessor_generation,boundary_action,
			feedback,feedback_sha256
		) VALUES ($1,2,'replan',1,'v3_planning',$2,
			encode(digest($2,'sha256'),'hex'))
	`, jobID, feedback); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET current_generation=2 WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return jobID
}

func assertConversationCutoverCatalog(
	t *testing.T,
	ctx context.Context,
	queryer interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
) {
	t.Helper()
	var retiredColumns, retiredRelations, retiredRoutines, retiredIndex int
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='ai_channels'
		  AND column_name IN ('persona','system','provider','model','context')
	`).Scan(&retiredColumns); err != nil {
		t.Fatal(err)
	}
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema()
		  AND c.relname IN ('task_artifact_projections','task_artifact_projection_items')
	`).Scan(&retiredRelations); err != nil {
		t.Fatal(err)
	}
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
		WHERE n.nspname=current_schema() AND p.proname=ANY($1::text[])
	`, []string{
		"validate_task_artifact_projection", "validate_task_artifact_projection_item",
		"prevent_task_artifact_projection_mutation", "prevent_projected_artifact_mutation",
		"require_intent_artifact_projection",
	}).Scan(&retiredRoutines); err != nil {
		t.Fatal(err)
	}
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=current_schema() AND c.relkind='i'
		  AND c.relname='idx_artifacts_id_job_step'
	`).Scan(&retiredIndex); err != nil {
		t.Fatal(err)
	}
	if retiredColumns != 0 || retiredRelations != 0 || retiredRoutines != 0 || retiredIndex != 0 {
		t.Fatalf("retired authority remains columns/relations/routines/index=%d/%d/%d/%d",
			retiredColumns, retiredRelations, retiredRoutines, retiredIndex)
	}

	var oldConstraint, exactConstraint, exactTrigger int
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE conname='job_generations_check'),
		       COUNT(*) FILTER (
			WHERE conname='job_generations_authoritative_shape' AND contype='c'
			  AND convalidated AND NOT connoinherit
			  AND encode(digest(pg_get_constraintdef(oid,true),'sha256'),'hex')=
			      '6d35378110ee10f551a3db1f9384099ddcae7bbf2e15763262bafcb437e493b3'
		)
		FROM pg_constraint WHERE conrelid='job_generations'::regclass
	`).Scan(&oldConstraint, &exactConstraint); err != nil {
		t.Fatal(err)
	}
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_trigger t
		JOIN pg_class c ON c.oid=t.tgrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		JOIN pg_proc p ON p.oid=t.tgfoid
		WHERE n.nspname=current_schema() AND c.relname='job_generations'
		  AND t.tgname='job_generations_require_current_boundary'
		  AND p.proname='require_current_job_generation_boundary'
		  AND t.tgtype=7 AND t.tgenabled='O' AND NOT t.tgisinternal
		  AND p.pronargs=0 AND p.prorettype='trigger'::regtype
		  AND encode(digest(p.prosrc,'sha256'),'hex')=
		      'b8eecfca02b64a0a72f493c64e93608a67adadaaaff3fc90a6dcf55ea3e02ed3'
	`).Scan(&exactTrigger); err != nil {
		t.Fatal(err)
	}
	if oldConstraint != 0 || exactConstraint != 1 || exactTrigger != 1 {
		t.Fatalf("066 constraint/trigger identity old/new/guard=%d/%d/%d",
			oldConstraint, exactConstraint, exactTrigger)
	}
}

func assertConversationCutoverData(
	t *testing.T,
	ctx context.Context,
	queryer interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	liveJobID, liveStepID, historicalJobID int64,
	channelID string,
) {
	t.Helper()
	var liveJobs, liveSteps, history, channels, messages int
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM jobs WHERE id=$1 AND status='pending' AND current_generation=1
	`, liveJobID).Scan(&liveJobs); err != nil {
		t.Fatal(err)
	}
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=1 AND action='objective_resolve'
	`, liveStepID, liveJobID).Scan(&liveSteps); err != nil {
		t.Fatal(err)
	}
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_generations
		WHERE job_id=$1 AND generation=2 AND boundary_action='v3_planning'
	`, historicalJobID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM ai_channels WHERE id=$1 AND name='Current channel' AND tags=ARRAY['current']
	`, channelID).Scan(&channels); err != nil {
		t.Fatal(err)
	}
	if err := queryer.QueryRow(ctx, `
		SELECT COUNT(*) FROM ai_channel_messages
		WHERE channel_id=$1 AND role='user' AND content='Exact retained message'
	`, channelID).Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if liveJobs != 1 || liveSteps != 1 || history != 1 || channels != 1 || messages != 1 {
		t.Fatalf("066 preserved jobs/steps/history/channels/messages=%d/%d/%d/%d/%d",
			liveJobs, liveSteps, history, channels, messages)
	}
}

func assertCurrentGenerationBoundary(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID int64,
) {
	t.Helper()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	const feedback = "Exact current objective boundary."
	insertBoundary := `
		INSERT INTO job_generations (
			job_id,generation,purpose,predecessor_generation,boundary_action,
			feedback,feedback_sha256
		) VALUES ($1,2,'replan',1,$2,$3,encode(digest($3,'sha256'),'hex'))
	`
	assertPostgresFailureContains(t, ctx, tx, insertBoundary,
		"new job generation boundary v3_planning is retired", jobID, "v3_planning", feedback)
	if _, err := tx.Exec(ctx, insertBoundary, jobID, "objective_resolve", feedback); err != nil {
		t.Fatalf("objective_resolve boundary rejected: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps SET superseded_at_generation=2 WHERE job_id=$1 AND generation=1
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET current_generation=2 WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_steps (job_id,generation,action,sort_index,status)
		VALUES ($1,2,'objective_resolve',5,'pending')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var generations, currentSteps int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_generations
		WHERE job_id=$1 AND generation=2 AND boundary_action='objective_resolve'
	`, jobID).Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_steps
		WHERE job_id=$1 AND generation=2 AND action='objective_resolve'
		  AND superseded_at_generation IS NULL
	`, jobID).Scan(&currentSteps); err != nil {
		t.Fatal(err)
	}
	if generations != 1 || currentSteps != 1 {
		t.Fatalf("objective_resolve current boundary generations/steps=%d/%d want 1/1",
			generations, currentSteps)
	}
}
