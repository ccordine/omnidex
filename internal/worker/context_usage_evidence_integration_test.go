package worker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
)

// Fixed responses exercise the real provider boundary and PostgreSQL state;
// they do not establish live relevance or summary quality.
func TestContextUsageRecordsActualSuccessAndFailureWithoutCallReceipts(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for context usage evidence")
	}
	for _, fixture := range []struct{ question, fact, summary string }{
		{"When is the inspection?", "The inspection takes place on Tuesday. ", "The inspection is Tuesday."},
		{"Which display language is preferred?", "The preferred display language is French. ", "Use French for display text."},
	} {
		for _, outcome := range []string{"accepted", "relevance failure", "minification failure"} {
			t.Run(fixture.question+"/"+outcome, func(t *testing.T) {
				pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
				config, err := modelconfig.Freeze(modelconfig.Config{
					"context_relevance_model": "fixture-relevance", "context_minification_model": "fixture-minification",
				})
				if err != nil {
					t.Fatal(err)
				}
				repository := queue.New(pool, config)
				ctx := context.Background()
				job, err := repository.EnqueueCodingJob(ctx, fixture.question, t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				claim, err := repository.ClaimNextStep(ctx, "context-usage-worker")
				if err != nil || claim == nil || claim.Job.ID != job.ID {
					t.Fatalf("claim=%+v error=%v", claim, err)
				}
				responses := []string{"A", "A", fixture.summary}
				if outcome == "relevance failure" {
					responses = []string{"A", "invalid"}
				} else if outcome == "minification failure" {
					responses[2] = strings.Repeat("x", assemblyline.MaxContextMinifiedBytes+1)
				}
				provider := &exactEvidenceStationClient{}
				for _, response := range responses {
					provider.fixtures = append(provider.fixtures, exactEvidenceStationFixture{candidate: response})
				}
				candidates := make([]assemblyline.ContextCandidateAuthority, 2)
				for index := range candidates {
					candidates[index], err = assemblyline.NewContextCandidateAuthority("repository", fmt.Sprintf("CTX_%d", index+1),
						strings.Repeat(fixture.fact, 40)+fmt.Sprintf("Observation %d.", index+1))
					if err != nil {
						t.Fatal(err)
					}
				}
				request := contextcompiler.Request{ExactInstruction: fixture.question, ModelInstruction: fixture.question, KnownArtifactPaths: []string{}}
				var accepted assemblyline.ObjectiveContext
				for attempt := range 2 {
					service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
					stations := portableObjectiveContextSieveStations{runtime: &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}}
					result, err := contextcompiler.Compile(ctx, request, contextUsageEvidenceProvider{candidates}, contextcompiler.Stations{Relevance: stations, Minification: stations})
					multiplier := 1 - attempt
					if result.ModelCalls != len(responses)*multiplier || provider.calls != len(responses) {
						t.Fatalf("attempt=%d actual usage was lost or repeated: %+v provider=%d / %v", attempt, result, provider.calls, err)
					}
					if outcome != "accepted" {
						if err == nil || len(result.Context.Capsules) != 0 {
							t.Fatalf("failure accepted partial context: %+v / %v", result.Context, err)
						}
						continue
					}
					if err != nil || len(result.Context.Capsules) != 1 || result.Context.Capsules[0].Content != fixture.summary || len(result.Context.Capsules[0].Sources) != 2 {
						t.Fatalf("compiled context did not consume actual values: %+v / %v", result.Context, err)
					}
					if attempt == 0 {
						accepted = result.Context
					} else if !reflect.DeepEqual(accepted, result.Context) {
						t.Fatal("persisted zero-call reuse changed accepted context")
					}
				}
				calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
				if err != nil || len(calls) != len(responses) {
					t.Fatalf("recorded calls=%d / %v", len(calls), err)
				}
				for index, recorded := range calls {
					kind, modelName := assemblyline.WorkContextRelevanceRelation, "fixture-relevance"
					if index == 2 {
						kind, modelName = assemblyline.WorkContextMinification, "fixture-minification"
					}
					if outcome == "accepted" || index < len(calls)-1 {
						assertPortableLeafRecordedCall(t, recorded, provider.prepared[index], kind, responses[index], modelName)
					} else {
						assertPortableLeafRejectedCall(t, recorded, provider.prepared[index], kind, responses[index], modelName)
					}
					assertGroundedCallContains(t, recorded.ModelInput, fixture.question)
					assertGroundedCallOmits(t, recorded.ModelInput, "CTX_1", "CTX_2", "selected_authorities", "schema", "repository")
					if index < 2 {
						assertGroundedCallContains(t, recorded.ModelInput, candidates[index].Content)
						assertGroundedCallOmits(t, recorded.ModelInput, candidates[1-index].Content)
					}
				}
			})
		}
	}
}

func assertPortableLeafRejectedCall(t *testing.T, recorded queue.LLMCallEvidence, prepared llm.PreparedModel, kind assemblyline.WorkKind, raw, modelName string) {
	t.Helper()
	prompt, err := assemblyline.RenderPortableJob(assemblyline.PortableJob{Schema: assemblyline.PortableJobSchemaV2, Kind: kind, Payload: recorded.WorkInput})
	if err != nil {
		t.Fatal(err)
	}
	request, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := exactEvidenceSuccessfulGeneration(prepared, raw)
	if err != nil {
		t.Fatal(err)
	}
	if recorded.WorkKind != string(kind) || recorded.Model != modelName || recorded.RequestedModel != modelName ||
		recorded.ModelInput != prompt || recorded.ModelInputBytes != len(prompt) || !bytes.Equal(recorded.ProviderRequest, request) ||
		recorded.Candidate != raw || !recorded.RawResponsePresent || !bytes.Equal(recorded.RawResponse, generation.ProviderResponseCapture) ||
		recorded.Outcome == nil || recorded.Outcome.Status != queue.LLMCallRejected || strings.TrimSpace(recorded.Outcome.ValidationError) == "" {
		t.Fatal("failed semantic call lost its actual request, response, model route, or rejection")
	}
}

type contextUsageEvidenceProvider struct {
	candidates []assemblyline.ContextCandidateAuthority
}

func (contextUsageEvidenceProvider) SearchAvailability(context.Context) (contextcompiler.SearchAvailability, error) {
	return contextcompiler.SearchUnavailable, nil
}

func (provider contextUsageEvidenceProvider) Retrieve(context.Context, []string) (contextcompiler.CandidateSet, error) {
	return contextcompiler.CandidateSet{Optional: append([]assemblyline.ContextCandidateAuthority(nil), provider.candidates...)}, nil
}
