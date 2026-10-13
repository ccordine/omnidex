package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestAgentNativeCommandStreamOnlyEmptyAggregateRetainsOutputAndCompletion(t *testing.T) {
	frames := []string{
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution","command":"printf result","cwd":"/project","status":"inProgress"}}}`,
		`{"method":"item/commandExecution/outputDelta","params":{"threadId":"thread","turnId":"turn","itemId":"command","delta":"streamed result\n"}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution","command":"printf result","cwd":"/project","status":"completed","aggregatedOutput":"","exitCode":0}}}`,
	}
	activities, receipts := collectNativeActivities(t, codingagent.Assignment{Role: codingagent.OverallReview}, frames)
	if len(activities) != 3 || activities[1].Text != "streamed result\n" || activities[2].Kind != model.AgentActivityCommand || activities[2].Status != model.AgentActivityCompleted || activities[2].ExitCode == nil || *activities[2].ExitCode != 0 {
		t.Fatalf("stream-only command lost validated output or terminal status: %+v", activities)
	}
	if len(receipts) != len(frames) {
		t.Fatal("stream-only command lost its original native receipts")
	}
}

func TestAgentNativePublicSummaryUsesOpenedSectionIndex(t *testing.T) {
	frames := []string{
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/reasoning/summaryPartAdded","params":{"threadId":"thread","turnId":"turn","itemId":"summary","summaryIndex":0}}`,
		`{"method":"item/reasoning/summaryPartAdded","params":{"threadId":"thread","turnId":"turn","itemId":"summary","summaryIndex":1}}`,
		`{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"thread","turnId":"turn","itemId":"summary","summaryIndex":1,"delta":"Second "}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning","summary":["","Second complete."],"content":["PRIVATE_REASONING"]}}}`,
	}
	activities, receipts := collectNativeActivities(t, codingagent.Assignment{Role: codingagent.OverallReview}, frames)
	var public strings.Builder
	for _, activity := range activities {
		if activity.Kind != model.AgentActivityReasoningSummary {
			t.Fatalf("indexed summary changed its public kind: %+v", activity)
		}
		public.WriteString(activity.Text)
	}
	if public.String() != "\nSecond complete." || len(activities) == 0 || activities[len(activities)-1].Status != model.AgentActivityCompleted {
		t.Fatalf("indexed summary lost opened empty section, suffix or completion: %q", public.String())
	}
	if len(receipts) != len(frames) {
		t.Fatal("indexed summary lost its original native receipts")
	}
}

func TestAgentNativeCompletionRejectsNonemptyContradictionWithBoundedDiagnostic(t *testing.T) {
	for _, kind := range []model.AgentActivityKind{model.AgentActivityMessage, model.AgentActivityReasoningSummary} {
		t.Run(string(kind), func(t *testing.T) {
			published := 0
			stream := newAgentActivityStream(codingagent.Assignment{Role: codingagent.OverallReview}, &codingagent.Result{ThreadID: "thread", TurnID: "turn"}, func(codingagent.NativeEvent) error {
				published++
				return nil
			})
			sent, final := "PRIVATE_SENT_VALUE", "PRIVATE_FINAL_VALUE"
			err := stream.completeText("public-item", kind, model.AgentActivityCompleted, sent, final)
			if err == nil || published != 0 {
				t.Fatalf("nonempty public contradiction escaped validation: published=%d error=%v", published, err)
			}
			for _, field := range []string{"kind=" + string(kind), `item="public-item"`, fmt.Sprintf("sentBytes=%d", len(sent)), fmt.Sprintf("finalBytes=%d", len(final))} {
				if !strings.Contains(err.Error(), field) {
					t.Fatalf("completion diagnostic lacks bounded correlation %q: %v", field, err)
				}
			}
			if len(err.Error()) > 512 || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("completion diagnostic exposed content or exceeded its bound: %v", err)
			}
		})
	}
}

func TestAgentNativeCommandProjectionKeepsLiveOutputOverLossyAggregate(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		deltas    []string
		aggregate string
	}{
		{name: "stdout stderr reordered", deltas: []string{"stderr\n", "stdout\n"}, aggregate: "stdout\nstderr\n"},
		{name: "retained aggregate capped", deltas: []string{"full ", "streamed output\n"}, aggregate: "full "},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			frames := []string{`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution","command":"fixture","cwd":"/project","status":"inProgress"}}}`}
			for _, delta := range fixture.deltas {
				raw, err := json.Marshal(map[string]any{"method": "item/commandExecution/outputDelta", "params": map[string]any{"threadId": "thread", "turnId": "turn", "itemId": "command", "delta": delta}})
				if err != nil {
					t.Fatal(err)
				}
				frames = append(frames, string(raw))
			}
			raw, err := json.Marshal(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "thread", "turnId": "turn", "item": map[string]any{"id": "command", "type": "commandExecution", "command": "fixture", "cwd": "/project", "status": "completed", "aggregatedOutput": fixture.aggregate, "exitCode": 0}}})
			if err != nil {
				t.Fatal(err)
			}
			frames = append(frames, string(raw))
			activities, receipts := collectNativeActivities(t, codingagent.Assignment{Role: codingagent.OverallReview}, frames)
			var observed strings.Builder
			for _, activity := range activities {
				if activity.Kind == model.AgentActivityCommandOutput {
					observed.WriteString(activity.Text)
				}
			}
			last := activities[len(activities)-1]
			if observed.String() != strings.Join(fixture.deltas, "") || last.Kind != model.AgentActivityCommand || last.Status != model.AgentActivityCompleted || last.ExitCode == nil || *last.ExitCode != 0 {
				t.Fatalf("lossy aggregate changed original live output or completion: output=%q terminal=%+v", observed.String(), last)
			}
			if len(receipts) != len(frames) {
				t.Fatal("lossy aggregate discarded its original native receipt")
			}
		})
	}
}

func TestAgentNativeCommandCompletionProjectsAggregateWithoutDeltas(t *testing.T) {
	frames := []string{
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution","command":"fixture","cwd":"/project","status":"inProgress"}}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution","command":"fixture","cwd":"/project","status":"completed","aggregatedOutput":"completed output\n","exitCode":0}}}`,
	}
	activities, _ := collectNativeActivities(t, codingagent.Assignment{Role: codingagent.OverallReview}, frames)
	if len(activities) != 3 || activities[1].Kind != model.AgentActivityCommandOutput || activities[1].Text != "completed output\n" || activities[2].Status != model.AgentActivityCompleted {
		t.Fatalf("unstreamed native command lost its available aggregate or completion: %+v", activities)
	}
}
