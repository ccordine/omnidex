package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
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
	if record.ID != string(authority.DataSourceID) || !record.ReadOnly || record.Driver != datasource.DriverPostgres {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("bound data source %q is not an exact read-only PostgreSQL authority", authority.DataSourceID)
	}
	pool, err := datasource.ConnectReadOnly(ctx, record.Connection())
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("connect bound data source %q failed", authority.DataSourceID)
	}
	defer pool.Close()
	snapshot, err := datasource.InspectCatalog(ctx, pool, record.ID, record.Name)
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("inspect bound data source %q schema: %w", authority.DataSourceID, err)
	}
	if err := r.svc.repo.SaveDataSourceSchemaSnapshot(ctx, snapshot); err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("persist bound data source %q schema authority: %w", authority.DataSourceID, err)
	}
	stations := portableObjectiveDatabaseStations{runtime: r}
	limits := datasource.DefaultExecutionLimits()
	limits.MaxRows = maxObjectiveDatabaseRows
	limits.MaxBytes = 64 * 1024
	execute := func(
		callContext context.Context,
		exactSnapshot datasource.SchemaSnapshot,
		compiled datasource.CompiledQuery,
	) (datasource.EvidenceResult, error) {
		if exactSnapshot.SourceID != record.ID || exactSnapshot.Fingerprint != snapshot.Fingerprint {
			return datasource.EvidenceResult{}, fmt.Errorf("database executor received stale schema authority")
		}
		evidence, err := datasource.ExecuteEvidence(callContext, pool, exactSnapshot, compiled, limits)
		if err != nil {
			return datasource.EvidenceResult{}, err
		}
		if _, err := r.svc.repo.SaveDatabaseEvidenceReceipt(callContext, authority.JobID, evidence); err != nil {
			return datasource.EvidenceResult{}, err
		}
		return evidence, nil
	}
	return runObjectiveDatabaseEvidenceWorkflow(
		ctx, authority, requirementID, snapshot, stations, execute,
	)
}
