package worker

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

// Fixed provider responses exercise the real call and PostgreSQL evidence path.
// These are focused selection fixtures, not a live-model or autonomy benchmark.
func TestDatabaseSelectionCallEvidenceRetainsOnlyLocalAuthority(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for selection call-evidence coverage")
	}
	for _, fixture := range []struct {
		relation, field, purpose, response, expected string
		aggregate                                    bool
	}{
		{"shipments", "weight", "Show the total weight.", "A", "focused-field", false},
		{"circuits", "connected", "Require more than three non-null connected values.", "B", string(datasource.AggregateCount), true},
	} {
		t.Run(fixture.relation, func(t *testing.T) {
			_, repository := freshWorkerEvidenceRepository(t, databaseURL)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise focused selection evidence", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "selection-evidence-worker")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v err=%v", claim, err)
			}
			provider := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: fixture.response}}}
			service := &Service{
				repo: repository, stationClient: provider, inferenceContextTokens: "8192",
				runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
			}
			runtime := portableWorkerRuntime(&nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}, "selection-evidence")
			state := databaseParameterEvidenceState(fixture.relation, fixture.field, false)
			var work assemblyline.PortableJob
			var decode objectiveRawLeafDecoder[string]
			if fixture.aggregate {
				state.Authority.SchemaProjection.Relations[0].Columns = []datasource.IntentColumnProjection{
					{ID: "focused-field", Name: fixture.field, TypeCategory: datasource.TypeBoolean, Nullable: true},
					{ID: "other-value", Name: "unrelated_flag", TypeCategory: datasource.TypeBoolean},
				}
				state.TemporalWindows = []assemblyline.DatabaseTemporalWindowDecision{}
				state.Having[0].Aggregate = datasource.AggregateCountRows
				state.Having[0].FieldID = ""
				input := assemblyline.DatabaseQueryHavingLeafInput{State: state, Purpose: fixture.purpose}
				work, err = assemblyline.NewDatabaseQueryHavingAggregateJob(input)
				decode = func(raw string) (string, error) {
					value, err := assemblyline.DecodeDatabaseQueryHavingAggregateLeaf(input, raw)
					return string(value), err
				}
			} else {
				input := assemblyline.DatabaseQueryProjectionLeafInput{
					State: state, Purpose: fixture.purpose, Aggregate: datasource.AggregateSum,
				}
				work, err = assemblyline.NewDatabaseQueryProjectionFieldJob(input)
				decode = func(raw string) (string, error) {
					return assemblyline.DecodeDatabaseQueryProjectionFieldLeaf(input, raw)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			for attempt := range 2 {
				value, err := runObjectiveRawLeafWorkerCall(runtime, "fixture-model", "database-selection", work, decode)
				if err != nil || value != fixture.expected {
					t.Fatalf("attempt %d decoded %q: %v", attempt, value, err)
				}
			}
			if provider.calls != 1 || runtime.ProviderCalls() != 1 {
				t.Fatalf("accepted selection replay invoked provider %d times", provider.calls)
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
				t.Fatal("persisted evidence does not match the actual selection input, response, and acceptance")
			}
			if !strings.Contains(recorded.ModelInput, fixture.purpose) || !strings.Contains(recorded.ModelInput, fixture.field) {
				t.Fatal("provider did not receive the focused selection meaning")
			}
			for _, hidden := range []string{
				"ACCEPTED RESULT SHAPE", "ACCEPTED PROJECTIONS", "ACCEPTED TEMPORAL WINDOWS",
				"ACCEPTED HAVING PREDICATES", "391", "exact_question", "deterministic_consumer", "source-1", "need-1",
			} {
				if strings.Contains(recorded.ModelInput, hidden) {
					t.Errorf("actual selection input exposed unrelated state %q", hidden)
				}
			}
			if fixture.aggregate && (strings.Contains(recorded.ModelInput, "sum numeric") || strings.Contains(recorded.ModelInput, "average numeric")) {
				t.Error("provider received a numeric-only aggregate for a boolean-only schema")
			}
			if !strings.Contains(string(recorded.WorkInput), "391") {
				t.Fatal("narrowing the selection prompt discarded retained validation state")
			}
			t.Logf("one provider call; %d model-input bytes; exact accepted selection replay", recorded.ModelInputBytes)
		})
	}
}
