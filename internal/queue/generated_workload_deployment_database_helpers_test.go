package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type generatedDeploymentDatabaseFixture struct {
	repository   *Repository
	pool         *pgxpool.Pool
	ctx          context.Context
	projectID    int64
	jobID        int64
	authority    model.StepAttemptAuthority
	command      GeneratedWorkloadDeploymentCommand
	verification GeneratedWorkloadVerificationRecord
	manifest     GeneratedWorkloadDeploymentLifecycleManifest
	rollback     GeneratedWorkloadDeploymentRollbackPlan
	evidenceID   int64
	receipt      GeneratedWorkloadDeploymentReceipt
	rawSecret    string
	rawConfig    string
}

func newGeneratedDeploymentDatabaseFixture(
	t *testing.T, label string,
) generatedDeploymentDatabaseFixture {
	t.Helper()
	pool := openIsolatedDatabasePool(t)
	repository, ctx := New(pool), t.Context()
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	location := t.TempDir()
	project, err := repository.CreateProject(ctx, "deployment-"+label, location, "")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]any{"project_id": project.ID, "client_cwd": location})
	if err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("generated-deployment-%s-%d", label, time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, metadata)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("generated deployment claim=%+v err=%v want job %d", claim, err, job.ID)
	}
	command := generatedDeploymentTestCommand()
	command.Authority = GeneratedWorkloadDeploymentAuthority{
		JobID: job.ID, Generation: claim.Authority.Generation, StepID: claim.Authority.StepID,
		ProjectID: project.ID,
	}
	command.ComposeProject = fmt.Sprintf("generated-%d-g%d", job.ID, claim.Authority.Generation)
	command.EndpointPort, command.EndpointPortAuthority = 0, GeneratedWorkloadDeploymentPortAllocate
	rawSecret := "never-store-secret-value-" + label
	rawConfig := "services:\n  api:\n    environment:\n      PASSWORD: " + rawSecret
	command.SecretSetSHA256 = generatedDeploymentSHA(rawSecret)
	command.ConfigSHA256 = generatedDeploymentSHA("resolved-config\x00" + command.WorkspaceSHA256 + "\x00" + command.SecretSetSHA256)
	verification, firstEvidenceID := recordGeneratedDeploymentVerification(
		t, repository, ctx, claim.Authority, command, "verify workspace", "docker compose config --hash=*",
	)
	manifest := generatedDeploymentTestManifest(command, false)
	rollback := generatedDeploymentTestRollbackPlan(command)
	receipt := generatedDeploymentTestReceipt(t, command)
	receipt.EndpointPort = uint16(20000 + job.ID%30000)
	receipt.WorkspaceVerificationReceiptID = verification.ID
	return generatedDeploymentDatabaseFixture{
		repository: repository, pool: pool, ctx: ctx, projectID: project.ID, jobID: job.ID,
		authority: claim.Authority, command: command, verification: verification,
		manifest: manifest, rollback: rollback, evidenceID: firstEvidenceID, receipt: receipt,
		rawSecret: rawSecret, rawConfig: rawConfig,
	}
}

func recordGeneratedDeploymentVerification(
	t *testing.T, repository *Repository, ctx context.Context, authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand, commandTexts ...string,
) (GeneratedWorkloadVerificationRecord, int64) {
	return recordGeneratedDeploymentVerificationWithNamespace(
		t, repository, ctx, authority, command, true, commandTexts...,
	)
}

func recordGeneratedDeploymentVerificationWithNamespace(
	t *testing.T, repository *Repository, ctx context.Context, authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand, includeNamespace bool, commandTexts ...string,
) (GeneratedWorkloadVerificationRecord, int64) {
	t.Helper()
	ids := make([]int64, len(commandTexts))
	for index, commandText := range commandTexts {
		record := evidence.Record{
			JobID: authority.JobID, StepID: authority.StepID, Kind: evidence.KindTestResult,
			SourceType: "command", SourceRef: fmt.Sprintf("verification-%d-%d", authority.Attempt, index),
			Command: commandText, Summary: "Exact workspace verification succeeded.",
			Metadata: map[string]any{"succeeded": true},
		}
		if index == len(commandTexts)-1 {
			record.SourceType = GeneratedWorkloadResolvedConfigEvidenceSource
			record.Metadata["resolved_config_sha256"] = command.ConfigSHA256
			record.Metadata["workspace_sha256"] = command.WorkspaceSHA256
			record.Metadata["secret_set_sha256"] = command.SecretSetSHA256
			serviceHashes := make([]map[string]string, len(command.Services))
			for serviceIndex, service := range command.Services {
				serviceHashes[serviceIndex] = map[string]string{
					"service": service, "sha256": generatedDeploymentSHA("config-service-" + service),
				}
			}
			environmentNames := append([]string{"HOST_BIND_ADDRESS", "HOST_HTTP_PORT"}, command.RequiredSecretNames...)
			sort.Strings(environmentNames)
			record.Metadata["service_hashes"] = serviceHashes
			record.Metadata["implicit_env_disabled"] = true
			record.Metadata["environment_names"] = environmentNames
			if includeNamespace {
				namespace, _, err := BindGeneratedWorkloadDeploymentNamespacePreflight(
					GeneratedWorkloadDeploymentNamespacePreflight{
						Schema:         GeneratedWorkloadDeploymentNamespacePreflightV1,
						ComposeProject: command.ComposeProject,
						ContainerIDs:   []string{},
						NetworkIDs:     []string{},
						VolumeNames:    []string{},
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				record.Metadata[GeneratedWorkloadDeploymentNamespaceMetadataKey] = namespace
			}
		}
		id, err := repository.WriteEvidenceReturningID(ctx, authority, record)
		if err != nil {
			t.Fatal(err)
		}
		ids[index] = id
	}
	verification, err := repository.RecordGeneratedWorkloadVerification(
		ctx, authority, command.WorkspaceSHA256, ids,
	)
	if err != nil {
		t.Fatal(err)
	}
	return verification, ids[0]
}

func generatedDeploymentTestManifest(
	command GeneratedWorkloadDeploymentCommand,
	stateful bool,
) GeneratedWorkloadDeploymentLifecycleManifest {
	slots := []GeneratedWorkloadDeploymentLifecycleSlot{
		GeneratedDeploymentSlotBuild, GeneratedDeploymentSlotInitialStart,
	}
	if stateful {
		slots = append(slots, GeneratedDeploymentSlotMigrate)
	}
	slots = append(slots, GeneratedDeploymentSlotInitialObserve)
	if stateful {
		slots = append(slots, GeneratedDeploymentSlotStateWrite)
	}
	slots = append(slots, GeneratedDeploymentSlotRestart,
		GeneratedDeploymentSlotRestartStart, GeneratedDeploymentSlotFinalObserve)
	if stateful {
		slots = append(slots, GeneratedDeploymentSlotStateRead)
	}
	commands := make([]GeneratedWorkloadDeploymentExecutionCommand, len(slots))
	for index, slot := range slots {
		commands[index] = GeneratedWorkloadDeploymentExecutionCommand{
			Slot: slot, WorkspaceSHA256: command.WorkspaceSHA256,
			CommandSHA256: generatedDeploymentSHA(generatedDeploymentTestExecutionText(slot)),
		}
	}
	return GeneratedWorkloadDeploymentLifecycleManifest{
		Schema: GeneratedWorkloadDeploymentLifecycleManifestV1, Commands: commands,
	}
}

func generatedDeploymentTestExecutionText(slot GeneratedWorkloadDeploymentLifecycleSlot) string {
	return "docker compose exact " + slot.Name
}

func generatedDeploymentTestRollbackPlan(
	command GeneratedWorkloadDeploymentCommand,
) GeneratedWorkloadDeploymentRollbackPlan {
	plan := GeneratedWorkloadDeploymentRollbackPlan{
		Policy:                  GeneratedWorkloadDeploymentRollbackDestroyFirstV1,
		MaxAttempts:             MaxGeneratedWorkloadDeploymentRollbackAttempts,
		ComposeProject:          command.ComposeProject,
		ResourceObservation:     GeneratedWorkloadDeploymentRollbackResourcesV1,
		RequireContainerAbsence: true,
		RequireNetworkAbsence:   true,
		RequireVolumeAbsence:    true,
		Execution: GeneratedWorkloadDeploymentExecutionCommand{
			Slot: GeneratedDeploymentSlotRollback,
			CommandSHA256: generatedDeploymentSHA(
				generatedDeploymentTestExecutionText(GeneratedDeploymentSlotRollback),
			),
			WorkspaceSHA256: command.WorkspaceSHA256,
		},
	}
	var err error
	plan.PostconditionJSON, plan.PostconditionSHA256, err =
		CanonicalGeneratedWorkloadDeploymentRollbackPostcondition(plan)
	if err != nil {
		panic(err)
	}
	return plan
}

func (fixture generatedDeploymentDatabaseFixture) prepare(
	t *testing.T, authority model.StepAttemptAuthority, verificationID string,
) GeneratedWorkloadDeploymentRecord {
	t.Helper()
	record, err := fixture.repository.PrepareGeneratedWorkloadDeployment(
		fixture.ctx, authority, fixture.command, verificationID, fixture.manifest, fixture.rollback,
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func (fixture generatedDeploymentDatabaseFixture) reserve(
	t *testing.T, authority model.StepAttemptAuthority,
	expectation GeneratedWorkloadProjectDeploymentHeadExpectation,
) GeneratedWorkloadProjectDeploymentReservation {
	t.Helper()
	reservation, err := fixture.repository.ReserveGeneratedWorkloadProjectDeploymentCandidate(
		fixture.ctx, authority, fixture.command, 1, strings.Repeat("d", 64), expectation,
	)
	if err != nil {
		t.Fatal(err)
	}
	return reservation
}

func (fixture generatedDeploymentDatabaseFixture) executeSuccessfulRail(
	t *testing.T, authority model.StepAttemptAuthority,
) GeneratedWorkloadDeploymentReceipt {
	t.Helper()
	receipt := fixture.receipt
	var executionIDs, observationIDs []int64
	for _, execution := range fixture.manifest.Commands {
		if generatedDeploymentSlotRequiresNamespaceRequalification(execution.Slot) {
			generatedDeploymentQualifyProtectedExecution(t, fixture, authority, execution)
		}
		if _, created, err := fixture.repository.BeginGeneratedWorkloadDeploymentExecution(
			fixture.ctx, authority, fixture.command, execution,
		); err != nil || !created {
			t.Fatalf("begin %s: created=%t err=%v", execution.Slot.Name, created, err)
		}
		result := evidence.Record{
			JobID: authority.JobID, StepID: authority.StepID, Kind: evidence.KindCommandOutput,
			SourceType: "command", SourceRef: "execution-" + execution.Slot.Name,
			Command:  generatedDeploymentTestExecutionText(execution.Slot),
			Summary:  "Exact deployment command completed.",
			Metadata: map[string]any{"execution": true, "side_effect_possible": true, "succeeded": true},
		}
		executed, err := fixture.repository.CompleteGeneratedWorkloadDeploymentExecution(
			fixture.ctx, authority, fixture.command, execution, result,
		)
		if err != nil {
			t.Fatal(err)
		}
		executionIDs = append(executionIDs, executed.EvidenceID)
		if execution.Slot == GeneratedDeploymentSlotInitialObserve || execution.Slot == GeneratedDeploymentSlotFinalObserve {
			observed, err := fixture.repository.RecordGeneratedWorkloadDeploymentObservation(
				fixture.ctx, authority, fixture.command, execution, fixture.observation(t),
			)
			if err != nil {
				t.Fatal(err)
			}
			observationIDs = append(observationIDs, observed.EvidenceID)
		}
	}
	sort.Slice(executionIDs, func(i, j int) bool { return executionIDs[i] < executionIDs[j] })
	sort.Slice(observationIDs, func(i, j int) bool { return observationIDs[i] < observationIDs[j] })
	receipt.ExecutionEvidenceIDs, receipt.ObservationEvidenceIDs = executionIDs, observationIDs
	return receipt
}

func generatedDeploymentQualifyProtectedExecution(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	authority model.StepAttemptAuthority,
	execution GeneratedWorkloadDeploymentExecutionCommand,
) {
	t.Helper()
	proof := generatedDeploymentVacantNamespaceProof(t, fixture.command.ComposeProject)
	if _, created, err := fixture.repository.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		fixture.ctx, authority, fixture.command, execution, proof,
	); err != nil || !created {
		t.Fatalf("qualify %s namespace: created=%t err=%v", execution.Slot.Name, created, err)
	}
}

func (fixture generatedDeploymentDatabaseFixture) observation(t *testing.T) GeneratedWorkloadDeploymentObservation {
	t.Helper()
	services := make([]GeneratedWorkloadDeploymentObservedService, len(fixture.receipt.Services))
	for index, service := range fixture.receipt.Services {
		services[index] = GeneratedWorkloadDeploymentObservedService{
			Service: service.Service, ContainerID: service.ContainerID, ImageDigest: service.ImageDigest,
			RestartPolicy: string(service.RestartPolicy), State: service.State, Health: service.Health,
		}
	}
	endpoint := GeneratedWorkloadDeploymentObservedEndpoint{
		Scheme: fixture.receipt.EndpointScheme, Host: fixture.receipt.EndpointHost,
		Port: fixture.receipt.EndpointPort, Path: fixture.receipt.EndpointPath,
	}
	servicesSHA, err := generatedDeploymentObservationHash("services.v1", services)
	if err != nil {
		t.Fatal(err)
	}
	endpointSHA, err := generatedDeploymentObservationHash("endpoint.v1", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	canonical := struct {
		Schema   string                                       `json:"schema"`
		Project  string                                       `json:"project"`
		Services []GeneratedWorkloadDeploymentObservedService `json:"services"`
		Endpoint GeneratedWorkloadDeploymentObservedEndpoint  `json:"endpoint"`
	}{GeneratedWorkloadDeploymentObservationV1, fixture.command.ComposeProject, services, endpoint}
	sha, err := generatedDeploymentObservationHash("observation.v1", canonical)
	if err != nil {
		t.Fatal(err)
	}
	return GeneratedWorkloadDeploymentObservation{
		Schema: GeneratedWorkloadDeploymentObservationV1, Project: fixture.command.ComposeProject,
		Services: services, Endpoint: endpoint, ServicesSHA256: servicesSHA,
		EndpointSHA256: endpointSHA, SHA256: sha,
	}
}

func (fixture generatedDeploymentDatabaseFixture) prepareAndApply(t *testing.T) GeneratedWorkloadDeploymentRecord {
	t.Helper()
	fixture.prepare(t, fixture.authority, fixture.verification.ID)
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	if _, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying},
	); err != nil {
		t.Fatal(err)
	}
	receipt := fixture.executeSuccessfulRail(t, fixture.authority)
	record, err := fixture.repository.SealGeneratedWorkloadDeploymentApplied(
		fixture.ctx, fixture.authority, fixture.command, receipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
