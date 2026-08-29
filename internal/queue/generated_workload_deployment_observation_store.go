package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const generatedWorkloadDeploymentObservationSource = "docker_compose_observation"

func (r *Repository) RecordGeneratedWorkloadDeploymentObservation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	execution GeneratedWorkloadDeploymentExecutionCommand,
	observation GeneratedWorkloadDeploymentObservation,
) (GeneratedWorkloadDeploymentObservationRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf("record deployment observation requires PostgreSQL and context")
	}
	if execution.Slot != GeneratedDeploymentSlotInitialObserve &&
		execution.Slot != GeneratedDeploymentSlotFinalObserve {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf("deployment observation requires an exact observation slot")
	}
	if err := validateGeneratedDeploymentExecutionCommand(command, execution); err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	observationJSON, err := validateAndCanonicalizeGeneratedDeploymentObservation(command, observation)
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	deployment, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	if err := requireGeneratedDeploymentIdentity(deployment, identity); err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	if deployment.State != GeneratedWorkloadDeploymentApplying {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf(
			"%w: deployment observation cannot be recorded from %s",
			ErrGeneratedWorkloadDeploymentState, deployment.State,
		)
	}
	executionRecord, found, err := loadGeneratedDeploymentExecutionTx(
		ctx, tx, identity.OperationID, execution.Slot.Ordinal, true,
	)
	if err != nil || !found || executionRecord.Status != GeneratedWorkloadDeploymentExecutionCompleted ||
		executionRecord.Succeeded == nil || !*executionRecord.Succeeded || executionRecord.EvidenceID <= 0 {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf("deployment observation command lacks completed successful evidence: %w", err)
	}
	if executionRecord.StepAttempt != authority.Attempt || executionRecord.WorkerID != authority.WorkerID ||
		deployment.Current.StepAttempt != authority.Attempt || deployment.Current.WorkerID != authority.WorkerID {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf(
			"%w: deployment observation executor differs from completed command owner",
			ErrGeneratedWorkloadDeploymentConflict,
		)
	}
	head, found, err := lockGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, command.Authority.ProjectID,
	)
	if err != nil || !found || head.Candidate == nil ||
		head.Candidate.DeploymentID != identity.OperationID ||
		head.Candidate.Authority != command.Authority ||
		head.Candidate.Executor != deployment.Current {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf(
			"%w: deployment observation lacks exact candidate authority",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	existing, found, err := loadGeneratedDeploymentObservationTx(
		ctx, tx, identity.OperationID, execution.Slot.Ordinal, true,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	if found {
		if existing.CommandEvidenceID != executionRecord.EvidenceID ||
			existing.Observation.SHA256 != observation.SHA256 ||
			existing.Observation.ServicesSHA256 != observation.ServicesSHA256 ||
			existing.Observation.EndpointSHA256 != observation.EndpointSHA256 {
			return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf("%w: deployment observation replay differs", ErrGeneratedWorkloadDeploymentConflict)
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentObservationRecord{}, err
		}
		return existing, nil
	}
	payload, err := generatedDeploymentObservationEvidencePayload(
		command, identity.OperationID, execution, executionRecord.EvidenceID, observationJSON, observation,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	var evidenceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, command.Authority.JobID, command.Authority.StepID, evidence.KindDeploymentObservation,
		generatedWorkloadDeploymentObservationSource, identity.OperationID, payload).Scan(&evidenceID); err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf("insert deployment observation evidence: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_observations(
		 operation_id,slot_name,slot_ordinal,command_evidence_id,observation_json,
		 observation_sha256,services_sha256,endpoint_sha256,evidence_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, identity.OperationID, execution.Slot.Name, execution.Slot.Ordinal,
		executionRecord.EvidenceID, observationJSON, observation.SHA256,
		observation.ServicesSHA256, observation.EndpointSHA256, evidenceID)
	if err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf("insert deployment observation: %w", err)
	}
	record, found, err := loadGeneratedDeploymentObservationTx(
		ctx, tx, identity.OperationID, execution.Slot.Ordinal, true,
	)
	if err != nil || !found {
		return GeneratedWorkloadDeploymentObservationRecord{}, fmt.Errorf("reload deployment observation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentObservationRecord{}, err
	}
	return record, nil
}

func validateAndCanonicalizeGeneratedDeploymentObservation(
	command GeneratedWorkloadDeploymentCommand,
	observation GeneratedWorkloadDeploymentObservation,
) (string, error) {
	if observation.Schema != GeneratedWorkloadDeploymentObservationV1 ||
		observation.Project != command.ComposeProject || len(observation.Services) != len(command.Services) {
		return "", fmt.Errorf("deployment observation authority is incomplete")
	}
	for index, service := range observation.Services {
		if service.Service != command.Services[index] ||
			!validSHA256Digest(service.ContainerID) ||
			!validSHA256ID(service.ImageDigest, "sha256:") ||
			service.RestartPolicy != "unless-stopped" || service.State != "running" || service.Health != "healthy" {
			return "", fmt.Errorf("deployment observation service %d is invalid", index)
		}
	}
	if observation.Endpoint.Scheme != command.EndpointScheme ||
		observation.Endpoint.Host != command.EndpointHost || observation.Endpoint.Port == 0 ||
		observation.Endpoint.Path != command.EndpointPath {
		return "", fmt.Errorf("deployment observation endpoint differs from command authority")
	}
	servicesSHA, err := generatedDeploymentObservationHash("services.v1", observation.Services)
	if err != nil || servicesSHA != observation.ServicesSHA256 {
		return "", fmt.Errorf("deployment observation service digest differs")
	}
	endpointSHA, err := generatedDeploymentObservationHash("endpoint.v1", observation.Endpoint)
	if err != nil || endpointSHA != observation.EndpointSHA256 {
		return "", fmt.Errorf("deployment observation endpoint digest differs")
	}
	canonical := struct {
		Schema   string                                       `json:"schema"`
		Project  string                                       `json:"project"`
		Services []GeneratedWorkloadDeploymentObservedService `json:"services"`
		Endpoint GeneratedWorkloadDeploymentObservedEndpoint  `json:"endpoint"`
	}{observation.Schema, observation.Project, observation.Services, observation.Endpoint}
	sha, err := generatedDeploymentObservationHash("observation.v1", canonical)
	if err != nil || sha != observation.SHA256 {
		return "", fmt.Errorf("deployment observation digest differs")
	}
	encoded, err := canonicalGeneratedDeploymentJSON(observation)
	if err != nil || len(encoded) > 32768 {
		return "", fmt.Errorf("deployment observation exceeds canonical bound: %w", err)
	}
	return encoded, nil
}

func generatedDeploymentObservationHash(domain string, value any) (string, error) {
	encoded, err := json.Marshal(struct {
		Domain string `json:"domain"`
		Value  any    `json:"value"`
	}{domain, value})
	if err != nil {
		return "", err
	}
	return generatedDeploymentSHA(string(encoded)), nil
}

func generatedDeploymentObservationEvidencePayload(
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	execution GeneratedWorkloadDeploymentExecutionCommand,
	commandEvidenceID int64,
	observationJSON string,
	observation GeneratedWorkloadDeploymentObservation,
) (string, error) {
	record := evidence.Record{
		JobID: command.Authority.JobID, StepID: command.Authority.StepID,
		Kind: evidence.KindDeploymentObservation, SourceType: generatedWorkloadDeploymentObservationSource,
		SourceRef: operationID,
		Excerpt:   observationJSON, Summary: "Observed the exact healthy Docker service and readiness state.",
		Hash: observation.SHA256, Confidence: 1,
		Metadata: map[string]any{
			"slot": execution.Slot.Name, "ordinal": execution.Slot.Ordinal,
			"compose_ps_evidence_id": commandEvidenceID,
			"command_sha256":         execution.CommandSHA256,
			"workspace_sha256":       execution.WorkspaceSHA256,
			"observation_sha256":     observation.SHA256,
			"services_sha256":        observation.ServicesSHA256,
			"endpoint_sha256":        observation.EndpointSHA256, "succeeded": true,
		},
	}
	encoded, err := json.Marshal(record)
	return string(encoded), err
}
