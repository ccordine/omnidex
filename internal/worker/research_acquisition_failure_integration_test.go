package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestResearchAcquisitionFailureCannotPublishOrInvokeModels(t *testing.T) {
	for _, fixture := range researchWorkflowCases() {
		for _, failurePath := range []string{"/search", "/second"} {
			t.Run(fixture.name+failurePath, func(t *testing.T) {
				run := newResearchWorkflowFixture(t, fixture)
				run.http.invalidPath.Store(&failurePath)
				ctx := context.Background()
				var messagesBefore int
				if err := run.pool.QueryRow(ctx, "SELECT count(*) FROM ai_channel_messages WHERE channel_id=$1", run.authority.ChannelID).Scan(&messagesBefore); err != nil {
					t.Fatal(err)
				}
				result, err := runObjectiveRoleplayResearchTurn(ctx, run.authority, run.runtime().acquireObjectiveRoleplayResearch)
				if !errors.Is(err, websearch.ErrInvalidFetchedText) || result.Complete || result.Output != "" ||
					len(result.Citations) != 0 || result.ModelCalls != 0 || run.provider.calls != 0 {
					t.Fatalf("failed acquisition was hidden or reached a model: %+v / %v", result, err)
				}
				calls, err := listAllWorkerLLMCallEvidence(ctx, run.repository, run.claim.Job.ID)
				if err != nil || len(calls) != 0 {
					t.Fatalf("acquisition failure created semantic jobs: %+v / %v", calls, err)
				}
				var acquisitions, publications, advances, messagesAfter int
				if err := run.pool.QueryRow(ctx, `SELECT
					(SELECT count(*) FROM web_evidence WHERE job_id=$1),
					(SELECT count(*) FROM roleplay_research_completions WHERE job_id=$1),
					(SELECT count(*) FROM roleplay_simulation_turn_advances WHERE job_id=$1),
					(SELECT count(*) FROM ai_channel_messages WHERE channel_id=$2)`, run.claim.Job.ID, run.authority.ChannelID).Scan(
					&acquisitions, &publications, &advances, &messagesAfter,
				); err != nil {
					t.Fatal(err)
				}
				wantRequests := int64(1)
				if failurePath == "/second" {
					wantRequests = 3
				}
				if acquisitions != 0 || publications != 0 || advances != 0 || messagesAfter != messagesBefore || run.http.requests.Load() != wantRequests {
					t.Fatalf("failed acquisition published, advanced, or retried: sources=%d completions=%d advances=%d messages=%d/%d HTTP=%d",
						acquisitions, publications, advances, messagesAfter, messagesBefore, run.http.requests.Load())
				}
			})
		}
	}
}
