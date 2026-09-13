package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// Fixed responses exercise the production database workflow and actual SQL.
// They establish call boundaries and execution, not live-model understanding.
func TestDatabaseClosedSubsetWorkflowRecordsChoicesAndExecutedRows(t *testing.T) {
	for _, fixture := range []struct {
		databaseWorkflowCase
		purpose, operator string
		choices, selected []string
		rows              []string
	}{
		{
			databaseWorkflowCase: databaseWorkflowCase{
				table: "documents", field: "state", value: "draft or published",
				ddl:    "CREATE TYPE document_state AS ENUM ('draft', 'published', 'archived'); CREATE TABLE documents (state document_state NOT NULL)",
				insert: "INSERT INTO documents VALUES ('draft'), ('published'), ('archived')",
			},
			purpose: "Include only draft or published states.", operator: "C",
			choices: []string{"A", "A", "B"}, selected: []string{"draft", "published"}, rows: []string{"draft", "published"},
		},
		{
			databaseWorkflowCase: databaseWorkflowCase{
				table: "sensors", field: "enabled", value: "true",
				ddl: "CREATE TABLE sensors (enabled boolean NOT NULL)", insert: "INSERT INTO sensors VALUES (true), (false)",
			},
			purpose: "Exclude disabled sensors.", operator: "D",
			choices: []string{"B", "B"}, selected: []string{"false"}, rows: []string{"true"},
		},
	} {
		t.Run(fixture.table, func(t *testing.T) {
			run := newDatabaseWorkflowFixture(t, fixture.databaseWorkflowCase)
			responses := []string{"A", "Return each matching " + fixture.field + ".", "A", fixture.purpose, "A", fixture.operator}
			kinds := []assemblyline.WorkKind{
				assemblyline.WorkDatabaseQueryShape, assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeNecessity,
				assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeNecessity, assemblyline.WorkDatabaseQueryFilterOperator,
			}
			for _, response := range fixture.choices {
				responses = append(responses, response)
				kinds = append(kinds, assemblyline.WorkDatabaseQueryFilterValueChoice)
			}
			for range 4 {
				responses = append(responses, assemblyline.DatabaseNoQueryPurposeCandidates)
				kinds = append(kinds, assemblyline.WorkDatabaseQueryPurposeInventory)
			}
			for _, response := range responses {
				run.provider.fixtures = append(run.provider.fixtures, exactEvidenceStationFixture{candidate: response})
			}
			ctx := context.Background()
			for attempt := range 2 {
				acquisition, err := runObjectiveDatabaseEvidenceWorkflow(ctx, run.authority, "subset-need", run.snapshot,
					portableObjectiveDatabaseStations{runtime: run.runtime()}, run.execute)
				wantCalls := 0
				if attempt == 0 {
					wantCalls = len(responses)
				}
				if err != nil || acquisition.ModelCalls != wantCalls || len(acquisition.Evidence) != 1 {
					t.Fatalf("attempt=%d calls=%d evidence=%d error=%v", attempt, acquisition.ModelCalls, len(acquisition.Evidence), err)
				}
				recorded, err := run.repository.GetDatabaseEvidence(ctx, run.claim.Job.ID, acquisition.Evidence[0].DatabaseEvidenceID)
				if err != nil {
					t.Fatal(err)
				}
				query := recorded.Evidence.Execution.Query
				if len(query.Parameters) != len(fixture.selected)+1 || !strings.Contains(query.SQL, "IN (") {
					t.Fatalf("membership query did not bind the selected set: %+v", query)
				}
				for index, expected := range fixture.selected {
					if query.Parameters[index].Value != expected {
						t.Fatalf("parameter %d = %v; expected %q", index, query.Parameters[index], expected)
					}
				}
				rows := make([]string, len(recorded.Evidence.Result.Rows))
				for index, row := range recorded.Evidence.Result.Rows {
					rows[index] = row[0].Value
				}
				sort.Strings(rows)
				if !reflect.DeepEqual(rows, fixture.rows) {
					t.Fatalf("executed rows=%v; expected %v", rows, fixture.rows)
				}
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, run.repository, run.claim.Job.ID)
			if err != nil || len(calls) != len(responses) || run.provider.calls != len(responses) {
				t.Fatalf("recorded=%d provider=%d error=%v", len(calls), run.provider.calls, err)
			}
			choiceIndex := 0
			for index, call := range calls {
				assertPortableLeafRecordedCall(t, call, run.provider.prepared[index], kinds[index], responses[index], "fixture-query")
				if kinds[index] != assemblyline.WorkDatabaseQueryFilterValueChoice {
					continue
				}
				var input assemblyline.DatabaseQueryFilterLeafInput
				if err := json.Unmarshal(call.WorkInput, &input); err != nil {
					t.Fatal(err)
				}
				if len(input.AcceptedValues) != choiceIndex {
					t.Fatalf("round %d retained %d values", choiceIndex, len(input.AcceptedValues))
				}
				for _, value := range input.AcceptedValues {
					if strings.Contains(call.ModelInput, "The exact value \""+value.Value+"\"") {
						t.Fatalf("accepted value appeared in a later provider choice: %s", call.ModelInput)
					}
				}
				choiceIndex++
			}
			t.Logf("%d subset calls, %d total calls, real SQL rows %v, zero-call accepted replay", choiceIndex, len(calls), fixture.rows)
		})
	}
}
