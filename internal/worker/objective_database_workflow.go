package worker

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/queue"
)

const maxObjectiveDatabaseRows = 50

type objectiveDatabaseExecutor func(
	context.Context,
	datasource.SchemaSnapshot,
	datasource.RelationalQueryPlan,
) (queue.DatabaseEvidenceRecord, error)

func objectiveDatabaseExecutionLimits() datasource.ExecutionLimits {
	limits := datasource.DefaultExecutionLimits()
	limits.MaxRows = maxObjectiveDatabaseRows
	limits.MaxBytes = 64 * 1024
	return limits
}

func runObjectiveDatabaseEvidenceWorkflow(
	ctx context.Context,
	authority turnAuthority,
	requirementID string,
	snapshot datasource.SchemaSnapshot,
	stations objectiveDatabaseStations,
	execute objectiveDatabaseExecutor,
) (objectiveEvidenceAcquisition, error) {
	result := objectiveEvidenceAcquisition{}
	if ctx == nil || authority.DataSourceID == "" || snapshot.SourceID != string(authority.DataSourceID) {
		return result, fmt.Errorf("database evidence workflow requires exact turn and schema authority")
	}
	if stations == nil || execute == nil {
		return result, fmt.Errorf("database evidence workflow requires semantic stations and a code-owned executor")
	}
	if len(snapshot.Relations) == 0 {
		return result, fmt.Errorf("database evidence workflow received an empty schema snapshot")
	}
	if _, err := snapshot.Relation(snapshot.Relations[0].ID); err != nil {
		return result, err
	}
	if err := validateDatabaseEvidenceNeed(authority.ModelInstruction); err != nil {
		return result, err
	}
	if err := validateObjectiveModelInput(
		authority, "database initial evidence need", authority.ModelInstruction,
	); err != nil {
		return result, err
	}
	exactNeed := authority.ModelInstruction
	needID := objectiveDatabaseEvidenceNeedID(requirementID)
	relationIDs, selectionCalls, err := selectObjectiveDatabaseRelations(
		ctx, snapshot, needID, exactNeed,
		assemblyline.CloneObjectiveContext(authority.Context), stations,
	)
	result.ModelCalls += selectionCalls
	if err != nil {
		return result, err
	}
	if err := validateObjectiveCallCount(
		"schema selection", selectionCalls, maxDatabaseSchemaSelectionModelCalls,
	); err != nil {
		return result, err
	}
	projection, err := datasource.ProjectSchemaForIntent(snapshot, relationIDs)
	if err != nil {
		return result, err
	}
	intentInput := assemblyline.DatabaseQueryIntentInput{
		EvidenceNeedID: needID, ExactNeed: exactNeed,
		Context:          assemblyline.CloneObjectiveContext(authority.Context),
		SchemaProjection: projection,
		TemporalAsOf:     snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
		MaxRows:          maxObjectiveDatabaseRows,
	}
	decision, dispatches, err := stations.BuildIntent(ctx, intentInput)
	result.ModelCalls += dispatches
	if err != nil {
		return result, err
	}
	if err := validateObjectiveCallCount(
		"query intent", dispatches, maxObjectiveDatabaseQueryIntentCalls,
	); err != nil {
		return result, err
	}
	if err := decision.ValidateFor(intentInput); err != nil {
		return result, err
	}
	intent := decision.Bind(intentInput)
	if err := intent.Validate(snapshot); err != nil {
		return result, fmt.Errorf("database query intent failed full schema validation: %w", err)
	}
	plan, planningCalls, err := prepareObjectiveDatabaseQueryPlan(
		ctx, snapshot, intent, needID, exactNeed,
		assemblyline.CloneObjectiveContext(authority.Context), stations,
	)
	result.ModelCalls += planningCalls
	if err != nil {
		return result, err
	}
	if err := validateObjectiveCallCount(
		"join-path selection", planningCalls, datasource.MaxProjectedRelations*exactSemanticLeafCalls,
	); err != nil {
		return result, err
	}
	executed, err := execute(ctx, snapshot, plan)
	if err != nil {
		return result, err
	}
	if executed.ID < 1 || executed.JobID != authority.JobID ||
		!reflect.DeepEqual(executed.Snapshot, snapshot) || !reflect.DeepEqual(executed.Plan, plan) {
		return result, fmt.Errorf("database executor returned a record for a different job, schema, or query")
	}
	if err := executed.Evidence.ValidateForPlan(snapshot, plan, objectiveDatabaseExecutionLimits()); err != nil {
		return result, fmt.Errorf("database executor returned invalid evidence: %w", err)
	}
	evidence, err := projectObjectiveDatabaseEvidence(executed)
	if err != nil {
		return result, err
	}
	result.Evidence = append(result.Evidence, evidence...)
	if len(result.Evidence) > maxDatabaseEvidenceCapsules {
		return result, fmt.Errorf("database cognition exceeded %d evidence capsules", maxDatabaseEvidenceCapsules)
	}
	return result, nil
}

func objectiveDatabaseEvidenceNeedID(requirementID string) string {
	return requirementID + "-database-need"
}
