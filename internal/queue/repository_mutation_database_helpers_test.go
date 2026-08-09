package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repositoryMutationDatabaseFixture struct {
	repository *Repository
	pool       *pgxpool.Pool
	ctx        context.Context
	job        model.Job
	stepID     int64
	workerID   string
	command    RepositoryMutationCommand
}

func newRepositoryMutationDatabaseFixture(
	t *testing.T,
	label string,
) repositoryMutationDatabaseFixture {
	t.Helper()
	repository, pool, ctx := replanTestRepository(t)
	root := t.TempDir()
	runQueueRepositoryGit(t, root, "init")
	runQueueRepositoryGit(t, root, "config", "user.email", "mutation@example.test")
	runQueueRepositoryGit(t, root, "config", "user.name", "Mutation Test")
	source := []byte("source-value")
	if err := os.WriteFile(filepath.Join(root, "value.go"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	runQueueRepositoryGit(t, root, "add", "value.go")
	runQueueRepositoryGit(t, root, "commit", "-m", "source")
	snapshot, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(
		ctx, "mutation-"+label, root, "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.StoreRepositorySnapshot(ctx, project.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("repository-mutation-%s-%d", label, time.Now().UnixNano())
	metadata := []byte(fmt.Sprintf(`{"project_id":%d,"client_cwd":%q}`, project.ID, root))
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, metadata)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("repository mutation claim=%+v want job %d", claim, job.ID)
	}
	file := snapshot.Files[0]
	command := repositoryMutationTestCommand(
		job.ID, claim.Step.ID, claim.Step.Generation, claim.Step.WorkerID, "return two",
	)
	command.SourceSnapshotID = snapshot.ID
	command.ChangedFiles[0].FileID = file.ID
	command.ChangedFiles[0].Path = file.Path
	command.ChangedFiles[0].SourceSHA256 = file.SHA256
	command.ChangedFiles[0].SourceSize = file.Size
	t.Cleanup(func() {
		_, _ = repository.CancelJob(context.Background(), testCancelCommand(
			t, job.ID, "mutation-database-cleanup", "close repository mutation database fixture",
		))
	})
	return repositoryMutationDatabaseFixture{
		repository: repository, pool: pool, ctx: ctx, job: job,
		stepID: claim.Step.ID, workerID: claim.Step.WorkerID, command: command,
	}
}

func stateClassifier(
	state *atomic.Value,
) RepositoryMutationClassifier {
	return func(context.Context, RepositoryMutationCommand) (RepositoryMutationState, error) {
		return state.Load().(RepositoryMutationState), nil
	}
}

func assertRepositoryMutationEvidenceCount(
	t *testing.T,
	pool *pgxpool.Pool,
	jobID int64,
	want int,
) {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM evidence
		WHERE job_id=$1 AND kind=$2 AND source_type='repository'
	`, jobID, evidence.KindGeneratedDiff).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("repository mutation evidence count=%d want %d", count, want)
	}
}

func repositoryMutationJournalStatus(
	t *testing.T,
	fixture repositoryMutationDatabaseFixture,
) (string, int, *int64) {
	t.Helper()
	operation, err := repositoryMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	var evidenceID *int64
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT status, attempt_count, evidence_id
		FROM repository_mutation_operations WHERE id=$1
	`, operation.ID).Scan(&status, &attempts, &evidenceID); err != nil {
		t.Fatal(err)
	}
	return status, attempts, evidenceID
}

func assertRepositoryMutationEventCount(
	t *testing.T,
	fixture repositoryMutationDatabaseFixture,
	eventType string,
	want int,
) {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT COUNT(*)
		FROM omni_run_events AS event
		WHERE event.run_id=NULLIF((
			SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id=$1
		), '')::uuid AND event.event_type=$2
	`, fixture.job.ID, eventType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("repository mutation event %q count=%d want %d", eventType, count, want)
	}
}
