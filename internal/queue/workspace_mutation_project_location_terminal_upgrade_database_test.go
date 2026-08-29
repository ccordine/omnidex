package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func TestPostgresWorkspaceMutationProjectLocationUpgradePreservesTerminalHistory(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "178"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := newWorkspaceMutationPipelineActionFixture(
		t, repository, model.PipelineCoding, "location-179-terminal-history",
	)
	if _, err := prepareWorkspaceMutationBeforeProjectLocation(t, fixture); err != nil {
		t.Fatal(err)
	}
	terminalizeWorkspaceMutationBeforeProjectLocation(t, fixture)
	if err := repository.CompleteStep(t.Context(), CompleteStepCommand{
		OperationID: testLifecycleOperationID(
			t, "location-179-terminal-upgrade", fixture.command.StepID,
		),
		Authority: fixture.claim.Authority, StepID: fixture.command.StepID,
		Output: "verified historical workspace mutation",
	}); err != nil {
		t.Fatal(err)
	}

	operationTimeRoot := fixture.command.Plan.WorkspaceRoot
	currentProjectLocation := t.TempDir()
	if currentProjectLocation == operationTimeRoot {
		t.Fatal("relocated project location is not distinct")
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE projects SET location=$2 WHERE id=$1
	`, fixture.command.ProjectID, currentProjectLocation); err != nil {
		t.Fatal(err)
	}
	var priorStatus, priorRoot, relocatedProject, jobStatus, stepStatus string
	if err := pool.QueryRow(t.Context(), `
		SELECT operation.status,operation.workspace_root,projects.location,
		       jobs.status,steps.status
		FROM workspace_mutation_operations AS operation
		JOIN projects ON projects.id=operation.project_id
		JOIN jobs ON jobs.id=operation.job_id
		JOIN job_steps AS steps ON steps.id=operation.step_id
		WHERE operation.id=$1
	`, fixture.identity.ID).Scan(
		&priorStatus, &priorRoot, &relocatedProject, &jobStatus, &stepStatus,
	); err != nil {
		t.Fatal(err)
	}
	if priorStatus != workspaceMutationVerified || priorRoot != operationTimeRoot ||
		relocatedProject != currentProjectLocation || priorRoot == relocatedProject ||
		jobStatus != model.JobStatusCompleted || stepStatus != model.StepStatusCompleted {
		t.Fatalf(
			"schema-178 terminal history mutation/job/step=%q/%q/%q root/project=%q/%q",
			priorStatus, jobStatus, stepStatus, priorRoot, relocatedProject,
		)
	}

	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, workspaceMutationProjectLocationAuthorityMigration, 1)
	assertWorkspaceMutationProjectLocationCatalog(t, pool)
	if err := repository.ValidateRuntimeAuthority(t.Context()); err != nil {
		t.Fatalf("validate preserved terminal project-location history: %v", err)
	}

	var status, workspaceRoot, projectLocation, currentLocation string
	var mutationEvidenceID, verificationEvidenceID int64
	var terminal, sealed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT operation.status,operation.workspace_root,operation.project_location,
		       projects.location,operation.mutation_evidence_id,
		       operation.verification_evidence_id,
		       operation.terminal_at IS NOT NULL,operation.sealed_at IS NOT NULL
		FROM workspace_mutation_operations AS operation
		JOIN projects ON projects.id=operation.project_id
		WHERE operation.id=$1
	`, fixture.identity.ID).Scan(
		&status, &workspaceRoot, &projectLocation, &currentLocation,
		&mutationEvidenceID, &verificationEvidenceID, &terminal, &sealed,
	); err != nil {
		t.Fatal(err)
	}
	if status != workspaceMutationVerified || workspaceRoot != operationTimeRoot ||
		projectLocation != operationTimeRoot || currentLocation != currentProjectLocation ||
		projectLocation == currentLocation || mutationEvidenceID <= 0 ||
		verificationEvidenceID <= mutationEvidenceID || !terminal || !sealed {
		t.Fatalf(
			"upgraded terminal history=%q roots=%q/%q/%q evidence=%d/%d terminal/sealed=%t/%t",
			status, workspaceRoot, projectLocation, currentLocation,
			mutationEvidenceID, verificationEvidenceID, terminal, sealed,
		)
	}
}

func terminalizeWorkspaceMutationBeforeProjectLocation(
	t *testing.T,
	fixture workspaceMutationPipelineActionFixture,
) {
	t.Helper()
	tx, err := fixture.commandDatabase().BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	transition, err := tx.Exec(t.Context(), `
		UPDATE workspace_mutation_operations
		SET status='applying',apply_attempt_count=1,
		    applying_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1 AND status='prepared'
	`, fixture.identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if transition.RowsAffected() != 1 {
		t.Fatalf("schema-178 applying transition affected %d rows", transition.RowsAffected())
	}
	mutationEvidenceID, err := insertWorkspaceMutationEvidenceTx(
		t.Context(), tx, fixture.command, fixture.identity.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	transition, err = tx.Exec(t.Context(), `
		UPDATE workspace_mutation_operations
		SET status='applied',mutation_evidence_id=$2,
		    applied_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1 AND status='applying'
	`, fixture.identity.ID, mutationEvidenceID)
	if err != nil {
		t.Fatal(err)
	}
	if transition.RowsAffected() != 1 {
		t.Fatalf("schema-178 applied transition affected %d rows", transition.RowsAffected())
	}
	transition, err = tx.Exec(t.Context(), `
		UPDATE workspace_mutation_operations
		SET status='verifying',verification_attempt_count=1,
		    verifying_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1 AND status='applied'
	`, fixture.identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if transition.RowsAffected() != 1 {
		t.Fatalf("schema-178 verifying transition affected %d rows", transition.RowsAffected())
	}

	if len(fixture.command.Verification.Commands) != 1 {
		t.Fatalf(
			"schema-178 terminal fixture has %d verification commands",
			len(fixture.command.Verification.Commands),
		)
	}
	planned := fixture.command.Verification.Commands[0]
	commandRecord := evidence.Record{
		JobID: fixture.command.JobID, StepID: fixture.command.StepID,
		Kind: planned.Kind, SourceType: "workspace_verification",
		SourceRef: fixture.identity.ID, Command: planned.Command,
		Metadata: map[string]any{"succeeded": true},
	}
	commandPayload, err := json.Marshal(commandRecord)
	if err != nil {
		t.Fatal(err)
	}
	var commandEvidenceID int64
	if err := tx.QueryRow(t.Context(), `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, fixture.command.JobID, fixture.command.StepID, commandRecord.Kind,
		commandRecord.SourceType, commandRecord.SourceRef, string(commandPayload),
	).Scan(&commandEvidenceID); err != nil {
		t.Fatal(err)
	}
	if commandEvidenceID <= mutationEvidenceID {
		t.Fatalf(
			"schema-178 command evidence identity=%d after mutation evidence=%d",
			commandEvidenceID, mutationEvidenceID,
		)
	}
	receipt := workspaceMutationVerificationReceipt{
		Schema: workspaceMutationReceiptSchema, OperationID: fixture.identity.ID,
		SourceStateID:   fixture.command.Plan.SourceStateID,
		ExpectedStateID: fixture.command.Plan.ExpectedStateID,
		ObservedStateID: fixture.command.Plan.ExpectedStateID,
		Succeeded:       true, CommandEvidenceIDs: []int64{commandEvidenceID},
	}
	receiptRaw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receiptJSON := string(receiptRaw)
	receiptSHA := digestWorkspaceMutationText(receiptJSON)
	receiptRecord := evidence.Record{
		JobID: fixture.command.JobID, StepID: fixture.command.StepID,
		Kind: evidence.KindWorkspaceVerification, SourceType: "workspace_mutation",
		SourceRef: fixture.identity.ID, Hash: receiptSHA, Excerpt: receiptJSON,
		Metadata: map[string]any{
			"workspace_mutation_operation_id": fixture.identity.ID,
			"observed_state_id":               fixture.command.Plan.ExpectedStateID,
			"succeeded":                       true,
		},
	}
	receiptPayload, err := json.Marshal(receiptRecord)
	if err != nil {
		t.Fatal(err)
	}
	var receiptEvidenceID int64
	if err := tx.QueryRow(t.Context(), `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, fixture.command.JobID, fixture.command.StepID, receiptRecord.Kind,
		receiptRecord.SourceType, receiptRecord.SourceRef, string(receiptPayload),
	).Scan(&receiptEvidenceID); err != nil {
		t.Fatal(err)
	}
	if receiptEvidenceID <= commandEvidenceID {
		t.Fatalf(
			"schema-178 receipt evidence identity=%d after command evidence=%d",
			receiptEvidenceID, commandEvidenceID,
		)
	}
	transition, err = tx.Exec(t.Context(), `
		UPDATE workspace_mutation_operations
		SET status='verified',verification_succeeded=TRUE,
		    verification_receipt_json=$2,verification_receipt_sha256=$3,
		    verification_evidence_id=$4,terminal_at=clock_timestamp(),
		    updated_at=clock_timestamp()
		WHERE id=$1 AND status='verifying'
	`, fixture.identity.ID, receiptJSON, receiptSHA, receiptEvidenceID)
	if err != nil {
		t.Fatal(err)
	}
	if transition.RowsAffected() != 1 {
		t.Fatalf("schema-178 terminal transition affected %d rows", transition.RowsAffected())
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}
