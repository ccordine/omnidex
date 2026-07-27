package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadDebuggerLastRunRejectsMalformedState(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{"debugger_last_run":42}`),
		json.RawMessage(`{"debugger_last_run":{"job_id":4,"project_id":0}}`),
	} {
		if _, err := loadDebuggerLastRun(raw); err == nil {
			t.Fatalf("settings %q must fail loudly", raw)
		}
	}
}

func TestProjectDebuggerSynchronizesMapBeforeEnqueue(t *testing.T) {
	source := readAPISource(t, "project_debugger.go")
	syncAt := strings.Index(source, "s.syncProjectMapByID(r.Context(), project.ID)")
	enqueueAt := strings.Index(source, "s.repo.EnqueueJob(")
	if syncAt < 0 || enqueueAt < 0 || syncAt > enqueueAt {
		t.Fatal("project debugger must synchronously refresh its map before enqueue")
	}
	for _, forbidden := range []string{
		"SyncProjectMapAsync",
		"agentResolved, _ :=",
		"if details, err := s.repo.GetJobDetails(ctx, lastRun.JobID); err == nil",
		"_ = json.Unmarshal",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("project debugger contains a stale or swallowed path %q", forbidden)
		}
	}
}
