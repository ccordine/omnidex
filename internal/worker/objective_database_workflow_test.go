package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
)

type scriptedObjectiveDatabaseStations struct {
	t             *testing.T
	fieldID       string
	intentCalls   int
	gapCalls      int
	missingByCall []string
}

type databaseTestKindStation struct {
	input assemblyline.ConversationObjectiveKindInput
}

func (station *databaseTestKindStation) Classify(
	_ context.Context,
	input assemblyline.ConversationObjectiveKindInput,
) (assemblyline.ConversationObjectiveKindDecision, objectiveStationReceipt, error) {
	station.input = input
	return assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindDatabaseRead,
	}, objectiveStationReceipt{Calls: 1}, nil
}

type databaseTestAnswerStation struct {
	input assemblyline.GroundedAnswerInput
}

func (station *databaseTestAnswerStation) Answer(
	_ context.Context,
	input assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	station.input = input
	return assemblyline.GroundedAnswerDecision{
		Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: input.RequirementID,
		Text: "There are three appointments.", EvidenceIDs: []string{input.Evidence[0].ID},
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station *scriptedObjectiveDatabaseStations) SelectSchema(
	context.Context,
	assemblyline.DatabaseSchemaSelectionInput,
) (assemblyline.DatabaseSchemaSelectionDecision, objectiveStationReceipt, error) {
	station.t.Fatal("single-relation snapshot must not spend inference on schema selection")
	return assemblyline.DatabaseSchemaSelectionDecision{}, objectiveStationReceipt{}, nil
}

func (station *scriptedObjectiveDatabaseStations) SelectJoinPath(
	context.Context,
	assemblyline.DatabaseJoinPathSelectionInput,
) (assemblyline.DatabaseJoinPathSelectionDecision, objectiveStationReceipt, error) {
	station.t.Fatal("single-relation intent must not spend inference on a join path")
	return assemblyline.DatabaseJoinPathSelectionDecision{}, objectiveStationReceipt{}, nil
}

func (station *scriptedObjectiveDatabaseStations) BuildIntent(
	_ context.Context,
	input assemblyline.DatabaseQueryIntentInput,
) (assemblyline.DatabaseQueryIntentDecision, objectiveStationReceipt, error) {
	station.intentCalls++
	if len(input.SchemaProjection.Relations) != 1 || input.MaxRows != maxObjectiveDatabaseRows {
		station.t.Fatalf("query projection=%+v", input)
	}
	return assemblyline.DatabaseQueryIntentDecision{
		Schema: assemblyline.DatabaseQueryIntentV1, EvidenceNeedID: input.EvidenceNeedID,
		FromRelationID: input.SchemaProjection.Relations[0].ID,
		Shape:          datasource.ResultScalar,
		Projections:    []datasource.RelationalProjection{{Aggregate: datasource.AggregateCountRows}},
		Filters:        []datasource.RelationalPredicate{}, TemporalWindows: []assemblyline.DatabaseTemporalWindowDecision{},
		Exists: []datasource.ExistencePredicate{}, GroupBy: []int{}, Having: []datasource.AggregatePredicate{},
		OrderBy: []datasource.OrderTerm{}, Limit: 1,
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station *scriptedObjectiveDatabaseStations) FindEvidenceGap(
	_ context.Context,
	input assemblyline.DatabaseEvidenceGapInput,
) (assemblyline.DatabaseEvidenceGapDecision, objectiveStationReceipt, error) {
	station.gapCalls++
	missing := ""
	if station.gapCalls <= len(station.missingByCall) {
		missing = station.missingByCall[station.gapCalls-1]
	}
	if len(input.Evidence) != station.gapCalls {
		station.t.Fatalf("gap call %d received %d accumulated capsules", station.gapCalls, len(input.Evidence))
	}
	return assemblyline.DatabaseEvidenceGapDecision{
		Schema: assemblyline.DatabaseEvidenceGapV1, RequirementID: input.RequirementID,
		MissingInformation: &missing,
	}, objectiveStationReceipt{Calls: 1}, nil
}

func TestDatabaseBoundObjectiveRunsTypedEvidenceLoopAndGroundsAnswer(t *testing.T) {
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	stations := &scriptedObjectiveDatabaseStations{t: t, fieldID: snapshot.Relations[0].Columns[0].ID}
	executions := 0
	workflow := func(
		ctx context.Context,
		authority turnAuthority,
		requirementID string,
	) (objectiveEvidenceAcquisition, error) {
		return runObjectiveDatabaseEvidenceWorkflow(ctx, authority, requirementID, snapshot, stations,
			func(_ context.Context, exact datasource.SchemaSnapshot, plan datasource.RelationalQueryPlan) (datasource.EvidenceResult, error) {
				executions++
				if exact.Fingerprint != snapshot.Fingerprint || plan.SourceID != snapshot.SourceID ||
					plan.SchemaFingerprint != snapshot.Fingerprint || plan.Intent.Projections[0].Aggregate != datasource.AggregateCountRows {
					t.Fatalf("relational plan authority=%+v", plan)
				}
				return objectiveDatabaseCountEvidence(plan, executions), nil
			})
	}
	kind := &databaseTestKindStation{}
	answer := &databaseTestAnswerStation{}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 71, Pipeline: model.PipelineChat, Instruction: "How many appointments exist?",
		Metadata: objectiveAssistantDataSourceMetadata(),
	}, scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind, nil, answer,
		objectiveWorkflows{DatabaseRead: workflow})
	if err != nil {
		t.Fatal(err)
	}
	if !kind.input.DatabaseEvidenceAvailable || executions != 1 || stations.intentCalls != 1 || stations.gapCalls != 1 {
		t.Fatalf("availability=%v executions=%d intent=%d gap=%d", kind.input.DatabaseEvidenceAvailable, executions, stations.intentCalls, stations.gapCalls)
	}
	if result.Kind != assemblyline.ObjectiveKindDatabaseRead || !result.Complete || result.ModelCalls != 4 || len(result.Citations) != 1 {
		t.Fatalf("result=%+v", result)
	}
	if strings.Contains(answer.input.Evidence[0].Text, "SELECT") || !strings.Contains(answer.input.Evidence[0].Text, `"label":"count_rows"`) {
		t.Fatalf("grounded evidence leaked SQL or lost exact label: %s", answer.input.Evidence[0].Text)
	}
	if _, _, err := prepareObjectiveTurnCompletion(result); err != nil {
		t.Fatalf("database completion evidence rejected: %v", err)
	}
}

func TestDatabaseEvidenceLoopAccumulatesOneNamedMissingFactThenStops(t *testing.T) {
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	stations := &scriptedObjectiveDatabaseStations{
		t: t, fieldID: snapshot.Relations[0].Columns[0].ID,
		missingByCall: []string{"The comparable count for the prior period."},
	}
	authority := turnAuthority{
		JobID: 72, Pipeline: model.PipelineChat, Instruction: "Compare this period with the prior period.",
		ModelInstruction: "Compare this period with the prior period.", ModelArtifactPaths: []string{},
		DataSourceID: "source-1",
	}
	executions := 0
	result, err := runObjectiveDatabaseEvidenceWorkflow(
		context.Background(), authority, "requirement-72", snapshot, stations,
		func(_ context.Context, _ datasource.SchemaSnapshot, plan datasource.RelationalQueryPlan) (datasource.EvidenceResult, error) {
			executions++
			return objectiveDatabaseCountEvidence(plan, executions), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if executions != 2 || stations.intentCalls != 2 || stations.gapCalls != 2 || result.ModelCalls != 4 || len(result.Evidence) != 2 {
		t.Fatalf("executions=%d intent=%d gap=%d result=%+v", executions, stations.intentCalls, stations.gapCalls, result)
	}
}

func objectiveDatabaseSingleRelationSnapshot(t *testing.T) datasource.SchemaSnapshot {
	t.Helper()
	snapshot, err := datasource.NewSchemaSnapshot("source-1", "appointments", []datasource.RelationDefinition{{
		Schema: "public", Name: "appointments", Kind: datasource.RelationTable,
		Columns: []datasource.ColumnDefinition{{
			Name: "id", Ordinal: 1, DataType: "uuid", TypeCategory: datasource.TypeUUID,
		}}, PrimaryKey: []string{"id"},
	}}, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func objectiveDatabaseCountEvidence(plan datasource.RelationalQueryPlan, sequence int) datasource.EvidenceResult {
	columns := []datasource.EvidenceColumn{{
		Name: plan.Outputs[0].Name, PostgresTypeOID: 20, FieldID: plan.Outputs[0].FieldID,
		Aggregate: plan.Outputs[0].Aggregate, TypeCategory: plan.Outputs[0].TypeCategory,
	}}
	rows := [][]datasource.EvidenceValue{{{
		Kind: datasource.EvidenceInteger, Value: strconv.Itoa(sequence + 2),
	}}}
	canonical, _ := json.Marshal(struct {
		Columns []datasource.EvidenceColumn  `json:"columns"`
		Rows    [][]datasource.EvidenceValue `json:"rows"`
	}{Columns: columns, Rows: rows})
	resultDigest := sha256.Sum256(canonical)
	resultHash := hex.EncodeToString(resultDigest[:])
	columnBytes, _ := json.Marshal(columns)
	rowBytes, _ := json.Marshal(rows[0])
	return datasource.EvidenceResult{
		Schema: datasource.EvidenceResultV1,
		Provenance: datasource.EvidenceProvenance{
			SourceID: plan.SourceID, SchemaFingerprint: plan.SchemaFingerprint,
			IntentHash: plan.IntentHash, QueryHash: objectiveDatabaseTestHash("query", sequence), ResultHash: resultHash,
			Plan:       datasource.ExecutionPlan{TotalCost: 1, EstimatedRows: 1},
			AcquiredAt: time.Unix(1_700_000_000+int64(sequence), 0).UTC(),
		},
		Result: datasource.TypedEvidenceResult{
			Columns: columns, Rows: rows,
			RowCount: 1, ByteCount: len(columnBytes) + len(rowBytes), Hash: resultHash,
		},
	}
}

func objectiveDatabaseTestHash(prefix string, sequence int) string {
	digest := sha256.Sum256([]byte(prefix + string(rune(sequence))))
	return hex.EncodeToString(digest[:])
}
