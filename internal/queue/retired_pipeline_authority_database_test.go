package queue

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

const executablePipelineAuthorityMigration = "087_executable_pipeline_authority.sql"

func TestPostgresExecutablePipelineCutoverPreservesTerminalHistoryAndGuardsWrites(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "086")); err != nil {
		t.Fatal(err)
	}
	historical := make(map[int64]string)
	for _, pipeline := range []string{"assistant", "story", "agent"} {
		historical[seedPipelineHistoryJob(t, pool, pipeline, model.JobStatusCompleted)] = pipeline
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "087")); err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidateRuntimeAuthority(t.Context()); err != nil {
		t.Fatalf("validate installed executable pipeline authority: %v", err)
	}
	jobs, err := repository.ListJobs(t.Context(), "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for id, pipeline := range historical {
		if !containsPipelineHistory(jobs, id, pipeline) {
			t.Fatalf("terminal history job %d pipeline %q was not readable", id, pipeline)
		}
	}
	for _, pipeline := range []string{"assistant", "story", "agent", "unknown"} {
		if _, err := pool.Exec(t.Context(), `
			INSERT INTO jobs(instruction,pipeline,status,metadata)
			VALUES ('must fail',$1,'pending','{}'::jsonb)
		`, pipeline); err == nil || !strings.Contains(err.Error(), "new job pipeline") {
			t.Fatalf("raw nonterminal insert pipeline %q error=%v", pipeline, err)
		}
		if _, err := pool.Exec(t.Context(), `
			INSERT INTO jobs(instruction,pipeline,status,metadata)
			VALUES ('must fail terminal forge',$1,'completed','{}'::jsonb)
		`, pipeline); err == nil || !strings.Contains(err.Error(), "new job pipeline") {
			t.Fatalf("raw terminal insert pipeline %q error=%v", pipeline, err)
		}
	}
	for id := range historical {
		if _, err := pool.Exec(t.Context(), `
			UPDATE jobs SET status='running' WHERE id=$1
		`, id); err == nil || !strings.Contains(err.Error(), "historical retired job is immutable") {
			t.Fatalf("terminal history job %d reopened error=%v", id, err)
		}
		break
	}
	for id, pipeline := range historical {
		replacement := "story"
		if pipeline == replacement {
			replacement = "assistant"
		}
		if _, err := pool.Exec(t.Context(), `UPDATE jobs SET pipeline=$2 WHERE id=$1`, id, replacement); err == nil ||
			!strings.Contains(err.Error(), "historical retired job is immutable") {
			t.Fatalf("terminal history pipeline rewrite job %d error=%v", id, err)
		}
		break
	}
	currentID := seedPipelineHistoryJob(t, pool, model.PipelineChat, model.JobStatusCompleted)
	if _, err := pool.Exec(t.Context(), `UPDATE jobs SET pipeline='agent' WHERE id=$1`, currentID); err == nil ||
		!strings.Contains(err.Error(), "current job pipeline cannot become retired") {
		t.Fatalf("current pipeline retirement error=%v", err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE jobs SET status='running' WHERE id=$1`, currentID); err == nil ||
		!strings.Contains(err.Error(), "terminal job cannot become nonterminal") {
		t.Fatalf("current terminal job reopened error=%v", err)
	}
	for id := range historical {
		if _, err := pool.Exec(t.Context(), `UPDATE jobs SET id=id+1000000000 WHERE id=$1`, id); err == nil ||
			!strings.Contains(err.Error(), "historical retired job is immutable") {
			t.Fatalf("terminal history identity rewrite job %d error=%v", id, err)
		}
		if _, err := pool.Exec(t.Context(), `UPDATE jobs SET result='rewritten' WHERE id=$1`, id); err == nil ||
			!strings.Contains(err.Error(), "historical retired job is immutable") {
			t.Fatalf("terminal history content rewrite job %d error=%v", id, err)
		}
		if _, err := pool.Exec(t.Context(), `DELETE FROM jobs WHERE id=$1`, id); err == nil ||
			!strings.Contains(err.Error(), "historical retired job is immutable") {
			t.Fatalf("terminal history deletion job %d error=%v", id, err)
		}
		break
	}
	for _, pipeline := range []string{model.PipelineChat, model.PipelineCoding, model.PipelineScrum} {
		seedPipelineHistoryJob(t, pool, pipeline, model.JobStatusPending)
	}
	assertSupportedPipelineDeleteIsReal(t, pool)
	assertAppliedMigrationCount(t, pool, executablePipelineAuthorityMigration, 1)
}

func TestPostgresExecutablePipelineCutoverRejectsLegacyWorkAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "086")); err != nil {
		t.Fatal(err)
	}
	id := seedPipelineHistoryJob(t, pool, "agent", model.JobStatusWaiting)
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "087"))
	if err == nil || !strings.Contains(err.Error(), "nonterminal retired or unregistered pipeline") {
		t.Fatalf("cutover error=%v", err)
	}
	var pipeline, status string
	if err := pool.QueryRow(t.Context(), `SELECT pipeline,status FROM jobs WHERE id=$1`, id).Scan(&pipeline, &status); err != nil {
		t.Fatal(err)
	}
	if pipeline != "agent" || status != model.JobStatusWaiting {
		t.Fatalf("rejected cutover changed job pipeline/status=%q/%q", pipeline, status)
	}
	assertAppliedMigrationCount(t, pool, executablePipelineAuthorityMigration, 0)
}

func TestPostgresClaimAndReplanRejectPreCutoverRetiredWork(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "086")); err != nil {
		t.Fatal(err)
	}
	fixture := seedPreInlineExecutionMigrationJob(
		t, t.Context(), pool, "retired runtime probe",
		model.PipelineCoding, "v3_coding", nil,
	)
	if _, err := pool.Exec(t.Context(), `UPDATE jobs SET pipeline='agent' WHERE id=$1`, fixture.Job.ID); err != nil {
		t.Fatal(err)
	}
	if claim, err := repository.ClaimNextStep(t.Context(), "retired-pipeline-probe"); claim != nil || !errors.Is(err, ErrUnsupportedPipeline) {
		t.Fatalf("retired claim=%+v error=%v", claim, err)
	}
	command := testReplanCommand(t, fixture.Job.ID, "retired-pipeline-replan", "Do not revive retired work.")
	if _, err := repository.ReplanJob(t.Context(), command); !errors.Is(err, ErrUnsupportedPipeline) {
		t.Fatalf("retired replan error=%v", err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE jobs SET pipeline='CHAT' WHERE id=$1`, fixture.Job.ID); err != nil {
		t.Fatal(err)
	}
	if claim, err := repository.ClaimNextStep(t.Context(), "noncanonical-pipeline-probe"); claim != nil || !errors.Is(err, ErrUnsupportedPipeline) {
		t.Fatalf("noncanonical claim=%+v error=%v", claim, err)
	}
	if _, err := repository.ReplanJob(t.Context(), command); !errors.Is(err, ErrUnsupportedPipeline) {
		t.Fatalf("noncanonical replan error=%v", err)
	}
}

func TestPostgresStartupRechecksExecutablePipelineRows(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "087")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		DROP TRIGGER jobs_executable_pipeline_authority ON jobs;
		ALTER TABLE jobs DROP CONSTRAINT jobs_executable_pipeline_authority;
	`); err != nil {
		t.Fatal(err)
	}
	seedPipelineHistoryJob(t, pool, "agent", model.JobStatusPending)
	if err := repository.ValidateRuntimeAuthority(t.Context()); err == nil ||
		(!strings.Contains(err.Error(), "nonterminal job") && !strings.Contains(err.Error(), "database authority differs")) {
		t.Fatalf("startup executable-pipeline validation error=%v", err)
	}
}

func TestPostgresStartupRejectsTamperedExecutablePipelineAuthorityWithoutRows(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "087")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `ALTER TABLE jobs DISABLE TRIGGER jobs_executable_pipeline_authority`); err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidateRuntimeAuthority(t.Context()); err == nil || !strings.Contains(err.Error(), "database authority differs") {
		t.Fatalf("disabled trigger startup validation error=%v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE jobs ENABLE TRIGGER jobs_executable_pipeline_authority;
		DROP TRIGGER jobs_executable_pipeline_authority ON jobs;
		CREATE TRIGGER jobs_executable_pipeline_authority
		BEFORE INSERT OR UPDATE OR DELETE ON jobs
		FOR EACH ROW WHEN (false)
		EXECUTE FUNCTION enforce_jobs_executable_pipeline_authority()
	`); err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidateRuntimeAuthority(t.Context()); err == nil || !strings.Contains(err.Error(), "database authority differs") {
		t.Fatalf("conditional trigger startup validation error=%v", err)
	}
}

func seedPipelineHistoryJob(t *testing.T, pool *pgxpool.Pool, pipeline, status string) int64 {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	var id int64
	instruction := fmt.Sprintf("pipeline-history-%s-%d", pipeline, time.Now().UnixNano())
	if err := tx.QueryRow(t.Context(), `
		INSERT INTO jobs(instruction,pipeline,status,metadata)
		VALUES ($1,$2,$3,'{}'::jsonb)
		RETURNING id
	`, instruction, pipeline, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO job_generations(job_id,generation,purpose) VALUES ($1,1,'initial')
	`, id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return id
}

func containsPipelineHistory(jobs []model.Job, id int64, pipeline string) bool {
	for _, job := range jobs {
		if job.ID == id && job.Pipeline == pipeline && job.Status == model.JobStatusCompleted {
			return true
		}
	}
	return false
}

func assertSupportedPipelineDeleteIsReal(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	var id int64
	if err := tx.QueryRow(t.Context(), `
		INSERT INTO jobs(instruction,pipeline,status,metadata)
		VALUES ('supported delete probe','chat','pending','{}'::jsonb)
		RETURNING id
	`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	result, err := tx.Exec(t.Context(), `DELETE FROM jobs WHERE id=$1`, id)
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf("supported delete rows affected=%d want 1", result.RowsAffected())
	}
	var exists bool
	if err := tx.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1)`, id).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("supported deleted job %d remains present", id)
	}
}
