package worker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/queue"
)

func (r *nativeRuntimeV3) acquireObjectiveDatabaseEvidence(
	ctx context.Context,
	authority turnAuthority,
	requirementID string,
) (objectiveEvidenceAcquisition, error) {
	if ctx == nil || r == nil || r.svc == nil || r.svc.repo == nil || r.claim == nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("database-read objective requires runtime and repository authority")
	}
	if authority.JobID != r.claim.Job.ID || authority.Instruction != r.claim.Job.Instruction ||
		authority.DataSourceID == "" {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("database-read authority does not match the claimed bound job")
	}
	record, err := r.svc.repo.GetDataSource(ctx, string(authority.DataSourceID))
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("load bound data source %q: %w", authority.DataSourceID, err)
	}
	if record.ID != string(authority.DataSourceID) || !record.ReadOnly ||
		record.Driver != datasource.DriverPostgres {
		return objectiveEvidenceAcquisition{}, fmt.Errorf(
			"bound data source %q is not an exact read-only PostgreSQL authority", authority.DataSourceID,
		)
	}
	switch record.ExecutionMode {
	case datasource.ExecutionModeDirect:
		if authority.DelegatedDataAuthorityID != "" {
			return objectiveEvidenceAcquisition{}, fmt.Errorf("direct database authority cannot carry a delegated turn identity")
		}
		return r.acquireDirectDatabaseEvidence(ctx, authority, requirementID, record)
	case datasource.ExecutionModeDelegated:
		if err := validateDelegatedDatabaseInferenceProvider(r.svc.inferenceProvider); err != nil {
			return objectiveEvidenceAcquisition{}, err
		}
		if err := datasource.ValidateDelegatedAuthorityID(authority.DelegatedDataAuthorityID); err != nil {
			return objectiveEvidenceAcquisition{}, fmt.Errorf("delegated database authority is unavailable: %w", err)
		}
		return r.acquireDelegatedDatabaseEvidence(ctx, authority, requirementID, record)
	default:
		return objectiveEvidenceAcquisition{}, fmt.Errorf(
			"bound data source %q has unsupported execution mode %q", authority.DataSourceID, record.ExecutionMode,
		)
	}
}

func (r *nativeRuntimeV3) acquireDirectDatabaseEvidence(
	ctx context.Context,
	authority turnAuthority,
	requirementID string,
	record queue.DataSourceRecord,
) (objectiveEvidenceAcquisition, error) {
	connection, err := record.DirectConnection()
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	pool, err := datasource.ConnectReadOnly(ctx, connection)
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("connect bound data source %q failed", authority.DataSourceID)
	}
	defer pool.Close()
	snapshot, err := datasource.InspectCatalog(ctx, pool, record.ID, record.Name)
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("inspect bound data source %q schema: %w", authority.DataSourceID, err)
	}
	if err := r.svc.repo.SaveDataSourceSchemaSnapshot(ctx, snapshot); err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf(
			"persist bound data source %q schema authority: %w", authority.DataSourceID, err,
		)
	}
	execute := func(
		callContext context.Context,
		exactSnapshot datasource.SchemaSnapshot,
		plan datasource.RelationalQueryPlan,
	) (datasource.EvidenceResult, error) {
		if !sameDatabaseSnapshotAuthority(exactSnapshot, snapshot) {
			return datasource.EvidenceResult{}, fmt.Errorf("database executor received stale schema authority")
		}
		compiled, err := datasource.CompilePostgresPlan(exactSnapshot, plan)
		if err != nil {
			return datasource.EvidenceResult{}, err
		}
		evidence, err := datasource.ExecuteEvidence(
			callContext, pool, exactSnapshot, compiled, objectiveDatabaseExecutionLimits(),
		)
		if err != nil {
			return datasource.EvidenceResult{}, err
		}
		return r.persistDatabaseEvidence(callContext, authority.JobID, evidence)
	}
	return runObjectiveDatabaseEvidenceWorkflow(
		ctx, authority, requirementID, snapshot, portableObjectiveDatabaseStations{runtime: r}, execute,
	)
}

func (r *nativeRuntimeV3) acquireDelegatedDatabaseEvidence(
	ctx context.Context,
	authority turnAuthority,
	requirementID string,
	record queue.DataSourceRecord,
) (objectiveEvidenceAcquisition, error) {
	token, err := loadDelegatedDatabaseCredential(record)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	client, err := datasource.NewDelegatedClient(
		record.AuthorityURL, token, &http.Client{Timeout: 30 * time.Second},
	)
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("configure delegated database authority: %w", err)
	}
	snapshot, err := client.FetchSchema(
		ctx, record.ID, record.Name, authority.DelegatedDataAuthorityID,
	)
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("fetch delegated database schema: %w", err)
	}
	execute := func(
		callContext context.Context,
		exactSnapshot datasource.SchemaSnapshot,
		plan datasource.RelationalQueryPlan,
	) (datasource.EvidenceResult, error) {
		if !sameDatabaseSnapshotAuthority(exactSnapshot, snapshot) {
			return datasource.EvidenceResult{}, fmt.Errorf("delegated database executor received stale schema authority")
		}
		evidence, err := client.Execute(
			callContext, authority.DelegatedDataAuthorityID, exactSnapshot, plan,
			objectiveDatabaseExecutionLimits(),
		)
		if err != nil {
			return datasource.EvidenceResult{}, err
		}
		return r.persistDatabaseEvidence(callContext, authority.JobID, evidence)
	}
	return runObjectiveDatabaseEvidenceWorkflow(
		ctx, authority, requirementID, snapshot, portableObjectiveDatabaseStations{runtime: r}, execute,
	)
}

func loadDelegatedDatabaseCredential(record queue.DataSourceRecord) (string, error) {
	urlEnvironment, err := datasource.DelegatedAuthorityURLEnvironmentName(record.CredentialEnv)
	if err != nil {
		return "", err
	}
	boundURL, exists := os.LookupEnv(urlEnvironment)
	if !exists {
		return "", fmt.Errorf("delegated database authority URL environment %q is not configured", urlEnvironment)
	}
	if boundURL != record.AuthorityURL {
		return "", fmt.Errorf("delegated database authority URL does not match environment %q", urlEnvironment)
	}
	token, exists := os.LookupEnv(record.CredentialEnv)
	if !exists {
		return "", fmt.Errorf(
			"delegated database credential environment %q is not configured", record.CredentialEnv,
		)
	}
	return token, nil
}

func (r *nativeRuntimeV3) persistDatabaseEvidence(
	ctx context.Context,
	jobID int64,
	evidence datasource.EvidenceResult,
) (datasource.EvidenceResult, error) {
	if _, err := r.svc.repo.SaveDatabaseEvidenceReceipt(ctx, jobID, evidence); err != nil {
		return datasource.EvidenceResult{}, err
	}
	return evidence, nil
}

func sameDatabaseSnapshotAuthority(left, right datasource.SchemaSnapshot) bool {
	return left.SourceID == right.SourceID && left.Fingerprint == right.Fingerprint
}

func validateDelegatedDatabaseInferenceProvider(provider string) error {
	if provider != "ollama" {
		return fmt.Errorf(
			"delegated database evidence requires local Ollama inference; active provider is %q", provider,
		)
	}
	return nil
}
