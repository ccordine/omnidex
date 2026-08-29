package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

const generatedProjectDeploymentTestSecretGeneration int64 = 1

var generatedProjectDeploymentTestKeyFingerprint = strings.Repeat("d", 64)

func newGeneratedProjectDeploymentHeadFixture(
	t *testing.T,
	label string,
) generatedDeploymentDatabaseFixture {
	t.Helper()
	pool := openIsolatedDatabasePool(t)
	repository, ctx := New(pool), t.Context()
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	location := t.TempDir()
	project, err := repository.CreateProject(ctx, "deployment-head-"+label, location, "")
	if err != nil {
		t.Fatal(err)
	}
	composeProject := fmt.Sprintf("project-%d-service", project.ID)
	return newGeneratedProjectDeploymentJobFixture(
		t, repository, pool, ctx, project.ID, location, label, composeProject, "", 0,
	)
}

func newGeneratedProjectDeploymentJobFixture(
	t *testing.T,
	repository *Repository,
	pool *pgxpool.Pool,
	ctx context.Context,
	projectID int64,
	location string,
	label string,
	composeProject string,
	priorDeploymentID string,
	endpointPort uint16,
) generatedDeploymentDatabaseFixture {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{
		"project_id": projectID, "client_cwd": location,
	})
	if err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("project-head-%s-%d", label, time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, metadata)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("project deployment claim=%+v err=%v want job %d", claim, err, job.ID)
	}
	command := generatedDeploymentTestCommand()
	command.Authority = GeneratedWorkloadDeploymentAuthority{
		JobID: job.ID, Generation: claim.Authority.Generation,
		StepID: claim.Authority.StepID, ProjectID: projectID,
	}
	command.ComposeProject = composeProject
	command.PriorDeploymentID = priorDeploymentID
	command.WorkspaceSHA256 = generatedDeploymentSHA(fmt.Sprintf("workspace:%d", job.ID))
	command.SourceSnapshotSHA256 = generatedDeploymentSHA(fmt.Sprintf("snapshot:%d", job.ID))
	command.SecretSetSHA256 = generatedDeploymentSHA(fmt.Sprintf(
		"project-secret:%d:generation:%d", projectID,
		generatedProjectDeploymentTestSecretGeneration,
	))
	command.ConfigSHA256 = generatedDeploymentSHA(
		"resolved-config\x00" + command.WorkspaceSHA256 + "\x00" + command.SecretSetSHA256,
	)
	command.EndpointPort = 0
	command.EndpointPortAuthority = GeneratedWorkloadDeploymentPortAllocate
	verification, firstEvidenceID := recordGeneratedDeploymentVerification(
		t, repository, ctx, claim.Authority, command,
		"verify project workspace", "docker compose config --hash=*",
	)
	manifest := generatedDeploymentTestManifest(command, false)
	rollback := generatedDeploymentTestRollbackPlan(command)
	receipt := generatedDeploymentTestReceipt(t, command)
	if endpointPort == 0 {
		endpointPort = uint16(22000 + projectID%30000)
	}
	receipt.EndpointPort = endpointPort
	receipt.WorkspaceVerificationReceiptID = verification.ID
	return generatedDeploymentDatabaseFixture{
		repository: repository, pool: pool, ctx: ctx, projectID: projectID,
		jobID: job.ID, authority: claim.Authority, command: command,
		verification: verification, manifest: manifest, rollback: rollback,
		evidenceID: firstEvidenceID, receipt: receipt,
	}
}

func reserveSealAndPromoteGeneratedProjectDeployment(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	expectation GeneratedWorkloadProjectDeploymentHeadExpectation,
) (GeneratedWorkloadProjectDeploymentReservation, GeneratedWorkloadProjectDeploymentHead) {
	t.Helper()
	fixture.prepare(t, fixture.authority, fixture.verification.ID)
	reservation, err := fixture.repository.ReserveGeneratedWorkloadProjectDeploymentCandidate(
		fixture.ctx, fixture.authority, fixture.command,
		generatedProjectDeploymentTestSecretGeneration,
		generatedProjectDeploymentTestKeyFingerprint, expectation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying},
	); err != nil {
		t.Fatal(err)
	}
	receipt := fixture.executeSuccessfulRail(t, fixture.authority)
	if _, err := fixture.repository.SealGeneratedWorkloadDeploymentApplied(
		fixture.ctx, fixture.authority, fixture.command, receipt,
	); err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	head, err := sealGeneratedWorkloadProjectDeploymentHeadTx(
		fixture.ctx, tx, fixture.authority, fixture.command, receipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	return reservation, head
}
