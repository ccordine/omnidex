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

func TestProjectDebuggerDoesNotPersistOrSynchronizeVersionedMaps(t *testing.T) {
	source := readAPISource(t, "project_debugger.go")
	if !strings.Contains(source, "s.repo.EnqueueJob(") {
		t.Fatal("project debugger no longer enqueues analysis")
	}
	for _, forbidden := range []string{
		"syncProjectMapByID",
		"SyncProjectMapAsync",
		"WriteCodebaseMap",
		"ReadCodebaseMap",
		"agentResolved, _ :=",
		"if details, err := s.repo.CurrentJobDetails(ctx, lastRun.JobID); err == nil",
		"_ = json.Unmarshal",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("project debugger contains a stale or swallowed path %q", forbidden)
		}
	}
}
