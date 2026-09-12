package worker

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

// The provider is a fixed response fixture. This tests the production call,
// decoding, and PostgreSQL evidence path, not live-model understanding.
func TestDatabaseParameterCallEvidenceRetainsFocusedInputAndActualResponse(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for parameter call-evidence coverage")
	}
	for _, fixture := range []struct {
		relation, field, purpose, response string
		window                             bool
	}{
		{"shipments", "dispatched_at", "Include the previous five days measured on dispatched_at.", "5", true},
		{"samples", "reading", "Require the average reading to exceed seven and a half.", "7.5", false},
	} {
		t.Run(fixture.relation, func(t *testing.T) {
			_, repository := freshWorkerEvidenceRepository(t, databaseURL)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise focused parameter evidence", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "parameter-evidence-worker")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v err=%v", claim, err)
			}
			provider := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: fixture.response}}}
			service := &Service{
				repo: repository, stationClient: provider, inferenceContextTokens: "8192",
				runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
			}
			runtime := portableWorkerRuntime(&nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}, "parameter-evidence")
			state := databaseParameterEvidenceState(fixture.relation, fixture.field, fixture.window)
			var work assemblyline.PortableJob
			var decode objectiveRawLeafDecoder[string]
			if fixture.window {
				input := assemblyline.DatabaseQueryWindowLeafInput{
					State: state, Purpose: fixture.purpose, FieldID: "focused-field", Unit: datasource.WindowDay,
				}
				work, err = assemblyline.NewDatabaseQueryWindowAmountJob(input)
				decode = func(raw string) (string, error) {
					amount, err := assemblyline.DecodeDatabaseQueryWindowAmountLeaf(input, raw)
					return strconv.Itoa(amount), err
				}
			} else {
				input := assemblyline.DatabaseQueryHavingLeafInput{
					State: state, Purpose: fixture.purpose, FieldID: "focused-field",
					Aggregate: datasource.AggregateAverage, Operator: datasource.FilterGT,
				}
				work, err = assemblyline.NewDatabaseQueryHavingValueJob(input)
				decode = func(raw string) (string, error) {
					value, err := assemblyline.DecodeDatabaseQueryHavingValueLeaf(input, raw)
					return value.Value, err
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			for attempt := range 2 {
				value, err := runObjectiveRawLeafWorkerCall(runtime, "fixture-model", "database-parameter", work, decode)
				if err != nil || value != fixture.response {
					t.Fatalf("attempt %d decoded %q: %v", attempt, value, err)
				}
			}
			if provider.calls != 1 || runtime.ProviderCalls() != 1 {
				t.Fatalf("exact accepted replay invoked provider %d times", provider.calls)
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
			if err != nil || len(calls) != 1 {
				t.Fatalf("call evidence count=%d err=%v", len(calls), err)
			}
			recorded := calls[0]
			expectedPrompt, err := assemblyline.RenderPortableJob(work)
			if err != nil {
				t.Fatal(err)
			}
			expectedRequest, err := llm.ExactPreparedRequestBytes(provider.prepared[0])
			if err != nil {
				t.Fatal(err)
			}
			if recorded.ModelInput != expectedPrompt || recorded.ModelInputBytes != len(expectedPrompt) ||
				!bytes.Equal(recorded.ProviderRequest, expectedRequest) || recorded.Candidate != fixture.response ||
				!recorded.RawResponsePresent || len(recorded.RawResponse) == 0 ||
				recorded.Outcome == nil || recorded.Outcome.Status != queue.LLMCallAccepted {
				t.Fatalf("persisted call does not match actual input, response, and accepted outcome: %#v", recorded)
			}
			if !strings.Contains(recorded.ModelInput, fixture.purpose) || !strings.Contains(recorded.ModelInput, fixture.field) {
				t.Fatal("provider did not receive the focused semantic question")
			}
			for _, unrelated := range []string{
				"unrelated_", "ACCEPTED PROJECTIONS", "ACCEPTED TEMPORAL WINDOWS", "ACCEPTED HAVING PREDICATES",
				"exact_question", "deterministic_consumer", "semantic-uncertainty", "source-1", "need-1",
			} {
				if strings.Contains(recorded.ModelInput, unrelated) {
					t.Errorf("actual model input exposed unrelated state %q", unrelated)
				}
			}
			if !strings.Contains(string(recorded.WorkInput), "unrelated_measure") {
				t.Fatal("code lost the retained state while narrowing model input")
			}
			t.Logf("one provider call; %d model-input bytes; exact accepted replay", recorded.ModelInputBytes)
		})
	}
}

func databaseParameterEvidenceState(relation, field string, temporal bool) assemblyline.DatabaseQueryIntentLeafState {
	authority := databaseSingleChoiceIntentInput()
	typeCategory := datasource.TypeDecimal
	if temporal {
		typeCategory = datasource.TypeTemporal
	}
	authority.SchemaProjection.Relations[0].Name = relation
	authority.SchemaProjection.Relations[0].Columns = []datasource.IntentColumnProjection{
		{ID: "focused-field", Name: field, TypeCategory: typeCategory},
		{ID: "other-value", Name: "unrelated_measure", TypeCategory: datasource.TypeInteger},
		{ID: "other-time", Name: "unrelated_timestamp", TypeCategory: datasource.TypeTemporal},
	}
	state := assemblyline.NewDatabaseQueryIntentLeafState(authority)
	state.FromRelationID = "metrics"
	state.Shape = datasource.ResultRecords
	state.Projections = []datasource.RelationalProjection{{FieldID: "other-value"}}
	state.TemporalWindows = []assemblyline.DatabaseTemporalWindowDecision{{FieldID: "other-time", Unit: datasource.WindowYear, Amount: 73}}
	state.Having = []datasource.AggregatePredicate{{
		Aggregate: datasource.AggregateSum, FieldID: "other-value", Operator: datasource.FilterGT,
		Value: datasource.IntentLiteral{Type: datasource.LiteralInteger, Value: "391"},
	}}
	return state
}
