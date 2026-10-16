package codex

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestAgentNativeForeignActivityNeverPublished(t *testing.T) {
	for _, raw := range []string{
		`{"method":"item/started","params":{"threadId":"foreign","turnId":"turn","item":{"id":"command","type":"commandExecution","command":"cat source.go","cwd":"/project","status":"inProgress"}}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"foreign","item":{"id":"command","type":"commandExecution","status":"completed"}}}`,
	} {
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		published := 0
		_, err := consumeAgentFrame(decodeAgentTestFrame(t, raw), json.RawMessage(raw), &result, codingagent.Execute, map[string]string{"command": "commandExecution"}, func(codingagent.NativeEvent) error {
			published++
			return nil
		})
		if err == nil || published != 0 {
			t.Fatalf("foreign activity escaped before assignment validation: published=%d error=%v", published, err)
		}
	}
}

func TestAgentNativeActivityDeltasAreRetained(t *testing.T) {
	for method, kind := range map[string]string{"item/reasoning/summaryTextDelta": "reasoning", "item/agentMessage/delta": "agentMessage"} {
		raw := json.RawMessage(`{"method":"` + method + `","params":{"threadId":"thread","turnId":"turn","itemId":"item","delta":"observed output"}}`)
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		var receipt json.RawMessage
		_, err := consumeAgentFrame(decodeAgentTestFrame(t, string(raw)), raw, &result, codingagent.Execute, map[string]string{"item": kind}, func(event codingagent.NativeEvent) error {
			receipt = append(json.RawMessage(nil), event.Frame...)
			return nil
		})
		if err != nil || !bytes.Equal(receipt, raw) || len(result.Evidence) != 1 {
			t.Fatalf("streamed activity lost its original receipt: method=%s evidence=%d error=%v", method, len(result.Evidence), err)
		}
	}
}

func collectNativeActivities(t *testing.T, call codingagent.Assignment, frames []string) ([]model.AgentActivity, []json.RawMessage) {
	t.Helper()
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	var activities []model.AgentActivity
	var receipts []json.RawMessage
	stream := newAgentActivityStream(call, &result, func(event codingagent.NativeEvent) error {
		if len(event.Frame) != 0 {
			receipts = append(receipts, append(json.RawMessage(nil), event.Frame...))
		}
		if event.Activity != nil {
			activities = append(activities, *event.Activity)
		}
		return nil
	})
	items := map[string]string{}
	for _, raw := range frames {
		if _, err := consumeAgentFrame(decodeAgentTestFrame(t, raw), json.RawMessage(raw), &result, call.Role, items, stream.observe); err != nil {
			t.Fatal(err)
		}
	}
	return activities, receipts
}

func TestAgentNativeProjectsPublicSummaryAndCommentaryWithoutPrivateOrFinalOutput(t *testing.T) {
	frames := []string{
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/reasoning/textDelta","params":{"threadId":"thread","turnId":"turn","itemId":"summary","delta":"PRIVATE_REASONING"}}`,
		`{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"thread","turnId":"turn","itemId":"summary","summaryIndex":0,"delta":"Checking the boundary."}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning","summary":["Checking the boundary."],"content":["PRIVATE_REASONING"]}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"message","type":"agentMessage","phase":"commentary"}}}`,
		`{"method":"item/agentMessage/delta","params":{"threadId":"thread","turnId":"turn","itemId":"message","delta":"The command is starting."}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"message","type":"agentMessage","phase":"commentary","text":"The command is starting."}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"final","type":"agentMessage","phase":"final_answer"}}}`,
		`{"method":"item/agentMessage/delta","params":{"threadId":"thread","turnId":"turn","itemId":"final","delta":"{\"questions\":[]}"}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"final","type":"agentMessage","phase":"final_answer","text":"{\"questions\":[]}"}}}`,
	}
	activities, receipts := collectNativeActivities(t, codingagent.Assignment{Role: codingagent.Plan}, frames)
	if len(activities) != 4 || len(receipts) != len(frames)-1 {
		t.Fatalf("public stream lost its exact projections or receipts: activities=%+v receipts=%d", activities, len(receipts))
	}
	var public string
	for _, activity := range activities {
		public += activity.Text
	}
	if public != "Checking the boundary.The command is starting." || strings.Contains(public, "PRIVATE") || strings.Contains(public, "questions") {
		t.Fatalf("native private or internal result content escaped: %q", public)
	}
}

func TestAgentNativeUnknownMessagePhaseWaitsUntilPublicCompletion(t *testing.T) {
	frames := []string{
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"message","type":"agentMessage"}}}`,
		`{"method":"item/agentMessage/delta","params":{"threadId":"thread","turnId":"turn","itemId":"message","delta":"unclassified text"}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"message","type":"agentMessage","phase":"commentary","text":"Public message."}}}`,
	}
	activities, _ := collectNativeActivities(t, codingagent.Assignment{Role: codingagent.Plan}, frames)
	if len(activities) != 1 || activities[0].Text != "Public message." || activities[0].Status != model.AgentActivityCompleted {
		t.Fatalf("unclassified message leaked or public completion was lost: %+v", activities)
	}
}

func TestAgentNativePublicCompletionRetainsUnstreamedRemainder(t *testing.T) {
	frames := []string{
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"message","type":"agentMessage","phase":"commentary"}}}`,
		`{"method":"item/agentMessage/delta","params":{"threadId":"thread","turnId":"turn","itemId":"message","delta":"First "}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"message","type":"agentMessage","phase":"commentary","text":"First complete."}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"thread","turnId":"turn","itemId":"summary","summaryIndex":0,"delta":"Public "}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning","summary":["Public details."]}}}`,
	}
	activities, _ := collectNativeActivities(t, codingagent.Assignment{Role: codingagent.Plan}, frames)
	var observed string
	for _, activity := range activities {
		observed += activity.Text
	}
	if observed != "First complete.Public details." {
		t.Fatalf("authoritative public completion lost its unstreamed remainder: %q", observed)
	}
}

func TestAgentNativeActivityBindsDeltaThatArrivesImmediatelyBeforeStart(t *testing.T) {
	frames := []string{
		`{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"thread","turnId":"turn","itemId":"summary","summaryIndex":0,"delta":"Observed first."}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning","summary":["Observed first."]}}}`,
	}
	activities, receipts := collectNativeActivities(t, codingagent.Assignment{Role: codingagent.Plan}, frames)
	if len(receipts) != 3 || len(activities) != 2 || activities[0].Text != "Observed first." || activities[1].Status != model.AgentActivityCompleted {
		t.Fatalf("delta-before-start lifecycle was not reconciled: activities=%+v receipts=%d", activities, len(receipts))
	}
}

func TestAgentNativeActivityRejectsProvisionalStartWithDifferentKind(t *testing.T) {
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	items := map[string]string{}
	delta := `{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"thread","turnId":"turn","itemId":"item","summaryIndex":0,"delta":"Observed."}}`
	if _, err := consumeAgentFrame(decodeAgentTestFrame(t, delta), json.RawMessage(delta), &result, codingagent.Execute, items, nil); err != nil {
		t.Fatal(err)
	}
	start := `{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"item","type":"agentMessage","phase":"commentary"}}}`
	if _, err := consumeAgentFrame(decodeAgentTestFrame(t, start), json.RawMessage(start), &result, codingagent.Execute, items, nil); err == nil {
		t.Fatal("provisional item accepted a contradictory start kind")
	}
}
