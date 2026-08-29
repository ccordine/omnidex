package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type legacyRepositoryMutationCutoverFixture struct {
	repository  *Repository
	pool        *pgxpool.Pool
	ctx         context.Context
	jobID       int64
	stepID      int64
	operationID string
	stageID     string
	patchSHA256 string
}

func newLegacyRepositoryMutationCutoverFixture(
	t *testing.T,
	label string,
) legacyRepositoryMutationCutoverFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "158")); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	runQueueRepositoryGit(t, root, "init")
	runQueueRepositoryGit(t, root, "config", "user.email", "workspace-cutover@example.test")
	runQueueRepositoryGit(t, root, "config", "user.name", "Workspace Cutover Test")
	if err := os.WriteFile(filepath.Join(root, "value.go"), []byte("source-value"), 0o644); err != nil {
		t.Fatal(err)
	}
	runQueueRepositoryGit(t, root, "add", "value.go")
	runQueueRepositoryGit(t, root, "commit", "-m", "source")
	snapshot, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(ctx, "workspace-cutover-"+label, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.StoreRepositorySnapshot(ctx, project.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("workspace-cutover-%s-%d", label, time.Now().UnixNano())
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
		t.Fatalf("workspace cutover claim=%+v want job %d", claim, job.ID)
	}
	file := snapshot.Files[0]
	commandSHA := workspaceCutoverDigest("legacy-command-" + label)
	operationID := "repository_mutation_" + commandSHA
	contractID := "change_contract_" + workspaceCutoverDigest("legacy-contract-"+label)
	stageID := "repository_change_stage_" + workspaceCutoverDigest("legacy-stage-"+label)
	patch := "legacy repository mutation patch " + label
	patchSHA := workspaceCutoverDigest(patch)
	expected := []byte("legacy expected value " + label)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO repository_mutation_operations (
			id,command_sha256,job_id,step_id,generation,worker_id,
			contract_id,stage_id,source_snapshot_id,patch,patch_sha256,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'prepared')
	`, operationID, commandSHA, job.ID, claim.Step.ID, claim.Step.Generation,
		claim.Step.WorkerID, contractID, stageID, snapshot.ID, patch, patchSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO repository_mutation_files (
			operation_id,ordinal,file_id,path,
			source_present,source_sha256,source_size,source_mode,
			expected_present,expected_sha256,expected_size,expected_mode
		) VALUES ($1,0,$2,$3,TRUE,$4,$5,$6,TRUE,$7,$8,$6)
	`, operationID, file.ID, file.Path, file.SHA256, file.Size, file.Mode,
		workspaceCutoverDigest(string(expected)), len(expected)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE repository_mutation_operations
		SET sealed_at=clock_timestamp()
		WHERE id=$1
	`, operationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return legacyRepositoryMutationCutoverFixture{
		repository: repository, pool: pool, ctx: ctx,
		jobID: job.ID, stepID: claim.Step.ID,
		operationID: operationID, stageID: stageID, patchSHA256: patchSHA,
	}
}

func sealAppliedLegacyRepositoryMutation(
	t *testing.T,
	fixture legacyRepositoryMutationCutoverFixture,
) {
	t.Helper()
	var evidenceID int64
	if err := fixture.pool.QueryRow(fixture.ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,'generated_diff','repository',$3,jsonb_build_object(
			'hash',$4::text,'metadata',jsonb_build_object(
				'repository_mutation_operation_id',$5::text
			)
		)) RETURNING id
	`, fixture.jobID, fixture.stepID, fixture.stageID,
		fixture.patchSHA256, fixture.operationID).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE repository_mutation_operations
		SET status='applied',evidence_id=$2,applied_at=clock_timestamp(),
		    last_error=NULL,updated_at=clock_timestamp()
		WHERE id=$1 AND status='prepared'
	`, fixture.operationID, evidenceID)
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf("seal applied legacy repository mutation changed %d rows", result.RowsAffected())
	}
}

func workspaceCutoverDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func workspaceCutoverOpaqueID(prefix string, values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(hash.Sum(nil))
}
