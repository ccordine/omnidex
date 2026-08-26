package queue

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const generatedWorkloadDeploymentNamespacePreflightMigration = "145_generated_workload_deployment_namespace_preflight.sql"

func TestGeneratedDeploymentNamespacePreflightMigrationDefinesExactVacantProofRail(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + generatedWorkloadDeploymentNamespacePreflightMigration)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(raw), "\n"); lines >= 180 {
		t.Fatalf("deployment namespace preflight migration has %d lines; maximum is 179", lines)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"generated_deployment_vacant_namespace_preflight_valid",
		"docker_compose_resolved_config",
		"namespace_preflight",
		"omnidex.generated-deployment-namespace-preflight.v1",
		"proof->'container_ids' IS DISTINCT FROM '[]'::JSONB",
		"proof->'network_ids' IS DISTINCT FROM '[]'::JSONB",
		"proof->'volume_names' IS DISTINCT FROM '[]'::JSONB",
		"proof->>'schema' IS DISTINCT FROM",
		"proof->>'compose_project' IS DISTINCT FROM project",
		"proof->>'sha256' IS NULL",
		"encode(digest(convert_to(expected,'UTF8'),'sha256'),'hex')",
		"requires a vacant proof for every existing deployment binding",
		"generated_deployment_binding_namespace_preflight_validate",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("namespace preflight migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"DELETE FROM", "docker compose down", "adopt", "cleanup"} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("namespace preflight migration contains forbidden recovery %q", forbidden)
		}
	}
}

func TestGeneratedDeploymentNamespacePreflightSQLRejectsJSONNullAuthority(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixtureAtPrefix(t, "namespace-null", "145")
	proof, _, err := BindGeneratedWorkloadDeploymentNamespacePreflight(
		GeneratedWorkloadDeploymentNamespacePreflight{
			Schema:         GeneratedWorkloadDeploymentNamespacePreflightV1,
			ComposeProject: fixture.command.ComposeProject,
			ContainerIDs:   []string{},
			NetworkIDs:     []string{},
			VolumeNames:    []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"schema", "compose_project", "sha256"} {
		t.Run(field, func(t *testing.T) {
			raw, err := json.Marshal(proof)
			if err != nil {
				t.Fatal(err)
			}
			var candidate map[string]any
			if err := json.Unmarshal(raw, &candidate); err != nil {
				t.Fatal(err)
			}
			candidate[field] = nil
			metadata, err := json.Marshal(map[string]any{
				GeneratedWorkloadDeploymentNamespaceMetadataKey: candidate,
			})
			if err != nil {
				t.Fatal(err)
			}
			var valid bool
			err = fixture.pool.QueryRow(fixture.ctx, `
				SELECT generated_deployment_vacant_namespace_preflight_valid($1,$2,$3::jsonb)
			`, fixture.command.ComposeProject, GeneratedWorkloadResolvedConfigEvidenceSource,
				string(metadata)).Scan(&valid)
			if err != nil {
				t.Fatal(err)
			}
			if valid {
				t.Fatalf("SQL namespace preflight accepted JSON null %s authority", field)
			}
		})
	}
}

func TestGeneratedDeploymentPreparationRejectsOmittedNamespaceProofInGoAndSQL(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixtureAtPrefix(t, "namespace-boundary", "145")
	verification, _ := recordGeneratedDeploymentVerificationWithNamespace(
		t, fixture.repository, fixture.ctx, fixture.authority, fixture.command, false,
		"verify workspace without namespace proof", "docker compose config --hash=*",
	)
	_, err := fixture.repository.PrepareGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command, verification.ID,
		fixture.manifest, fixture.rollback,
	)
	if err == nil || !strings.Contains(err.Error(), "namespace preflight") {
		t.Fatalf("Go preparation boundary error=%v", err)
	}
	if err := insertGeneratedDeploymentBindingDirect(fixture, verification.ID); err == nil ||
		!strings.Contains(err.Error(), "vacant Compose namespace preflight proof") {
		t.Fatalf("SQL binding boundary error=%v", err)
	}
	assertGeneratedDeploymentCount(t, fixture, 0)
}

func TestGeneratedDeploymentNamespacePreflightMigrationRejectsLegacyUnprovenBinding(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixtureAtPrefix(t, "namespace-upgrade", "144")
	verification, _ := recordGeneratedDeploymentVerificationWithNamespace(
		t, fixture.repository, fixture.ctx, fixture.authority, fixture.command, false,
		"verify legacy workspace", "docker compose config --hash=*",
	)
	if err := insertGeneratedDeploymentBindingDirect(fixture, verification.ID); err != nil {
		t.Fatalf("install pre-145 unproven binding: %v", err)
	}
	err := fixture.repository.EnsureSchema(
		fixture.ctx, loadMigrationBundleThroughPrefix(t, "145"),
	)
	if err == nil || !strings.Contains(err.Error(), "requires a vacant proof for every existing deployment binding") {
		t.Fatalf("namespace preflight upgrade error=%v", err)
	}
}

func insertGeneratedDeploymentBindingDirect(
	fixture generatedDeploymentDatabaseFixture,
	verificationID string,
) error {
	identity, err := generatedWorkloadDeploymentOperation(fixture.command)
	if err != nil {
		return err
	}
	manifestJSON, manifestSHA, err := canonicalGeneratedDeploymentLifecycleManifest(
		fixture.command, fixture.manifest,
	)
	if err != nil {
		return err
	}
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(fixture.ctx)
	_, err = tx.Exec(fixture.ctx, `
		INSERT INTO generated_workload_deployments(
		 id,command_sha256,command_json,job_id,generation,step_id,
		 creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id,
		 project_id,compose_project,bind_host,endpoint_port_authority,
		 requested_endpoint_port,prior_deployment_id,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),'prepared')
	`, identity.OperationID, identity.CommandSHA256, identity.CommandJSON,
		fixture.command.Authority.JobID, fixture.command.Authority.Generation,
		fixture.command.Authority.StepID, fixture.authority.Attempt, fixture.authority.WorkerID,
		fixture.command.Authority.ProjectID, fixture.command.ComposeProject,
		fixture.command.BindHost, fixture.command.EndpointPortAuthority,
		fixture.command.EndpointPort, fixture.command.PriorDeploymentID)
	if err != nil {
		return err
	}
	plan := fixture.rollback
	_, err = tx.Exec(fixture.ctx, `
		INSERT INTO generated_workload_deployment_rollback_plans(
		 operation_id,policy,max_attempts,slot_name,slot_ordinal,command_sha256,workspace_sha256,
		 compose_project,resource_observation,require_container_absence,
		 require_network_absence,require_volume_absence,state_marker_sha256,
		 postcondition_json,postcondition_sha256)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''),$14,$15)
	`, identity.OperationID, plan.Policy, plan.MaxAttempts, plan.Execution.Slot.Name,
		plan.Execution.Slot.Ordinal, plan.Execution.CommandSHA256,
		plan.Execution.WorkspaceSHA256, plan.ComposeProject, plan.ResourceObservation,
		plan.RequireContainerAbsence, plan.RequireNetworkAbsence,
		plan.RequireVolumeAbsence, plan.StateMarkerSHA256,
		plan.PostconditionJSON, plan.PostconditionSHA256)
	if err != nil {
		return err
	}
	_, err = tx.Exec(fixture.ctx, `
		INSERT INTO generated_workload_deployment_verifications(
		 operation_id,verification_id,workspace_sha256,lifecycle_manifest_json,lifecycle_manifest_sha256)
		VALUES($1,$2,$3,$4,$5)
	`, identity.OperationID, verificationID, fixture.command.WorkspaceSHA256,
		manifestJSON, manifestSHA)
	if err != nil {
		return err
	}
	return tx.Commit(fixture.ctx)
}

func assertGeneratedDeploymentCount(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	want int,
) {
	t.Helper()
	var count int
	err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT count(*) FROM generated_workload_deployments WHERE job_id=$1
	`, fixture.jobID).Scan(&count)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("deployment rows=%d want %d", count, want)
	}
}
