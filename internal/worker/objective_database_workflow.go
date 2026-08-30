package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

const (
	maxObjectiveDatabaseRows = 50
)

type objectiveDatabaseExecutor func(
	context.Context,
	datasource.SchemaSnapshot,
	datasource.RelationalQueryPlan,
) (datasource.EvidenceResult, error)

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
	var ledger objectiveDatabaseAcquisitionCallLedger
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
	needID := objectiveDatabaseEvidenceNeedID(requirementID, exactNeed)
	relationIDs, selectionReceipt, err := selectObjectiveDatabaseRelations(
		ctx, snapshot, needID, exactNeed,
		assemblyline.CloneObjectiveContext(authority.Context), stations,
	)
	if err != nil {
		return result, err
	}
	if selectionReceipt != (objectiveStationReceipt{}) {
		if err := ledger.record(
			"schema selection", selectionReceipt,
			maxDatabaseSchemaSelectionModelCalls,
		); err != nil {
			return result, err
		}
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
	decision, receipt, err := stations.BuildIntent(ctx, intentInput)
	if err != nil {
		return result, err
	}
	if err := ledger.record(
		"query intent", receipt, maxObjectiveDatabaseQueryIntentCalls,
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
	plan, planningReceipt, err := prepareObjectiveDatabaseQueryPlan(
		ctx, snapshot, intent, needID, exactNeed,
		assemblyline.CloneObjectiveContext(authority.Context), stations,
	)
	if err != nil {
		return result, err
	}
	if planningReceipt != (objectiveStationReceipt{}) {
		if err := ledger.record(
			"join-path selection", planningReceipt,
			datasource.MaxProjectedRelations*exactSemanticLeafCalls,
		); err != nil {
			return result, err
		}
	}
	executed, err := execute(ctx, snapshot, plan)
	if err != nil {
		return result, err
	}
	if err := executed.ValidateForPlan(snapshot, plan, objectiveDatabaseExecutionLimits()); err != nil {
		return result, fmt.Errorf("database executor returned invalid evidence: %w", err)
	}
	evidence, err := projectObjectiveDatabaseEvidence(snapshot, intent, executed)
	if err != nil {
		return result, err
	}
	result.Evidence = append(result.Evidence, evidence...)
	if len(result.Evidence) > maxDatabaseEvidenceCapsules {
		return result, fmt.Errorf("database cognition exceeded %d evidence capsules", maxDatabaseEvidenceCapsules)
	}
	return completeObjectiveDatabaseEvidenceAcquisition(result, ledger)
}

func completeObjectiveDatabaseEvidenceAcquisition(
	result objectiveEvidenceAcquisition,
	ledger objectiveDatabaseAcquisitionCallLedger,
) (objectiveEvidenceAcquisition, error) {
	receipt, err := ledger.totalForSuccess()
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	result.ModelCalls = receipt.Calls
	result.DatabaseCallLedger = ledger
	return result, nil
}

func objectiveDatabaseEvidenceNeedID(requirementID string, exactNeed string) string {
	digest := sha256.Sum256([]byte(requirementID + "\x00" + exactNeed))
	return "database-need-" + hex.EncodeToString(digest[:])
}
