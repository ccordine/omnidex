package sessionintent

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/userintent"
)

func TestTypedInteractionPreservesContentAndRecipient(t *testing.T) {
	inputID := int64(9)
	for _, current := range []Context{{Items: []Item{}}, {Planning: true, Items: []Item{}}, {InputID: &inputID, Items: []Item{}}} {
		request := Request{Mode: Interact, Text: "  Don't approve anything.\nKeep /mode research literal.  "}
		result, err := DirectContent(request, current)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateRoutedPatch(request, current, result.Patch); err != nil {
			t.Fatal(err)
		}
		after, err := Apply(current, result.Patch)
		if err != nil {
			t.Fatal(err)
		}
		if after.NativeReply != nil {
			if *after.NativeReply != request.Text {
				t.Fatal("changed native reply")
			}
		} else if after.Message == nil || after.Message.Text != request.Text || after.Message.Recipient != string(InteractionRecipient(current)) {
			t.Fatal(after)
		}
		request.Recipient = ResearchConversation
		if _, err := DirectContent(request, current); err == nil {
			t.Fatal("wrong recipient accepted")
		}
	}
}

func TestTypedRoutesRejectModeAndAuthorityConfusion(t *testing.T) {
	current := Context{Pending: &Proposal{ID: "lifecycle_operation_pending", Pitch: "Remove item?"}, Items: []Item{}}
	for _, mode := range []Mode{Ledger, Research, "other"} {
		if err := (Request{Mode: mode}).ValidateRouting(current); err == nil {
			t.Fatalf("intent endpoint accepted %s", mode)
		}
	}
	if _, err := DirectContent(Request{Mode: Interact, Text: "yes"}, current); err == nil {
		t.Fatal("approval bypassed interpretation")
	}
	bad := []userintent.Patch{{Op: "replace", Path: "/open", Value: json.RawMessage(`false`)}}
	if err := ValidateRoutedPatch(Request{Mode: Interact}, current, bad); err == nil {
		t.Fatal("approval reply changed client state")
	}
	good := []userintent.Patch{{Op: "replace", Path: "/approval", Value: json.RawMessage(`true`)}}
	if err := ValidateRoutedPatch(Request{Mode: Interact}, current, good); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(current, good); err != nil {
		t.Fatal(err)
	}
}

func TestTypedInteractionRejectsChangedRecipientAndText(t *testing.T) {
	inputID := int64(9)
	contexts := []Context{{Items: []Item{}}, {Planning: true, Items: []Item{}}, {InputID: &inputID, Items: []Item{}}, {Pending: &Proposal{ID: "lifecycle_operation_pending"}, Items: []Item{}}}
	for i, before := range contexts {
		request := Request{Mode: Interact, Recipient: InteractionRecipient(before), Text: "  exact\nowner text  "}
		for j, after := range contexts {
			if i != j && request.ValidateRouting(after) == nil {
				t.Fatalf("recipient %s redirected to %s", request.Recipient, InteractionRecipient(after))
			}
		}
		if request.Recipient == Approval {
			continue
		}
		result, err := DirectContent(request, before)
		if err != nil {
			t.Fatal(err)
		}
		var changed any = Content{Text: "changed", Recipient: string(request.Recipient)}
		if request.Recipient == Native {
			changed = "changed"
		}
		result.Patch[0].Value, _ = json.Marshal(changed)
		if ValidateRoutedPatch(request, before, result.Patch) == nil {
			t.Fatal("rewritten owner text accepted", request.Recipient)
		}
	}
}
