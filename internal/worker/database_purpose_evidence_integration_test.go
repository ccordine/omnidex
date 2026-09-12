package worker

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
)

// This runs the production query-intent pipeline and PostgreSQL call records
// with fixed provider responses. It is not live interpretation or SQL-read proof.
func TestDatabasePurposeInventoriesRecordReducedCallsAndReplay(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for query-purpose evidence")
	}
	for _, fixture := range []struct {
		relation, field, value string
		category               datasource.ColumnTypeCategory
		literal                datasource.IntentLiteral
	}{
		{"samples", "reading", "7", datasource.TypeInteger, datasource.IntentLiteral{Type: datasource.LiteralInteger, Value: "7"}},
		{"shipments", "status", "pending", datasource.TypeText, datasource.IntentLiteral{Type: datasource.LiteralString, Value: "pending"}},
	} {
		t.Run(fixture.relation, func(t *testing.T) {
			pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
			config, err := modelconfig.Freeze(modelconfig.Config{"database_query_intent_model": "fixture-query"})
			if err != nil {
				t.Fatal(err)
			}
			repository := queue.New(pool, config)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise database-purpose intake", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "query-purpose-worker")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v error=%v", claim, err)
			}
			input := databaseSingleChoiceIntentInput()
			input.ExactNeed = "Return " + fixture.field + " for " + fixture.relation + " whose " + fixture.field + " equals " + fixture.value + "."
			input.SchemaProjection.Relations[0].Name = fixture.relation
			input.SchemaProjection.Relations[0].Columns = []datasource.IntentColumnProjection{{ID: "selected-field", Name: fixture.field, TypeCategory: fixture.category}}
			projection := "Return each matching " + fixture.field + "."
			filter := "Require " + fixture.field + " to equal " + fixture.value + "."
			equivalent := "The " + fixture.field + " must be equal to " + fixture.value + "."
			const unsupported = "Use newest records first."
			responses := []string{
				"A", projection, "A",
				strings.Join([]string{filter, filter, unsupported, equivalent}, "\n"),
				"A", "B", "A", "A", "A", fixture.value,
				assemblyline.DatabaseNoQueryPurposeCandidates, assemblyline.DatabaseNoQueryPurposeCandidates,
				assemblyline.DatabaseNoQueryPurposeCandidates, assemblyline.DatabaseNoQueryPurposeCandidates,
			}
			kinds := []assemblyline.WorkKind{
				assemblyline.WorkDatabaseQueryShape, assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeNecessity,
				assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeNecessity,
				assemblyline.WorkDatabaseQueryPurposeNecessity, assemblyline.WorkDatabaseQueryPurposeNecessity, assemblyline.WorkDatabaseQueryPurposeRelation,
				assemblyline.WorkDatabaseQueryFilterOperator, assemblyline.WorkDatabaseQueryFilterValue,
				assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeInventory,
				assemblyline.WorkDatabaseQueryPurposeInventory, assemblyline.WorkDatabaseQueryPurposeInventory,
			}
			provider := &exactEvidenceStationClient{}
			for _, response := range responses {
				provider.fixtures = append(provider.fixtures, exactEvidenceStationFixture{candidate: response})
			}
			var accepted assemblyline.DatabaseQueryIntentDecision
			for attempt := range 2 {
				service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "32768", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
				station := portableObjectiveDatabaseStations{runtime: &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}}
				decision, dispatches, err := station.BuildIntent(ctx, input)
				wantCalls := 0
				if attempt == 0 {
					wantCalls = len(responses)
				}
				if err != nil || dispatches != wantCalls || provider.calls != len(responses) {
					t.Fatalf("attempt=%d dispatches=%+v provider=%d error=%v", attempt, dispatches, provider.calls, err)
				}
				if decision.FromRelationID != "metrics" || decision.Shape != datasource.ResultRecords || decision.EvidenceNeedID != input.EvidenceNeedID ||
					!reflect.DeepEqual(decision.Projections, []datasource.RelationalProjection{{FieldID: "selected-field"}}) ||
					!reflect.DeepEqual(decision.Filters, []datasource.RelationalPredicate{{FieldID: "selected-field", Operator: datasource.FilterEqual, Values: []datasource.IntentLiteral{fixture.literal}}}) ||
					len(decision.TemporalWindows)+len(decision.Exists)+len(decision.Having)+len(decision.OrderBy) != 0 {
					t.Fatalf("query intent differs from the accepted semantic leaves: %+v", decision)
				}
				if attempt == 0 {
					accepted = decision
				} else if !reflect.DeepEqual(decision, accepted) {
					t.Fatal("database replay changed the accepted query intent")
				}
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
			if err != nil || len(calls) != len(responses) {
				t.Fatalf("recorded calls=%d error=%v", len(calls), err)
			}
			for index, recorded := range calls {
				assertPortableLeafRecordedCall(t, recorded, provider.prepared[index], kinds[index], responses[index], "fixture-query")
				assertGroundedCallOmits(t, recorded.ModelInput, "source-1", "need-1", "selected-field", string(kinds[index]), `"schema"`, `"candidates"`)
				if index == 7 {
					assertGroundedCallContains(t, recorded.ModelInput, filter, equivalent)
					assertGroundedCallOmits(t, recorded.ModelInput, input.ExactNeed, projection, unsupported)
				} else if index == 5 || index == 6 {
					assertGroundedCallOmits(t, recorded.ModelInput, filter, projection)
				}
				t.Logf("%s: %d model-input bytes, exact response %q", recorded.WorkKind, recorded.ModelInputBytes, recorded.Candidate)
			}
		})
	}
}
