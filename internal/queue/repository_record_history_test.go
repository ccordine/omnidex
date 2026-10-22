package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestHistoricalEvidenceProjectsNativeJournalWithoutRawEnvelopes(t *testing.T) {
	raw := []byte(`{"generation":1,"phase":"execution_response","role":"execute","task_id":"task-1","notes":["Implemented and checked."],"assignment":{"private":"RAW_ENVELOPE"},"frame":"UkFXX0ZSQU1F"}`)
	record, err := historicalEvidenceRecord("coding_agent", "", "", raw)
	if err != nil || record.Kind != "coding_agent" || record.Summary != "Implemented and checked." || record.Metadata["task_id"] != "task-1" {
		t.Fatalf("native projection: %#v, %v", record, err)
	}
	encoded, err := json.Marshal(record)
	if err != nil || strings.Contains(string(encoded), "RAW_") || strings.Contains(string(encoded), "assignment") || strings.Contains(string(encoded), "frame") {
		t.Fatalf("raw private journal exposed: %s, %v", encoded, err)
	}
	for _, kind := range []string{"channel_session_message", "coding_planning_amendment"} {
		receipt, err := historicalEvidenceRecord(kind, "lifecycle_operation", "operation", []byte(`{"internal":"private"}`))
		if err != nil || receipt.Kind != kind || receipt.SourceRef != "operation" {
			t.Fatal(receipt, err)
		}
	}
	public, err := historicalEvidenceRecord("command_output", "", "", []byte(`{"kind":"command_output","command":"test","summary":"passed"}`))
	if err != nil || public.Command != "test" || public.Summary != "passed" {
		t.Fatal(public, err)
	}
}

func TestNativeJournalHistoryPostgreSQLPagination(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	job, err := repo.EnqueueCodingJob(t.Context(), "Inspect this fixture", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(t.Context(), "history-fixture")
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	var ids []int64
	for _, phase := range []string{"call_opened", "call_completed"} {
		id, err := repo.CodingAgentEvidence(claim.Authority).Append(t.Context(), codingagent.Entry{
			Generation: claim.Authority.Generation, Phase: phase, Role: codingagent.Plan,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	cursor := ""
	for index, id := range ids {
		page, err := repo.ReadJobHistoryPage(t.Context(), job.ID, JobHistoryRequest{Stream: JobHistoryEvidence, Limit: 1, Cursor: cursor})
		if err != nil || len(page.Evidence) != 1 {
			t.Fatalf("page %d: %#v %v", index, page, err)
		}
		item := page.Evidence[0]
		if item.Evidence.ID != id || item.Evidence.Kind != "coding_agent" || item.Step.StepID != claim.Step.ID || item.Step.Generation != 1 {
			t.Fatalf("wrong historical identity: %#v", item)
		}
		cursor = page.NextCursor
		if (cursor != "") != (index == 0) {
			t.Fatalf("wrong cursor after page %d: %q", index, cursor)
		}
	}
}
