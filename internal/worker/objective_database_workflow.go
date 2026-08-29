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
	maxObjectiveDatabaseRounds = 3
	maxObjectiveDatabaseRows   = 50
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
	currentNeed := authority.ModelInstruction
	seenNeeds := map[string]struct{}{}
	for round := 1; round <= maxObjectiveDatabaseRounds; round++ {
		needID := objectiveDatabaseEvidenceNeedID(requirementID, round, currentNeed)
		needDigest := objectiveDatabaseEvidenceNeedDigest(currentNeed)
		if _, duplicate := seenNeeds[needDigest]; duplicate {
			return result, fmt.Errorf("database cognition repeated an unresolved evidence need without progress")
		}
		seenNeeds[needDigest] = struct{}{}

		relationIDs, selectionCalls, err := selectObjectiveDatabaseRelations(
			ctx, snapshot, needID, currentNeed,
			assemblyline.CloneObjectiveContext(authority.Context), stations,
		)
		result.ModelCalls += selectionCalls
		if err != nil {
			return result, err
		}
		projection, err := datasource.ProjectSchemaForIntent(snapshot, relationIDs)
		if err != nil {
			return result, err
		}
		intentInput := assemblyline.DatabaseQueryIntentInput{
			EvidenceNeedID: needID, ExactNeed: currentNeed,
			Context:          assemblyline.CloneObjectiveContext(authority.Context),
			SchemaProjection: projection,
			TemporalAsOf:     snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
			MaxRows:          maxObjectiveDatabaseRows,
		}
		decision, receipt, err := stations.BuildIntent(ctx, intentInput)
		result.ModelCalls += receipt.Calls
		if err != nil {
			return result, err
		}
		if err := validateObjectiveDatabaseStationCalls("query intent", receipt); err != nil {
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
			ctx, snapshot, intent, needID, currentNeed,
			assemblyline.CloneObjectiveContext(authority.Context), stations,
		)
		result.ModelCalls += planningCalls
		if err != nil {
			return result, err
		}
		executed, err := execute(ctx, snapshot, plan)
		if err != nil {
			return result, err
		}
		if err := executed.ValidateForPlan(snapshot, plan, objectiveDatabaseExecutionLimits()); err != nil {
			return result, fmt.Errorf("database executor returned invalid evidence: %w", err)
		}
		evidence, err := projectObjectiveDatabaseEvidence(round, snapshot, intent, executed)
		if err != nil {
			return result, err
		}
		result.Evidence = append(result.Evidence, evidence...)
		if len(result.Evidence) > maxDatabaseEvidenceCapsules {
			return result, fmt.Errorf("database cognition exceeded %d accumulated evidence capsules", maxDatabaseEvidenceCapsules)
		}
		modelEvidence, err := objectiveModelEvidence(result.Evidence)
		if err != nil {
			return result, err
		}
		gapInput := assemblyline.DatabaseEvidenceGapInput{
			RequirementID: requirementID, ExactRequirement: authority.ModelInstruction,
			Context: assemblyline.CloneObjectiveContext(authority.Context), Evidence: modelEvidence,
			KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...),
		}
		gap, gapReceipt, err := stations.FindEvidenceGap(ctx, gapInput)
		result.ModelCalls += gapReceipt.Calls
		if err != nil {
			return result, err
		}
		if err := validateObjectiveDatabaseStationCalls("evidence gap", gapReceipt); err != nil {
			return result, err
		}
		if err := gap.ValidateFor(gapInput); err != nil {
			return result, err
		}
		missing := gap.Missing()
		if missing == "" {
			return result, nil
		}
		if err := validateDatabaseEvidenceNeed(missing); err != nil {
			return result, err
		}
		currentNeed = missing
	}
	return result, fmt.Errorf("database cognition still has unresolved information after %d bounded rounds", maxObjectiveDatabaseRounds)
}

func objectiveDatabaseEvidenceNeedID(requirementID string, round int, exactNeed string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", requirementID, round, exactNeed)))
	return "database-need-" + hex.EncodeToString(digest[:])
}

func objectiveDatabaseEvidenceNeedDigest(exactNeed string) string {
	digest := sha256.Sum256([]byte(exactNeed))
	return hex.EncodeToString(digest[:])
}
