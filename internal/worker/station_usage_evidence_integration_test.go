package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

// Fixed provider text verifies execution and value consumption, not live
// language quality or an autonomous application build.
func TestSharedStationUsageRecordsActualValuesAndRejections(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for shared station usage evidence")
	}
	for _, fixture := range []struct{ question, answer, character, contribution, action string }{
		{"When is the inspection?", "The inspection is Tuesday.", "Mira", "Mira begins crossing the bridge.", "Crossing the bridge."},
		{"Which display language is preferred?", "Use French for display text.", "Ivo", "Ivo starts kneading the dough.", "Kneading the dough."},
	} {
		for _, mode := range []string{"conversation", "ongoing action"} {
			for _, reject := range []bool{false, true} {
				name := fixture.question + "/" + mode
				if reject {
					name += "/oversized"
				}
				t.Run(name, func(t *testing.T) {
					pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
					config, err := modelconfig.Freeze(modelconfig.Config{
						"conversation_response_model": "fixture-response", "roleplay_semantic_model": "fixture-action",
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
					claim, err := repository.ClaimNextStep(ctx, "shared-station-usage-worker")
					if err != nil || claim == nil || claim.Job.ID != job.ID {
						t.Fatalf("claim=%+v / %v", claim, err)
					}
					responses := []string{fixture.answer}
					kinds := []assemblyline.WorkKind{assemblyline.WorkConversationResponse}
					modelName, expected, maximum := "fixture-response", fixture.answer, 8*1024
					if mode == "ongoing action" {
						responses = []string{"B", fixture.action}
						kinds = []assemblyline.WorkKind{assemblyline.WorkRoleplayOngoingActionRelation, assemblyline.WorkRoleplayOngoingActionValue}
						modelName, expected, maximum = "fixture-action", fixture.action, roleplay.MaxOngoingActionBytes
					}
					if reject {
						responses[len(responses)-1] = strings.Repeat("x", maximum+1)
					}
					provider := &exactEvidenceStationClient{}
					for _, raw := range responses {
						provider.fixtures = append(provider.fixtures, exactEvidenceStationFixture{candidate: raw})
					}
					for attempt := range 2 {
						service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
						runtime := &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}
						var value string
						var calls int
						var complete bool
						if mode == "conversation" {
							result, failure := runObjectiveConversationResponse(ctx, turnAuthority{ModelInstruction: fixture.question},
								objectiveTurnResult{Kind: assemblyline.ObjectiveKindAnswer}, portableObjectiveConversationStation{runtime: runtime}, "")
							value, calls, complete, err = result.Output, result.ModelCalls, result.Complete, failure
						} else {
							result, observed, failure := extractRoleplayOngoingAction(ctx, portableObjectiveRoleplayOngoingActionStation{runtime: runtime},
								assemblyline.RoleplayOngoingActionSourceAssistantResponse, fixture.character, fixture.contribution, nil)
							calls, complete, err = observed, result.Action != nil, failure
							if result.Action != nil {
								value = *result.Action
							}
						}
						if calls != len(responses)*(1-attempt) || provider.calls != len(responses) {
							t.Fatalf("attempt=%d actual calls=%d provider=%d / %v", attempt, calls, provider.calls, err)
						}
						if reject {
							if err == nil || value != "" || complete {
								t.Fatalf("rejection accepted a semantic value: %q complete=%t / %v", value, complete, err)
							}
						} else if err != nil || value != expected || !complete {
							t.Fatalf("valid result was not consumed: %q complete=%t / %v", value, complete, err)
						}
					}
					calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
					if err != nil || len(calls) != len(responses) {
						t.Fatalf("recorded calls=%d / %v", len(calls), err)
					}
					for index, recorded := range calls {
						if reject && index == len(calls)-1 {
							assertPortableLeafRejectedCall(t, recorded, provider.prepared[index], kinds[index], responses[index], modelName)
						} else {
							assertPortableLeafRecordedCall(t, recorded, provider.prepared[index], kinds[index], responses[index], modelName)
						}
						assertGroundedCallOmits(t, recorded.ModelInput, string(kinds[index]), `"schema"`, "call_evidence_id")
						if mode == "conversation" {
							assertGroundedCallContains(t, recorded.ModelInput, fixture.question)
							assertGroundedCallOmits(t, recorded.ModelInput, fixture.contribution, fixture.action)
						} else {
							assertGroundedCallContains(t, recorded.ModelInput, fixture.character, fixture.contribution)
							assertGroundedCallOmits(t, recorded.ModelInput, fixture.question, fixture.answer)
						}
					}
				})
			}
		}
	}
}
