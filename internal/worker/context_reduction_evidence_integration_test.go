package worker

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
)

// Fixed provider responses prove production call boundaries and consumption,
// not the semantic quality of a live model's summary.
func TestContextReductionRecordsOnlyNeededInferenceAndReusesItsResult(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for context-reduction call evidence")
	}
	for _, fixture := range []struct{ instruction, repeated, reduced, tail string }{
		{"When is the inspection?", "The inspection takes place on Tuesday. ", "The inspection is Tuesday.", "The venue is the north hall."},
		{"Which display language is preferred?", "The preferred display language is French. ", "Use French for display text.", "Retain the submitted punctuation."},
	} {
		t.Run(fixture.instruction, func(t *testing.T) {
			pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
			config, err := modelconfig.Freeze(modelconfig.Config{"context_minification_model": "fixture-minification"})
			if err != nil {
				t.Fatal(err)
			}
			repository := queue.New(pool, config)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, fixture.instruction, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "context-reduction-evidence-worker")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v err=%v", claim, err)
			}
			provider := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: fixture.reduced}}}
			service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
			var candidates []assemblyline.ContextCandidateAuthority
			for index := range 10 {
				content := strings.Repeat(fixture.repeated, 8) + fmt.Sprintf("Observation %d.", index+1)
				if index == 8 {
					content = fixture.tail
				}
				if index == 9 {
					content = "This statement remains unchanged."
				}
				candidate, err := assemblyline.NewContextCandidateAuthority("repository", fmt.Sprintf("CTX_%d", index+1), content)
				if err != nil {
					t.Fatal(err)
				}
				candidates = append(candidates, candidate)
			}
			request := contextcompiler.Request{ExactInstruction: fixture.instruction, ModelInstruction: fixture.instruction, KnownArtifactPaths: []string{}}
			var accepted assemblyline.ObjectiveContext
			for attempt := range 2 {
				stations := portableObjectiveContextSieveStations{runtime: &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}}
				result, err := contextcompiler.Compile(ctx, request, contextReductionEvidenceProvider{candidates}, contextcompiler.Stations{Relevance: stations, Minification: stations})
				if err != nil {
					t.Fatalf("attempt %d: %v", attempt, err)
				}
				wantCalls := 1 - attempt
				if result.ModelCalls != wantCalls || provider.calls != 1 {
					t.Fatalf("attempt %d: unnecessary inference: %+v provider=%d", attempt, result, provider.calls)
				}
				want := strings.Join([]string{fixture.reduced, fixture.tail, "This statement remains unchanged."}, "\n\n")
				if len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != want || len(result.Context.Capsules[0].Sources) != len(candidates) {
					t.Fatalf("compiled context did not consume the reduced leaf and exact tail: %+v", result.Context)
				}
				if attempt == 0 {
					accepted = result.Context
				} else if !reflect.DeepEqual(result.Context, accepted) {
					t.Fatal("persisted reuse changed accepted context")
				}
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
			if err != nil || len(calls) != 1 {
				t.Fatalf("call count=%d err=%v", len(calls), err)
			}
			assertPortableLeafRecordedCall(t, calls[0], provider.prepared[0], assemblyline.WorkContextMinification, fixture.reduced, "fixture-minification")
			prompt := calls[0].ModelInput
			for _, included := range []string{fixture.instruction, candidates[0].Content, candidates[7].Content} {
				if !strings.Contains(prompt, included) {
					t.Errorf("model input omitted the necessary reduction context %q", included)
				}
			}
			for _, excluded := range []string{fixture.tail, candidates[9].Content, "CTX_1", "repository", "selected_authorities", "schema"} {
				if strings.Contains(prompt, excluded) {
					t.Errorf("model input included unrelated or code-owned %q", excluded)
				}
			}
		})
	}
}

type contextReductionEvidenceProvider struct {
	candidates []assemblyline.ContextCandidateAuthority
}

func (contextReductionEvidenceProvider) SearchAvailability(context.Context) (contextcompiler.SearchAvailability, error) {
	return contextcompiler.SearchUnavailable, nil
}

func (provider contextReductionEvidenceProvider) Retrieve(context.Context, []string) (contextcompiler.CandidateSet, error) {
	return contextcompiler.CandidateSet{Required: append([]assemblyline.ContextCandidateAuthority(nil), provider.candidates...)}, nil
}
