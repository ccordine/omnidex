package sessionintent

import (
	"encoding/json"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/userintent"
	"reflect"
	"testing"
)

func patch(path string, value any) userintent.Patch {
	raw, _ := json.Marshal(value)
	return userintent.Patch{Op: "replace", Path: path, Value: raw}
}
func TestStateDeltaProtectedAndExact(t *testing.T) {
	c := Context{JobID: 4, Generation: 2, LedgerID: 3, Revision: 7, Planning: true, PlanState: "proposed", JobStatus: model.JobStatusWaiting, Items: []Item{{ID: "one", Text: "First", ReviewState: model.LedgerReviewProposed, Disposition: model.LedgerDispositionActive}, {ID: "two", Text: "Second", ReviewState: model.LedgerReviewProposed, Disposition: model.LedgerDispositionActive}}}
	after, err := Apply(c, []userintent.Patch{patch("/items/0/review_state", "agreed")})
	if err != nil {
		t.Fatal(err)
	}
	expected := State(c)
	expected.Items = append([]Item{}, c.Items...)
	expected.Items[0].ReviewState = model.LedgerReviewAgreed
	if !reflect.DeepEqual(expected, after) || c.Items[0].ReviewState != model.LedgerReviewProposed {
		t.Fatal("changed outside delta")
	}
	for _, p := range []userintent.Patch{patch("/authority/job_id", 9), patch("/authority/state/workspace_identity", "foreign"), patch("/items/0/id", "foreign"), patch("/items/0/text", "rewritten"), patch("/plan_state", "frozen"), patch("/native_reply", "allow"), patch("/approval", true), patch("/message", map[string]any{"text": "hello", "recipient": "tool"}), patch("/job_status", "completed")} {
		if _, err := Apply(c, []userintent.Patch{p}); err == nil {
			t.Fatalf("accepted %s", p.Path)
		}
	}
}
func TestStateConfirmationDoesNotRequireConfirmation(t *testing.T) {
	c := Context{Pending: &Proposal{ID: "lifecycle_operation_pending", Pitch: "Cancel?"}}
	after, err := Apply(c, []userintent.Patch{patch("/approval", true)})
	if err != nil || NeedsApproval(c, after) {
		t.Fatal(after, err)
	}
}
