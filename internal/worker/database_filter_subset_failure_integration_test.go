package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestDatabaseClosedSubsetFailureReplaysWithoutInferenceOrSQL(t *testing.T) {
	run := newDatabaseWorkflowFixture(t, databaseWorkflowCase{
		table: "signals", field: "enabled", value: "true",
		ddl: "CREATE TABLE signals (enabled boolean NOT NULL)", insert: "INSERT INTO signals VALUES (true), (false)",
	})
	responses := []string{"A", "Return the matching enabled values.", "A", "Include enabled signals.", "A", "C", "A", "Z"}
	for _, response := range responses {
		run.provider.fixtures = append(run.provider.fixtures, exactEvidenceStationFixture{candidate: response})
	}
	ctx := context.Background()
	for attempt := range 2 {
		acquisition, err := runObjectiveDatabaseEvidenceWorkflow(ctx, run.authority, "subset-failure", run.snapshot,
			portableObjectiveDatabaseStations{runtime: run.runtime()}, run.execute)
		wantCalls := 0
		if attempt == 0 {
			wantCalls = len(responses)
		}
		if err == nil || !strings.Contains(err.Error(), "unavailable") || len(acquisition.Evidence) != 0 || acquisition.ModelCalls != wantCalls {
			t.Fatalf("attempt=%d calls=%d evidence=%d error=%v", attempt, acquisition.ModelCalls, len(acquisition.Evidence), err)
		}
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, run.repository, run.claim.Job.ID)
	if err != nil || len(calls) != len(responses) || run.provider.calls != len(responses) {
		t.Fatalf("recorded=%d provider=%d error=%v", len(calls), run.provider.calls, err)
	}
	for index, call := range calls[len(calls)-2:] {
		if call.WorkKind != string(assemblyline.WorkDatabaseQueryFilterValueChoice) || call.Outcome == nil {
			t.Fatalf("choice has no recorded outcome: %+v", call)
		}
		want := queue.LLMCallAccepted
		if index == 1 {
			want = queue.LLMCallRejected
		}
		if call.Outcome.Status != want || call.Candidate != responses[len(responses)-2+index] {
			t.Fatalf("choice evidence lost its actual response/outcome: %+v", call)
		}
	}
	var queries int
	if err := run.pool.QueryRow(ctx, "SELECT count(*) FROM database_evidence WHERE job_id=$1", run.claim.Job.ID).Scan(&queries); err != nil || queries != 0 {
		t.Fatalf("rejected subset executed a query: count=%d error=%v", queries, err)
	}
}
