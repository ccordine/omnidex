package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workspaceMutationProjectLocationFixture struct {
	claim           *model.ClaimedStep
	command         WorkspaceMutationCommand
	identity        workspaceMutationOperationIdentity
	projectLocation string
	runtimeRoot     string
}

func newWorkspaceMutationProjectLocationFixture(
	t *testing.T,
	repository *Repository,
	label string,
) workspaceMutationProjectLocationFixture {
	t.Helper()
	projectLocation := t.TempDir()
	runtimeRoot := t.TempDir()
	job, projectID := enqueueWorkspaceMutationPipelineActionJob(
		t, repository, model.PipelineCoding, "location-179-"+label, projectLocation,
	)
	claim, err := repository.ClaimNextStep(t.Context(), "location-179-worker-"+label)
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("workspace project-location claim=%+v error=%v", claim, err)
	}
	if claim.Job.Pipeline != model.PipelineCoding || claim.Step.Action != "v3_coding" {
		t.Fatalf("workspace project-location claim=%s/%s", claim.Job.Pipeline, claim.Step.Action)
	}

	source, err := workspacefacts.Capture(t.Context(), runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		t.Context(), source, "objective_"+queueTestSHA256("location-179-"+label),
		[]workspacefacts.DesiredFileState{{
			Path: "generated/" + label + ".txt", Present: true,
			Content: []byte("generated " + label + "\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	verification, err := NewWorkspaceMutationVerificationPlan(
		[]WorkspaceMutationVerificationIntent{{
			Kind: evidence.KindTestResult, Command: "verify " + label,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	command := WorkspaceMutationCommand{
		JobID: job.ID, StepID: claim.Step.ID, Generation: claim.Authority.Generation,
		CreatorAttempt: claim.Authority.Attempt, CreatorWorkerID: claim.Authority.WorkerID,
		ProjectID: projectID, ProjectLocation: projectLocation,
		Plan: plan, Verification: verification,
	}
	identity, err := workspaceMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	if projectLocation == runtimeRoot || plan.WorkspaceRoot != runtimeRoot {
		t.Fatalf("fixture project/runtime locations=%q/%q", projectLocation, plan.WorkspaceRoot)
	}
	return workspaceMutationProjectLocationFixture{
		claim: claim, command: command, identity: identity,
		projectLocation: projectLocation, runtimeRoot: runtimeRoot,
	}
}

func insertWorkspaceMutationProjectLocationDirect(
	t *testing.T,
	pool *pgxpool.Pool,
	fixture workspaceMutationProjectLocationFixture,
	projectLocation string,
) error {
	t.Helper()
	var gitSource any
	if fixture.command.Plan.GitSourceSnapshotID != "" {
		gitSource = fixture.command.Plan.GitSourceSnapshotID
	}
	_, err := pool.Exec(t.Context(), `
		INSERT INTO workspace_mutation_operations (
			id,command_sha256,job_id,generation,step_id,
			creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id,
			project_id,project_location,owner_id,stage_id,workspace_id,workspace_root,
			source_state_id,expected_state_id,source_repository_snapshot_id,
			patch,patch_sha256,verification_plan_json,verification_plan_sha256,status
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,
			$17,$18,$19,$20,'prepared'
		)
	`, fixture.identity.ID, fixture.identity.CommandSHA256,
		fixture.command.JobID, fixture.command.Generation, fixture.command.StepID,
		fixture.command.CreatorAttempt, fixture.command.CreatorWorkerID,
		fixture.command.ProjectID, projectLocation, fixture.command.Plan.OwnerID,
		fixture.command.Plan.ID, fixture.command.Plan.WorkspaceID,
		fixture.command.Plan.WorkspaceRoot, fixture.command.Plan.SourceStateID,
		fixture.command.Plan.ExpectedStateID, gitSource, fixture.command.Plan.Patch,
		fixture.command.Plan.PatchSHA256, fixture.identity.PlanJSON, fixture.identity.PlanSHA256,
	)
	return err
}

func assertWorkspaceMutationProjectLocationCatalog(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var columnCount int
	var columnExact bool
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*),COALESCE(bool_and(
			data_type='text' AND udt_name='text' AND is_nullable='NO' AND
			column_default IS NULL AND character_maximum_length IS NULL
		),FALSE)
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='workspace_mutation_operations'
		  AND column_name='project_location'
	`).Scan(&columnCount, &columnExact); err != nil {
		t.Fatal(err)
	}
	if columnCount != 1 || !columnExact {
		t.Fatalf("workspace project-location column catalog count/exact=%d/%t", columnCount, columnExact)
	}

	assertWorkspaceMutationProjectLocationFunction(
		t, pool, "validate_workspace_mutation_insert()",
		workspaceMutationProjectLocationInsertFunctionSHA256, 0, "trigger", "plpgsql",
	)
	assertWorkspaceMutationProjectLocationFunction(
		t, pool, "workspace_mutation_current_authority_valid(workspace_mutation_operations)",
		workspaceMutationProjectLocationCurrentFunctionSHA256, 1, "boolean", "sql",
	)
	assertWorkspaceMutationProjectLocationFunction(
		t, pool, "prevent_workspace_mutation_project_location_change()",
		workspaceMutationProjectLocationImmutableFunctionSHA256, 0, "trigger", "plpgsql",
	)
	assertWorkspaceMutationProjectLocationFunction(
		t, pool, "validate_workspace_mutation_update()",
		workspaceMutationProjectLocationUpdateFunctionSHA256,
		0, "trigger", "plpgsql",
	)
	assertWorkspaceMutationProjectLocationFunction(
		t, pool, "prevent_project_location_change_during_active_work()",
		workspaceMutationProjectLocationChangeGuardSHA256, 0, "trigger", "plpgsql",
	)
	assertWorkspaceMutationProjectLocationTrigger(
		t, pool, "workspace_mutation_operations", "workspace_mutation_insert_validate",
		"validate_workspace_mutation_insert()", 7,
	)
	assertWorkspaceMutationProjectLocationTrigger(
		t, pool, "workspace_mutation_operations", "workspace_mutation_update_validate",
		"validate_workspace_mutation_update()", 19,
	)
	assertWorkspaceMutationProjectLocationTrigger(
		t, pool, "workspace_mutation_operations", "workspace_mutation_project_location_immutable",
		"prevent_workspace_mutation_project_location_change()", 19,
	)
	assertWorkspaceMutationProjectLocationTrigger(
		t, pool, "projects", "projects_active_work_location_guard",
		"prevent_project_location_change_during_active_work()", 19,
	)

	var constraintCount int
	var constraintExact bool
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*),COALESCE(bool_and(
			contype='c' AND convalidated AND conislocal AND coninhcount=0 AND
			NOT condeferrable AND NOT condeferred AND NOT connoinherit AND
			array_length(conkey,1)=1 AND (
				SELECT attribute.attname
				FROM pg_attribute AS attribute
				WHERE attribute.attrelid=constraint_row.conrelid
				  AND attribute.attnum=constraint_row.conkey[1]
			)='project_location'
		),FALSE)
		FROM pg_constraint AS constraint_row
		WHERE conrelid='workspace_mutation_operations'::regclass
		  AND conname='workspace_mutation_project_location_valid'
	`).Scan(&constraintCount, &constraintExact); err != nil {
		t.Fatal(err)
	}
	if constraintCount != 1 || !constraintExact {
		t.Fatalf("workspace project-location constraint catalog count/exact=%d/%t", constraintCount, constraintExact)
	}
}

func assertWorkspaceMutationProjectLocationFunction(
	t *testing.T,
	pool *pgxpool.Pool,
	signature, digest string,
	arguments int,
	returnType, languageName string,
) {
	t.Helper()
	var count int
	var exact bool
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*),COALESCE(bool_and(
			encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=$2 AND
			procedure.prokind='f' AND procedure.pronargs=$3 AND
			procedure.pronargdefaults=0 AND procedure.prorettype::regtype::text=$4 AND
			NOT procedure.proretset AND language.lanname=$5 AND
			procedure.provolatile='v' AND procedure.proparallel='u' AND
			NOT procedure.proisstrict AND NOT procedure.prosecdef AND
			NOT procedure.proleakproof AND procedure.proconfig IS NULL
		),FALSE)
		FROM pg_proc AS procedure
		JOIN pg_namespace AS namespace ON namespace.oid=procedure.pronamespace
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE namespace.nspname=current_schema()
		  AND procedure.oid=to_regprocedure(current_schema() || '.' || $1)
	`, signature, digest, arguments, returnType, languageName).Scan(&count, &exact); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !exact {
		t.Fatalf("workspace project-location function %q count/exact=%d/%t", signature, count, exact)
	}
}

func assertWorkspaceMutationProjectLocationTrigger(
	t *testing.T,
	pool *pgxpool.Pool,
	relation, name, functionSignature string,
	typeBits int,
) {
	t.Helper()
	var count int
	var exact bool
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*),COALESCE(bool_and(
			trigger_row.tgenabled='O' AND trigger_row.tgtype=$4 AND
			trigger_row.tgattr::text='' AND trigger_row.tgqual IS NULL AND
			trigger_row.tgconstraint=0 AND trigger_row.tgconstrrelid=0 AND
			trigger_row.tgconstrindid=0 AND NOT trigger_row.tgdeferrable AND
			NOT trigger_row.tginitdeferred AND trigger_row.tgnargs=0 AND
			octet_length(trigger_row.tgargs)=0 AND trigger_row.tgoldtable IS NULL AND
			trigger_row.tgnewtable IS NULL AND NOT trigger_row.tgisinternal AND
			trigger_row.tgfoid=to_regprocedure(current_schema() || '.' || $3)
		),FALSE)
		FROM pg_trigger AS trigger_row
		WHERE trigger_row.tgrelid=to_regclass(current_schema() || '.' || $1)
		  AND trigger_row.tgname=$2
	`, relation, name, functionSignature, typeBits).Scan(&count, &exact); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !exact {
		t.Fatalf("workspace project-location trigger %q count/exact=%d/%t", name, count, exact)
	}
}
