package worker

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// This is a fixed-provider framework fixture with real HTTP and PostgreSQL,
// not a live-language-quality or autonomous-building claim.
func TestResearchWorkflowAcceptsAllFourBoundedParagraphs(t *testing.T) {
	for _, fixture := range researchWorkflowCases() {
		t.Run(fixture.name, func(t *testing.T) {
			run := newResearchWorkflowFixture(t, fixture)
			responses := []string{"A", "A", strings.Join(fixture.paragraphs, "\n")}
			kinds := []assemblyline.WorkKind{assemblyline.WorkWebRelevanceRelation, assemblyline.WorkWebRelevanceRelation, assemblyline.WorkRoleplayGroundedResponseParagraphInventory}
			for index := range fixture.paragraphs {
				attribution := []string{"A", "B"}
				if index >= 2 {
					attribution = []string{"B", "A"}
				}
				responses = append(responses, "A", "A", attribution[0], attribution[1])
				kinds = append(kinds, assemblyline.WorkGroundedParagraphRelevance, assemblyline.WorkGroundedParagraphSupport,
					assemblyline.WorkRoleplayGroundedResponseEvidenceRelation, assemblyline.WorkRoleplayGroundedResponseEvidenceRelation)
			}
			for _, response := range responses {
				run.provider.fixtures = append(run.provider.fixtures, exactEvidenceStationFixture{candidate: response})
			}
			ctx := context.Background()
			var result objectiveTurnResult
			var previousSourceID int64
			for attempt := range 2 {
				answer, err := run.runtime().acquireObjectiveRoleplayResearch(ctx, run.authority)
				wantCalls := 0
				if attempt == 0 {
					wantCalls = len(responses)
				}
				if err != nil || answer.ModelCalls != wantCalls || len(answer.Artifact.Paragraphs) != 4 ||
					len(answer.Artifact.Sources) != 2 || answer.Sources.ID == previousSourceID || run.provider.calls != len(responses) {
					t.Fatalf("attempt=%d answer=%+v provider=%d error=%v", attempt, answer, run.provider.calls, err)
				}
				previousSourceID = answer.Sources.ID
				for index, paragraph := range answer.Artifact.Paragraphs {
					if paragraph.Text != fixture.paragraphs[index] || len(paragraph.EvidenceIDs) != 1 {
						t.Fatalf("accepted paragraph changed: %+v", paragraph)
					}
				}
				stored, err := run.repository.GetWebEvidence(ctx, run.claim.Job.ID, answer.Sources.ID)
				if err != nil || !reflect.DeepEqual(stored, answer.Sources) || stored.Acquired.Discovery.Query != fixture.question {
					t.Fatalf("actual acquisition did not round-trip: %+v / %v", stored, err)
				}
				result, err = runObjectiveRoleplayResearchTurn(ctx, run.authority,
					func(context.Context, turnAuthority) (objectiveRoleplayResearchAnswer, error) { return answer, nil })
				if err != nil || !result.Complete || len(result.Citations) != 2 || result.ModelCalls != wantCalls {
					t.Fatalf("research response did not reach completion: %+v / %v", result, err)
				}
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, run.repository, run.claim.Job.ID)
			if err != nil || len(calls) != 19 || len(calls) != len(responses) || run.http.requests.Load() != 6 {
				t.Fatalf("actual calls=%d HTTP=%d error=%v", len(calls), run.http.requests.Load(), err)
			}
			for index, call := range calls {
				model := "fixture-roleplay"
				if index < 2 {
					model = "fixture-web"
				}
				assertPortableLeafRecordedCall(t, call, run.provider.prepared[index], kinds[index], responses[index], model)
				assertGroundedCallOmits(t, call.ModelInput, "/research", run.authority.RoleplayWorldID, run.authority.RoleplaySceneID, "https://source.example")
				if index != 2 {
					assertGroundedCallOmits(t, call.ModelInput, "An attentive observer.", "Calm, concise sentences.")
				}
			}
			assertResearchWorkflowCompletion(t, run, result)
		})
	}
}
