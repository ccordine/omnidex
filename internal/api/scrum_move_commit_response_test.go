package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScrumMoveResponseDistinguishesCommittedAndCommittedDegraded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		result     scrumCardMoveResult
		wantStatus int
		wantState  scrumCardMutationCommitState
		wantError  bool
	}{
		{
			name: "committed", result: scrumCardMoveResult{Card: ScrumCard{ID: "card-1"}},
			wantStatus: http.StatusOK, wantState: scrumCardMutationCommitted,
		},
		{
			name: "committed degraded",
			result: scrumCardMoveResult{
				Card: ScrumCard{ID: "card-1"}, PostCommitError: errors.New("lifecycle unavailable"),
			},
			wantStatus: http.StatusMultiStatus, wantState: scrumCardMutationCommittedDegraded, wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeScrumCardMoveResponse(response, test.result)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var payload scrumCardMoveResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.CommitState != test.wantState || payload.Card.ID != "card-1" {
				t.Fatalf("payload=%+v", payload)
			}
			if (strings.TrimSpace(payload.OperationError) != "") != test.wantError {
				t.Fatalf("operation_error=%q wantError=%t", payload.OperationError, test.wantError)
			}
		})
	}
}
