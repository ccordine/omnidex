package userintent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
)

func TestActualServerSnapshotIsInterpretationEvidence(t *testing.T) {
	// The existing server DTO is supplied directly, without reconstructing it as
	// action descriptions. Caller integration is a separate frozen ledger task.
	current := sessionintent.Context{JobID: 25, Generation: 1, LedgerID: 23, Revision: 6, Planning: true, PlanState: "proposed", Items: []sessionintent.Item{{ID: "established", Text: "The accepted task"}}}
	snapshot, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	message := "  Approve this plan.\n"
	calls := 0
	result, err := userintent.Interpret(context.Background(), userintent.Input{Snapshot: snapshot, Prompt: message, Writable: func(path string) bool { return path == "/plan_state" }, Validate: func(before, after json.RawMessage) error {
		old, err := userintent.DecodeState[sessionintent.Context](before)
		if err != nil {
			return err
		}
		next, err := userintent.DecodeState[sessionintent.Context](after)
		if err != nil {
			return err
		}
		if old.JobID != next.JobID || next.PlanState != "approved" {
			return fmt.Errorf("invalid proposed state")
		}
		return nil
	}}, func(_ context.Context, prompt string) (string, error) {
		calls++
		var envelope struct {
			Snapshot json.RawMessage `json:"server_snapshot"`
			Message  string          `json:"user_message"`
		}
		marker := "Input:\n"
		at := strings.Index(prompt, marker)
		if at < 0 {
			t.Fatal("missing envelope")
		}
		if err := json.Unmarshal([]byte(prompt[at+len(marker):]), &envelope); err != nil {
			t.Fatal(err)
		}
		if string(envelope.Snapshot) != string(snapshot) || envelope.Message != message {
			t.Fatal("server snapshot or message was replaced")
		}
		return `{"patch":[{"op":"replace","path":"/plan_state","value":"approved"}]}`, nil
	})
	if err != nil || calls != 1 || len(result.Patch) != 1 || current.PlanState != "proposed" {
		t.Fatal(result, calls, err)
	}
}
