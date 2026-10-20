package api

import (
	"context"
	"encoding/json"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionUserIntentOneSnapshotPatch(t *testing.T) {
	current := sessionintent.Context{Planning: true, PlanState: "proposed", Revision: 12, JobID: 2, JobStatus: model.JobStatusWaiting, Items: []sessionintent.Item{{ID: "one", Text: "Keep the requested scope.", ReviewState: model.LedgerReviewProposed, Disposition: model.LedgerDispositionActive}}}
	for _, test := range []struct {
		name, message, response string
		count                   int
	}{
		{"compound", "Looks good, lock in and run.", `{"patch":[{"op":"replace","path":"/items/0/review_state","value":"agreed"},{"op":"replace","path":"/plan_state","value":"frozen"}]}`, 2},
		{"recipient", "  Tell Codex to add an approve button; don't approve the plan.\n", `{"patch":[{"op":"replace","path":"/message","value":{"text":"Add an approve button; don't approve the plan.","recipient":"planner"}}]}`, 1},
		{"clarification", "Remove that.", `{"patch":[],"clarification":"Which item should I remove?"}`, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			result, err := interpretSessionUserIntent(t.Context(), sessionintent.Request{Text: test.message}, current, func(_ context.Context, prompt string) (string, error) {
				calls++
				var envelope struct {
					Snapshot sessionintent.Snapshot `json:"server_snapshot"`
					Message  string                 `json:"user_message"`
				}
				at := strings.Index(prompt, "Input:\n")
				if at < 0 {
					t.Fatal("missing snapshot")
				}
				if err := json.Unmarshal([]byte(prompt[at+7:]), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Message != test.message || envelope.Snapshot.Authority.JobID != 2 {
					t.Fatal("changed authority/message")
				}
				if strings.Contains(prompt, "remaining meaning") || strings.Contains(prompt, "callback") {
					t.Fatal("legacy menu rendered")
				}
				return test.response, nil
			})
			if err != nil || calls != 1 || len(result.Patch) != test.count {
				t.Fatal(result, calls, err)
			}
		})
	}
}

func TestSessionUserIntentConfirmationAndNativeReply(t *testing.T) {
	id := int64(7)
	for _, test := range []struct {
		state    sessionintent.Context
		response string
	}{
		{sessionintent.Context{Pending: &sessionintent.Proposal{ID: "lifecycle_operation_pending", Pitch: "Remove the shown item?"}}, `{"patch":[{"op":"replace","path":"/approval","value":true}]}`},
		{sessionintent.Context{InputID: &id, NativeChoices: []string{"allow once", "deny"}}, `{"patch":[{"op":"replace","path":"/native_reply","value":"deny"}]}`},
	} {
		result, err := interpretSessionUserIntent(t.Context(), sessionintent.Request{Text: "yes"}, test.state, func(context.Context, string) (string, error) { return test.response, nil })
		if err != nil || len(result.Patch) != 1 {
			t.Fatal(result, err)
		}
	}
}

func TestUserIntentAPIRejectsCallerAuthoredCallbacks(t *testing.T) {
	for _, execute := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodPost, "/v1/channels/test/session/intent", strings.NewReader(`{"callback":"execute","action":"approve"}`))
		response := httptest.NewRecorder()
		server := &Server{}
		if execute {
			server.executeUserIntent(response, r, "test")
		} else {
			server.postUserIntent(response, r, "test")
		}
		if response.Code != http.StatusBadRequest {
			t.Fatalf("caller callback was not rejected before access: %d %s", response.Code, response.Body.String())
		}
	}
}
