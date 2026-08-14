package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectAutoWorkDegradedResponseKeepsCommittedAuthority(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeProjectAutoWorkResponse(recorder, 7, projectAutoWorkOutcome{
		Authority: projectAutoWorkAuthority{
			AutoWork:     ScrumAutoWorkConfig{Enabled: true, SourceColumns: []string{"assigned"}},
			ActiveCardID: "card-7", ActiveCardUpdatedAt: "2026-08-13T12:00:00Z",
			JobID: 19,
		},
		Message: "auto-work enabled and job queued",
	}, errors.New("board projection failed after commit"))
	if recorder.Code != http.StatusMultiStatus {
		t.Fatalf("status=%d want %d", recorder.Code, http.StatusMultiStatus)
	}
	var response projectAutoWorkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.CommitState != projectAutoWorkCommittedDegraded ||
		response.ProjectID != 7 || response.ActiveCardID != "card-7" ||
		response.JobID != 19 || !response.AutoWork.Enabled ||
		response.OperationError != "board projection failed after commit" {
		t.Fatalf("unexpected degraded authority: %+v", response)
	}
}
