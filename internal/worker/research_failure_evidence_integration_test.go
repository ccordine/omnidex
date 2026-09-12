package worker

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestResearchFailureRecordsActualCallsWithoutRetryOrPublication(t *testing.T) {
	for _, fixture := range researchWorkflowCases() {
		for _, failure := range []struct {
			name         string
			responses    []exactEvidenceStationFixture
			kind         assemblyline.WorkKind
			acquisitions int
		}{
			{"relevance", []exactEvidenceStationFixture{{candidate: "A"}, {candidate: "invalid"}}, assemblyline.WorkWebRelevanceRelation, 0},
			{"inventory", []exactEvidenceStationFixture{{candidate: "A"}, {candidate: "A"}, {candidate: `["invalid paragraph packet"]`}}, assemblyline.WorkRoleplayGroundedResponseParagraphInventory, 2},
			{"provider", []exactEvidenceStationFixture{{partial: []byte(`{"response":"unfinished`)}}, assemblyline.WorkWebRelevanceRelation, 0},
		} {
			t.Run(fixture.name+"/"+failure.name, func(t *testing.T) {
				run := newResearchWorkflowFixture(t, fixture)
				run.provider.fixtures = failure.responses
				ctx := context.Background()
				for attempt := range 2 {
					runtime := run.runtime()
					result, err := runObjectiveRoleplayResearchTurn(ctx, run.authority, runtime.acquireObjectiveRoleplayResearch)
					wantCalls := 0
					if attempt == 0 {
						wantCalls = len(failure.responses)
					}
					if err == nil || result.Complete || result.Output != "" || len(result.Citations) != 0 || result.ModelCalls != wantCalls || run.provider.calls != len(failure.responses) {
						t.Fatalf("attempt=%d result=%+v provider=%d error=%v", attempt, result, run.provider.calls, err)
					}
				}
				calls, err := listAllWorkerLLMCallEvidence(ctx, run.repository, run.claim.Job.ID)
				if err != nil || len(calls) != len(failure.responses) {
					t.Fatalf("recorded calls=%d error=%v", len(calls), err)
				}
				last := len(calls) - 1
				for index := 0; index < last; index++ {
					assertPortableLeafRecordedCall(t, calls[index], run.provider.prepared[index], assemblyline.WorkWebRelevanceRelation, "A", "fixture-web")
				}
				recorded, response := calls[last], failure.responses[last]
				request, err := llm.ExactPreparedRequestBytes(run.provider.prepared[last])
				if err != nil {
					t.Fatal(err)
				}
				work := assemblyline.PortableJob{Schema: assemblyline.PortableJobSchemaV2, Kind: failure.kind, Payload: recorded.WorkInput}
				prompt, err := assemblyline.RenderPortableJob(work)
				if err != nil {
					t.Fatal(err)
				}
				modelName := "fixture-web"
				if failure.kind == assemblyline.WorkRoleplayGroundedResponseParagraphInventory {
					modelName = "fixture-roleplay"
				}
				if recorded.WorkKind != string(failure.kind) || recorded.Model != modelName || recorded.RequestedModel != modelName ||
					recorded.ModelInput != prompt || recorded.ModelInputBytes != len(prompt) || !bytes.Equal(recorded.ProviderRequest, request) || recorded.Outcome == nil || !recorded.RawResponsePresent {
					t.Fatal("failed call lost its actual route, input, request, or outcome")
				}
				if response.partial != nil {
					if recorded.Status != queue.LLMCallFailed || recorded.Outcome.Status != queue.LLMCallProviderFailed || !bytes.Equal(recorded.RawResponse, response.partial) {
						t.Fatal("provider failure lost its captured response or terminal outcome")
					}
				} else {
					generation, err := exactEvidenceSuccessfulGeneration(run.provider.prepared[last], response.candidate)
					if err != nil {
						t.Fatal(err)
					}
					if recorded.Candidate != response.candidate || !bytes.Equal(recorded.RawResponse, generation.ProviderResponseCapture) || recorded.Outcome.Status != queue.LLMCallRejected || strings.TrimSpace(recorded.Outcome.ValidationError) == "" {
						t.Fatal("invalid semantic output lost its raw response or rejection diagnostic")
					}
				}
				var acquisitions, publications, advances int
				if err := run.pool.QueryRow(ctx, `SELECT
					(SELECT count(*) FROM web_evidence WHERE job_id=$1),
					(SELECT count(*) FROM roleplay_research_completions WHERE job_id=$1),
					(SELECT count(*) FROM roleplay_simulation_turn_advances WHERE job_id=$1)`, run.claim.Job.ID).Scan(
					&acquisitions, &publications, &advances,
				); err != nil || acquisitions != failure.acquisitions || publications != 0 || advances != 0 || run.http.requests.Load() != 6 {
					t.Fatalf("failed research published work: acquisitions=%d publications=%d advances=%d HTTP=%d error=%v", acquisitions, publications, advances, run.http.requests.Load(), err)
				}
			})
		}
	}
}
