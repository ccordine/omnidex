package sessionintent

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestStateContractDescribesNullDTOsAndPersistedValues(t *testing.T) {
	for _, planning := range []bool{false, true} {
		c := Context{Planning: planning, PlanState: "proposed", JobStatus: model.JobStatusWaiting}
		raw, err := StateContract(c)
		if err != nil {
			t.Fatal(err)
		}
		var schema stateSchema
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		p := schema.Properties
		if !reflect.DeepEqual(p["message"].Type, []string{"object", "null"}) || !reflect.DeepEqual(p["message"].Required, []string{"text", "recipient"}) {
			t.Fatal("lost nullable Content DTO", string(raw))
		}
		want := "codex"
		if planning {
			want = "planner"
		}
		if !reflect.DeepEqual(p["message"].Properties["recipient"].Enum, []any{want}) {
			t.Fatal("recipient differs from validator")
		}
		if !reflect.DeepEqual(p["approval"].Enum, []any{nil}) {
			t.Fatal("no pending proposal permits approval")
		}
		if !p["authority"].ReadOnly || !p["items"].Items.Properties["id"].ReadOnly || !p["items"].Items.Properties["text"].ReadOnly {
			t.Fatal("protected fields exposed as editable")
		}
		item := p["items"].Items.Properties
		if !reflect.DeepEqual(item["review_state"].Enum, []any{string(model.LedgerReviewProposed), string(model.LedgerReviewAgreed), string(model.LedgerReviewNeedsDiscussion)}) || !reflect.DeepEqual(item["disposition"].Enum, []any{string(model.LedgerDispositionActive), string(model.LedgerDispositionTombstoned)}) {
			t.Fatal("persisted enums missing")
		}
	}
}

func TestStateContractPendingConfirmationAndNativeChoices(t *testing.T) {
	input := int64(8)
	raw, err := StateContract(Context{Pending: &Proposal{ID: "lifecycle_operation_pending", Pitch: "Apply stored changes?"}, InputID: &input, NativeChoices: []string{"allow once", "deny"}})
	if err != nil {
		t.Fatal(err)
	}
	var schema stateSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Properties["approval"].Enum) != 0 || !reflect.DeepEqual(schema.Properties["approval"].Type, []string{"boolean", "null"}) {
		t.Fatal("pending confirmation unavailable")
	}
	if !reflect.DeepEqual(schema.Properties["native_reply"].Enum, []any{"allow once", "deny", nil}) {
		t.Fatal("lost exact native choices")
	}
}
