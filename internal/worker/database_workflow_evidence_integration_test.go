package worker

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

// Fixed model text exercises the production query workflow, real SQL execution,
// and completion's recorded-source checks. This is not an autonomy benchmark.
func TestDatabaseWorkflowUsesExecutedRowsWithoutCounterProofLedger(t *testing.T) {
	for _, fixture := range databaseWorkflowCases() {
		t.Run(fixture.table, func(t *testing.T) {
			run := newDatabaseWorkflowFixture(t, fixture)
			responses := []string{
				"A", "Return each matching " + fixture.field + ".", "A",
				"Require " + fixture.field + " to equal " + fixture.value + ".", "A", "A", fixture.value,
				assemblyline.DatabaseNoQueryPurposeCandidates, assemblyline.DatabaseNoQueryPurposeCandidates,
				assemblyline.DatabaseNoQueryPurposeCandidates, assemblyline.DatabaseNoQueryPurposeCandidates,
			}
			kinds := []assemblyline.WorkKind{
				assemblyline.WorkDatabaseQueryShape, assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeNecessity,
				assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeNecessity,
				assemblyline.WorkDatabaseQueryFilterOperator, assemblyline.WorkDatabaseQueryFilterValue,
				assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeInventory,
				assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeInventory,
			}
			for _, response := range responses {
				run.provider.fixtures = append(run.provider.fixtures, exactEvidenceStationFixture{candidate: response})
			}
			ctx := context.Background()
			initial := objectiveTurnResult{ObjectiveID: objectiveTurnID(run.authority), Kind: assemblyline.ObjectiveKindDatabaseRead}
			initial.RequirementID = objectiveRequirementID(initial.ObjectiveID)
			var result objectiveTurnResult
			var recorded queue.DatabaseEvidenceRecord
			for attempt := range 2 {
				acquisition, err := runObjectiveDatabaseEvidenceWorkflow(ctx, run.authority, initial.RequirementID,
					run.snapshot, portableObjectiveDatabaseStations{runtime: run.runtime()}, run.execute)
				wantCalls := 0
				if attempt == 0 {
					wantCalls = len(responses)
				}
				if err != nil || acquisition.ModelCalls != wantCalls || len(acquisition.Evidence) != 1 || run.provider.calls != len(responses) {
					t.Fatalf("attempt=%d acquisition=%+v provider=%d error=%v", attempt, acquisition, run.provider.calls, err)
				}
				previousID := recorded.ID
				recorded, err = run.repository.GetDatabaseEvidence(ctx, run.claim.Job.ID, acquisition.Evidence[0].DatabaseEvidenceID)
				if err != nil || recorded.ID == previousID || recorded.Evidence.Result.RowCount != 1 ||
					recorded.Evidence.Result.Rows[0][0].Value != fixture.value || len(recorded.Evidence.Execution.Query.Parameters) != 2 ||
					recorded.Evidence.Execution.Query.Parameters[0].Value != fixture.value ||
					recorded.Evidence.Execution.Query.Parameters[1] != (datasource.ExecutedParameter{Position: 2, Type: string(datasource.LiteralInteger), Value: strconv.Itoa(maxObjectiveDatabaseRows)}) ||
					strings.Contains(recorded.Evidence.Execution.Query.SQL, fixture.value) {
					t.Fatalf("actual SQL, parameters, or returned rows differ: %+v / %v", recorded, err)
				}
				answer := databaseUsageAnswerFunc(func(input assemblyline.GroundedAnswerInput) (assemblyline.GroundedAnswerDecision, error) {
					return assemblyline.AssembleGroundedAnswerDecision(input, []assemblyline.GroundedAnswerParagraph{{
						Text: "The recorded value is " + fixture.value + ".", EvidenceIDs: []string{input.Evidence[0].ID},
					}})
				})
				result, err = runObjectiveDatabaseRead(ctx, run.authority, initial, answer,
					func(context.Context, turnAuthority, string) (objectiveEvidenceAcquisition, error) {
						return acquisition, nil
					})
				if err != nil || !result.Complete || result.ModelCalls != wantCalls || !reflect.DeepEqual(result.Citations, acquisition.Evidence) {
					t.Fatalf("actual acquisition did not reach response completion: %+v / %v", result, err)
				}
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, run.repository, run.claim.Job.ID)
			if err != nil || len(calls) != len(responses) {
				t.Fatalf("recorded model calls=%d error=%v", len(calls), err)
			}
			for index, call := range calls {
				assertPortableLeafRecordedCall(t, call, run.provider.prepared[index], kinds[index], responses[index], "fixture-query")
			}
			for _, mutation := range []struct {
				name  string
				apply func(*queue.DatabaseEvidenceRecord)
			}{
				{"wrong job", func(value *queue.DatabaseEvidenceRecord) { value.JobID++ }},
				{"wrong query", func(value *queue.DatabaseEvidenceRecord) { value.Plan.Intent.Limit-- }},
				{"invalid rows", func(value *queue.DatabaseEvidenceRecord) { value.Evidence.Result.RowCount++ }},
			} {
				changed := recorded
				mutation.apply(&changed)
				acquisition, err := runObjectiveDatabaseEvidenceWorkflow(ctx, run.authority, initial.RequirementID, run.snapshot,
					portableObjectiveDatabaseStations{runtime: run.runtime()},
					func(context.Context, datasource.SchemaSnapshot, datasource.RelationalQueryPlan) (queue.DatabaseEvidenceRecord, error) {
						return changed, nil
					})
				if err == nil || len(acquisition.Evidence) != 0 || acquisition.ModelCalls != 0 {
					t.Fatalf("%s bypassed actual execution validation: %+v / %v", mutation.name, acquisition, err)
				}
			}
			statement := "ALTER TABLE " + pgx.Identifier{fixture.table}.Sanitize() + " RENAME COLUMN " + pgx.Identifier{fixture.field}.Sanitize() + " TO missing_field"
			if _, err := run.data.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
			failed, err := runObjectiveDatabaseEvidenceWorkflow(ctx, run.authority, initial.RequirementID, run.snapshot,
				portableObjectiveDatabaseStations{runtime: run.runtime()}, run.execute)
			if err == nil || !strings.Contains(err.Error(), "explain evidence query") || len(failed.Evidence) != 0 || failed.ModelCalls != 0 || run.provider.calls != len(responses) {
				t.Fatalf("missing queried field did not fail at execution: %+v / %v", failed, err)
			}
			assertDatabaseWorkflowCompletion(t, run, result)
			var reads int
			if err := run.pool.QueryRow(ctx, "SELECT count(*) FROM database_evidence WHERE job_id=$1", run.claim.Job.ID).Scan(&reads); err != nil || reads != 2 {
				t.Fatalf("expected exactly two executed reads, got %d / %v", reads, err)
			}
		})
	}
}

func assertDatabaseWorkflowCompletion(t *testing.T, run databaseWorkflowFixture, result objectiveTurnResult) {
	t.Helper()
	output, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil || len(records) != 1 {
		t.Fatalf("prepare recorded query citation: %+v / %v", records, err)
	}
	for index := range records {
		records[index].JobID = run.claim.Job.ID
		records[index].StepID = run.claim.Step.ID
	}
	operation, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	command := queue.CompleteStepEvidenceCommand{CompleteStepCommand: queue.CompleteStepCommand{
		OperationID: operation, Authority: run.claim.Authority, StepID: run.claim.Step.ID,
		Output: output, ContextKey: "objective_result",
	}, Evidence: records}
	invalid := command
	changed := records[0]
	changed.Excerpt = "Invented rows."
	invalid.Evidence = []evidence.Record{changed}
	if err := run.repository.CompleteStepWithEvidence(context.Background(), invalid); err == nil || !strings.Contains(err.Error(), "excerpt differs from the recorded query rows") {
		t.Fatalf("completion did not reject the invented query rows at the recorded-source check: %v", err)
	}
	for range 2 {
		if err := run.repository.CompleteStepWithEvidence(context.Background(), command); err != nil {
			t.Fatalf("complete exact executed query citation: %v", err)
		}
	}
}
