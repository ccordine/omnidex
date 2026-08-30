package worker

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/station"
)

func TestObjectiveDatabaseSchemaStationRestoresEveryLeafBeforeModelResolution(t *testing.T) {
	t.Parallel()
	input := assemblyline.DatabaseSchemaSelectionInput{
		EvidenceNeedID: "need-schema-reuse",
		ExactNeed:      "Find the appointment records.",
		Context:        assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		Candidates: []assemblyline.DatabaseSchemaCandidate{{
			RelationID: "relation_a", Descriptor: "appointment records",
		}},
		MaxSelections: 1,
	}
	runtime, reuseCalls := objectiveDatabaseReuseRuntime(
		t,
		station.DatabaseSchemaSelection,
		[]assemblyline.WorkKind{
			assemblyline.WorkDatabaseSchemaRelationInventory,
			assemblyline.WorkDatabaseSchemaRelationNecessity,
			assemblyline.WorkDatabaseSchemaRelationResolution,
		},
		[]string{
			"The appointment records.",
			assemblyline.DatabaseSchemaRelationNecessary,
			"relation_a",
		},
	)
	decision, receipt, err := (portableObjectiveDatabaseStations{runtime: runtime}).SelectSchema(
		t.Context(), input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if *reuseCalls != 3 || receipt != (objectiveStationReceipt{Reused: true}) ||
		len(decision.RelationIDs) != 1 || decision.RelationIDs[0] != "relation_a" {
		t.Fatalf("reuse_calls=%d receipt=%+v decision=%+v", *reuseCalls, receipt, decision)
	}
}

func TestObjectiveDatabaseQueryStationRestoresEveryLeafBeforeModelResolution(t *testing.T) {
	t.Parallel()
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	projection, err := datasource.ProjectSchemaForIntent(
		snapshot, []string{snapshot.Relations[0].ID},
	)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.DatabaseQueryIntentInput{
		EvidenceNeedID:   "need-query-reuse",
		ExactNeed:        "How many records exist?",
		Context:          assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		SchemaProjection: projection,
		TemporalAsOf:     snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
		MaxRows:          maxObjectiveDatabaseRows,
	}
	runtime, reuseCalls := objectiveDatabaseReuseRuntime(
		t,
		station.DatabaseQueryIntent,
		[]assemblyline.WorkKind{
			assemblyline.WorkDatabaseQueryShape,
			assemblyline.WorkDatabaseQueryPurposeInventory,
			assemblyline.WorkDatabaseQueryPurposeNecessity,
			assemblyline.WorkDatabaseQueryProjectionAggregate,
			assemblyline.WorkDatabaseQueryPurposeInventory,
			assemblyline.WorkDatabaseQueryPurposeInventory,
			assemblyline.WorkDatabaseQueryPurposeInventory,
			assemblyline.WorkDatabaseQueryPurposeInventory,
			assemblyline.WorkDatabaseQueryPurposeInventory,
		},
		[]string{
			"scalar",
			"count the records",
			assemblyline.DatabaseQueryPurposeNecessary,
			"count_rows",
			assemblyline.DatabaseQueryNoPurposeCandidates,
			assemblyline.DatabaseQueryNoPurposeCandidates,
			assemblyline.DatabaseQueryNoPurposeCandidates,
			assemblyline.DatabaseQueryNoPurposeCandidates,
			assemblyline.DatabaseQueryNoPurposeCandidates,
		},
	)
	decision, receipt, err := (portableObjectiveDatabaseStations{runtime: runtime}).BuildIntent(
		t.Context(), input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if *reuseCalls != 9 || receipt != (objectiveStationReceipt{Reused: true}) ||
		decision.Shape != datasource.ResultScalar || len(decision.Projections) != 1 ||
		decision.Projections[0].Aggregate != datasource.AggregateCountRows {
		t.Fatalf("reuse_calls=%d receipt=%+v decision=%+v", *reuseCalls, receipt, decision)
	}
}

func TestObjectiveDatabaseSchemaAndQueryLedgersPreserveMixedReceipts(t *testing.T) {
	t.Parallel()
	candidates := make([]assemblyline.DatabaseSchemaCandidate, databaseSchemaSelectionChunk+1)
	for index := range candidates {
		candidates[index] = assemblyline.DatabaseSchemaCandidate{
			RelationID: "relation_" + strings.Repeat("x", index+1),
			Descriptor: "bounded relation candidate",
		}
	}
	stations := &mixedDatabaseSchemaSelectionStations{
		scriptedObjectiveDatabaseStations: &scriptedObjectiveDatabaseStations{t: t},
		receipts: []objectiveStationReceipt{
			{Reused: true},
			{Calls: exactSemanticLeafCalls},
		},
	}
	_, schemaReceipt, err := reduceObjectiveDatabaseCandidates(
		t.Context(), "need-schema-mixed", "Find the relevant records.",
		assemblyline.ObjectiveContext{}, candidates, stations,
	)
	if err != nil {
		t.Fatal(err)
	}
	if schemaReceipt != (objectiveStationReceipt{Calls: exactSemanticLeafCalls}) {
		t.Fatalf("mixed schema receipt=%+v", schemaReceipt)
	}

	var queryLedger objectiveDatabaseRawLeafCallLedger
	if err := queryLedger.record(
		"database_query_shape", objectiveStationReceipt{Reused: true},
	); err != nil {
		t.Fatal(err)
	}
	if err := queryLedger.record(
		"database_query_projection", objectiveStationReceipt{Calls: exactSemanticLeafCalls},
	); err != nil {
		t.Fatal(err)
	}
	queryReceipt, err := queryLedger.complete(exactSemanticLeafCalls)
	if err != nil {
		t.Fatal(err)
	}
	if queryReceipt != (objectiveStationReceipt{Calls: exactSemanticLeafCalls}) {
		t.Fatalf("mixed query receipt=%+v", queryReceipt)
	}
}

func TestObjectiveDatabaseAcquisitionAcceptsRestoredAndFreshIntentReceipts(t *testing.T) {
	for _, fixture := range []struct {
		name          string
		intentReceipt objectiveStationReceipt
		want          objectiveStationReceipt
	}{
		{
			name:          "fully restored",
			intentReceipt: objectiveStationReceipt{Reused: true},
			want:          objectiveStationReceipt{Reused: true},
		},
		{
			name:          "fresh intent",
			intentReceipt: objectiveStationReceipt{Calls: exactSemanticLeafCalls},
			want:          objectiveStationReceipt{Calls: exactSemanticLeafCalls},
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			snapshot := objectiveDatabaseSingleRelationSnapshot(t)
			stations := &databaseReceiptOverrideStations{
				scriptedObjectiveDatabaseStations: &scriptedObjectiveDatabaseStations{t: t},
				intentReceipt:                     fixture.intentReceipt,
			}
			result, err := runObjectiveDatabaseEvidenceWorkflow(
				t.Context(),
				turnAuthority{
					JobID: 9301, Pipeline: model.PipelineChat,
					Instruction: "Count the appointments.", ModelInstruction: "Count the appointments.",
					ModelArtifactPaths: []string{}, DataSourceID: "source-1",
				},
				"requirement-db-reuse", snapshot, stations,
				func(_ context.Context, _ datasource.SchemaSnapshot, plan datasource.RelationalQueryPlan) (datasource.EvidenceResult, error) {
					return objectiveDatabaseCountEvidence(plan, 1), nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := result.DatabaseCallLedger.totalForSuccess()
			if err != nil {
				t.Fatal(err)
			}
			if receipt != fixture.want || result.ModelCalls != fixture.want.Calls || len(result.Evidence) != 1 {
				t.Fatalf("receipt=%+v result=%+v", receipt, result)
			}
		})
	}
}

func TestObjectiveDatabaseRejectsAbsentAndMalformedReceiptProvenance(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name    string
		receipt objectiveStationReceipt
	}{
		{name: "zero calls without reuse proof"},
		{name: "fresh call marked reused", receipt: objectiveStationReceipt{Calls: 1, Reused: true}},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			var rawLedger objectiveDatabaseRawLeafCallLedger
			if err := rawLedger.record("invalid", fixture.receipt); err == nil {
				t.Fatalf("raw ledger accepted receipt %+v", fixture.receipt)
			}
			var acquisitionLedger objectiveDatabaseAcquisitionCallLedger
			if err := acquisitionLedger.record(
				"invalid", fixture.receipt, exactSemanticLeafCalls,
			); err == nil {
				t.Fatalf("acquisition ledger accepted receipt %+v", fixture.receipt)
			}
		})
	}

	corrupt := objectiveDatabaseAcquisitionCallLedger{
		objectiveDatabaseBoundedCallLedger: objectiveDatabaseBoundedCallLedger{
			receipts: []objectiveDatabaseBoundedCallReceipt{{
				scope: "database acquisition", label: "query intent",
				receipt: objectiveStationReceipt{}, maximumCalls: maxObjectiveDatabaseQueryIntentCalls,
			}},
		},
	}
	if _, err := corrupt.totalForSuccess(); err == nil {
		t.Fatal("database acquisition accepted a malformed restored receipt")
	}
}

func TestObjectiveDatabaseReadRejectsMissingOrMismatchedReceiptLedger(t *testing.T) {
	t.Parallel()
	authority := turnAuthority{DataSourceID: "source-1"}
	for _, fixture := range []struct {
		name        string
		acquisition objectiveEvidenceAcquisition
	}{
		{name: "missing exact source"},
		{
			name: "counter differs from restored source",
			acquisition: objectiveEvidenceAcquisition{
				ModelCalls: 1,
				DatabaseCallLedger: func() objectiveDatabaseAcquisitionCallLedger {
					var ledger objectiveDatabaseAcquisitionCallLedger
					if err := ledger.record(
						"query intent", objectiveStationReceipt{Reused: true},
						maxObjectiveDatabaseQueryIntentCalls,
					); err != nil {
						t.Fatal(err)
					}
					return ledger
				}(),
			},
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			_, err := runObjectiveDatabaseRead(
				t.Context(), authority, objectiveTurnResult{}, nil,
				func(context.Context, turnAuthority, string) (objectiveEvidenceAcquisition, error) {
					return fixture.acquisition, nil
				},
			)
			if err == nil {
				t.Fatalf("database read accepted acquisition %+v", fixture.acquisition)
			}
		})
	}
}

func TestObjectiveDatabaseSourceHasNoFreshCallMinimumOrDirectLeafBypass(t *testing.T) {
	t.Parallel()
	rawLeafSource, err := os.ReadFile("objective_database_raw_leaf.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawLeafSource), "runObjectiveReusablePortableRawLeafCall") ||
		strings.Contains(string(rawLeafSource), "runObjectivePortableRawLeafCall(") {
		t.Fatal("database raw leaf adapter bypasses exact accepted-result reuse")
	}
	workflowSource, err := os.ReadFile("objective_database_workflow.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(workflowSource), "result.ModelCalls +=") {
		t.Fatal("database workflow reconstructs call authority from mutable counters")
	}
	turnSource, err := os.ReadFile("objective_turn_workflow.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(turnSource), "acquisition.ModelCalls < 1") {
		t.Fatal("database-read workflow retains a mandatory fresh-call minimum")
	}
}

func objectiveDatabaseReuseRuntime(
	t *testing.T,
	owner station.ID,
	wantKinds []assemblyline.WorkKind,
	responses []string,
) (*nativeRuntimeV3, *int) {
	t.Helper()
	if len(wantKinds) != len(responses) {
		t.Fatalf("reuse fixture kinds=%d responses=%d", len(wantKinds), len(responses))
	}
	next := 0
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		if request.Station != owner || next >= len(responses) || request.Job.Kind != wantKinds[next] {
			t.Fatalf("reuse request %d=%+v want owner=%s kind=%s", next, request, owner, wantKinds[next])
		}
		candidate := responses[next]
		next++
		projection, err := assemblyline.NewExactPortableResultProjection(candidate)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: candidate, Projection: &projection,
		}}, true, nil
	}}
	return &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}, &next
}

type mixedDatabaseSchemaSelectionStations struct {
	*scriptedObjectiveDatabaseStations
	receipts []objectiveStationReceipt
	next     int
}

func (stations *mixedDatabaseSchemaSelectionStations) SelectSchema(
	_ context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
) (assemblyline.DatabaseSchemaSelectionDecision, objectiveStationReceipt, error) {
	receipt := stations.receipts[stations.next]
	stations.next++
	return assemblyline.DatabaseSchemaSelectionDecision{
		Schema: assemblyline.DatabaseSchemaSelectionV1, EvidenceNeedID: input.EvidenceNeedID,
		RelationIDs: []string{},
	}, receipt, nil
}

type databaseReceiptOverrideStations struct {
	*scriptedObjectiveDatabaseStations
	intentReceipt objectiveStationReceipt
}

func (stations *databaseReceiptOverrideStations) BuildIntent(
	ctx context.Context,
	input assemblyline.DatabaseQueryIntentInput,
) (assemblyline.DatabaseQueryIntentDecision, objectiveStationReceipt, error) {
	decision, _, err := stations.scriptedObjectiveDatabaseStations.BuildIntent(ctx, input)
	return decision, stations.intentReceipt, err
}
