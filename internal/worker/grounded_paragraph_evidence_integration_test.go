package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
)

// These fixed-response fixtures exercise the production paragraph pipelines and
// actual PostgreSQL records, not live semantic quality or application autonomy.
func TestGroundedParagraphPipelinesRecordSeparateRelationsAndReplay(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for paragraph call-evidence coverage")
	}
	for _, mode := range []string{"repository", "roleplay"} {
		t.Run(mode, func(t *testing.T) {
			pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
			authority, err := modelconfig.Freeze(modelconfig.Config{
				"grounded_answer_model": "fixture-grounding", "conversation_response_model": "fixture-roleplay",
			})
			if err != nil {
				t.Fatal(err)
			}
			repository := queue.New(pool, authority)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise bounded paragraph evidence", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "paragraph-evidence-worker")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v err=%v", claim, err)
			}
			normalInput, paragraph := repositoryGroundedSelectionFixture()
			roleplayInput, roleplayParagraph := roleplayGroundedSelectionFixture()
			normalInput.Evidence = normalInput.Evidence[:1]
			roleplayInput.RealWorldEvidence = roleplayInput.RealWorldEvidence[:1]
			roleplayInput.RoleplayIdentity.Voice = "Brief, measured sentences."
			meaning := assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{{
				Sources: []assemblyline.ObjectiveContextSource{{Namespace: "session", CandidateID: "CTX_1"}},
				Content: "The question concerns the next scheduled occurrence.",
			}}}
			normalInput.Context, roleplayInput.Context = meaning, assemblyline.CloneObjectiveContext(meaning)
			question, evidence, unsupported := normalInput.ExactRequirement, normalInput.Evidence[0].Text, "The inspection occurs Friday."
			inventoryKind, attributionKind := assemblyline.WorkGroundedAnswerParagraphInventory, assemblyline.WorkGroundedAnswerParagraphEvidenceRelation
			expectedModel := "fixture-grounding"
			if mode == "roleplay" {
				paragraph, question, evidence = roleplayParagraph, roleplayInput.ExactQuestion, roleplayInput.RealWorldEvidence[0].Text
				unsupported = "The eclipse begins at noon."
				inventoryKind, attributionKind = assemblyline.WorkRoleplayGroundedResponseParagraphInventory, assemblyline.WorkRoleplayGroundedResponseEvidenceRelation
				expectedModel = "fixture-roleplay"
			}
			const unrelated = "The frame is blue."
			responses := []string{strings.Join([]string{paragraph, unrelated, unsupported, paragraph}, "\n"), "A", "A", "A", "B", "A", "B"}
			provider := &exactEvidenceStationClient{}
			for _, response := range responses {
				provider.fixtures = append(provider.fixtures, exactEvidenceStationFixture{candidate: response})
			}
			service := &Service{
				repo: repository, stationClient: provider, inferenceContextTokens: "8192",
				runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
			}
			runtime := &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}
			for attempt := range 2 {
				var dispatches int
				if mode == "roleplay" {
					var result assemblyline.RoleplayGroundedResponseDecision
					result, dispatches, err = (portableObjectiveRoleplayGroundedStation{runtime: runtime}).RespondGrounded(ctx, roleplayInput)
					if err == nil && (len(result.Paragraphs) != 1 || result.Paragraphs[0].Text != paragraph ||
						!reflect.DeepEqual(result.Paragraphs[0].EvidenceIDs, []string{"evidence_1"})) {
						t.Fatalf("accepted roleplay response changed: %+v", result)
					}
				} else {
					var result assemblyline.GroundedAnswerDecision
					result, dispatches, err = (&portableObjectiveRepositoryGroundingStation{runtime: runtime}).Answer(ctx, normalInput)
					if err == nil && (result.Text != paragraph || !reflect.DeepEqual(result.EvidenceIDs, []string{"evidence_1"})) {
						t.Fatalf("accepted grounded response changed: %+v", result)
					}
				}
				if err != nil {
					t.Fatalf("attempt %d: %v", attempt, err)
				}
				wantCalls := 0
				if attempt == 0 {
					wantCalls = len(responses)
				}
				if dispatches != wantCalls || provider.calls != len(responses) {
					t.Fatalf("attempt %d dispatches=%+v provider calls=%d", attempt, dispatches, provider.calls)
				}
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
			if err != nil || len(calls) != len(responses) {
				t.Fatalf("call count=%d err=%v", len(calls), err)
			}
			kinds := []assemblyline.WorkKind{
				inventoryKind, assemblyline.WorkGroundedParagraphRelevance, assemblyline.WorkGroundedParagraphSupport,
				attributionKind, assemblyline.WorkGroundedParagraphRelevance, assemblyline.WorkGroundedParagraphRelevance,
				assemblyline.WorkGroundedParagraphSupport,
			}
			paragraphs := []string{"", paragraph, paragraph, paragraph, unrelated, unsupported, unsupported}
			for index, recorded := range calls {
				assertPortableLeafRecordedCall(t, recorded, provider.prepared[index], kinds[index], responses[index], expectedModel)
				if index == 0 {
					if mode == "roleplay" {
						assertGroundedCallContains(t, recorded.ModelInput, roleplayInput.RoleplayIdentity.CharacterName, roleplayInput.RoleplayIdentity.Voice)
					}
					continue
				}
				var leaf struct {
					ParagraphText string `json:"paragraph_text"`
				}
				if err := json.Unmarshal(recorded.WorkInput, &leaf); err != nil || leaf.ParagraphText != paragraphs[index] {
					t.Fatalf("call %d has wrong paragraph: %q, %v", index, leaf.ParagraphText, err)
				}
				assertGroundedCallContains(t, recorded.ModelInput, paragraphs[index])
				assertGroundedCallOmits(t, recorded.ModelInput, "evidence_1", "CTX_1", roleplayInput.RoleplayIdentity.Voice, roleplayInput.RoleplayIdentity.Summary)
				if index >= 4 {
					assertGroundedCallOmits(t, recorded.ModelInput, paragraph)
				}
				if kinds[index] == assemblyline.WorkGroundedParagraphRelevance {
					assertGroundedCallContains(t, recorded.ModelInput, question, meaning.Capsules[0].Content)
					assertGroundedCallOmits(t, recorded.ModelInput, evidence, "factual claim")
				} else {
					assertGroundedCallContains(t, recorded.ModelInput, evidence)
					assertGroundedCallOmits(t, recorded.ModelInput, question, meaning.Capsules[0].Content)
					if kinds[index] == assemblyline.WorkGroundedParagraphSupport && mode == "roleplay" {
						assertGroundedCallContains(t, recorded.ModelInput, "real-world factual claim")
					}
				}
				t.Logf("%s: %d model-input bytes, result %q", recorded.WorkKind, recorded.ModelInputBytes, recorded.Candidate)
			}
		})
	}
}

func assertPortableLeafRecordedCall(t *testing.T, recorded queue.LLMCallEvidence, prepared llm.PreparedModel, kind assemblyline.WorkKind, response, modelName string) {
	t.Helper()
	work := assemblyline.PortableJob{Schema: assemblyline.PortableJobSchemaV2, Kind: kind, Payload: recorded.WorkInput}
	prompt, err := assemblyline.RenderPortableJob(work)
	if err != nil {
		t.Fatal(err)
	}
	request, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := exactEvidenceSuccessfulGeneration(prepared, response)
	if err != nil {
		t.Fatal(err)
	}
	if recorded.WorkKind != string(kind) || recorded.ModelInput != prompt || recorded.ModelInput != prepared.Prompt || recorded.ModelInputBytes != len(prompt) ||
		recorded.Model != modelName || recorded.RequestedModel != modelName ||
		!bytes.Equal(recorded.ProviderRequest, request) || recorded.Candidate != response ||
		!recorded.RawResponsePresent || !bytes.Equal(recorded.RawResponse, generation.ProviderResponseCapture) || recorded.Outcome == nil ||
		recorded.Outcome.Status != queue.LLMCallAccepted {
		t.Fatalf("call evidence does not match actual %s request, response, routing, and accepted semantic result", kind)
	}
}

func assertGroundedCallContains(t *testing.T, prompt string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(prompt, value) {
			t.Errorf("actual provider input omits local meaning %q", value)
		}
	}
}

func assertGroundedCallOmits(t *testing.T, prompt string, values ...string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(prompt, value) {
			t.Errorf("actual provider input exposes unrelated authority %q", value)
		}
	}
}
