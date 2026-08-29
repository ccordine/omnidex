package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScrumAutoWorkMutationResponseReportsExactCommitBoundary(t *testing.T) {
	t.Parallel()
	config := ScrumAutoWorkConfig{Enabled: true, SourceColumns: []string{"assigned"}}

	for _, test := range []struct {
		name      string
		failure   error
		status    int
		state     scrumAutoWorkCommitState
		errorText string
	}{
		{name: "complete", status: http.StatusOK, state: scrumAutoWorkCommitted},
		{
			name: "post-commit reconciliation failure", failure: errors.New("queue reconciliation unavailable"),
			status: http.StatusMultiStatus, state: scrumAutoWorkCommittedDegraded,
			errorText: "queue reconciliation unavailable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeScrumAutoWorkMutationResponse(response, 14, config, test.failure)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			var decoded scrumAutoWorkMutationResponse
			if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.CommitState != test.state || decoded.OperationError != test.errorText ||
				!decoded.AutoWork.Enabled || len(decoded.AutoWork.SourceColumns) != 1 {
				t.Fatalf("decoded=%+v", decoded)
			}
		})
	}
}
