package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestActivityDisplayHandlesBinaryAndPartialText(t *testing.T) {
	for _, fixture := range []struct{ name, input, want string }{
		{"binary command output", "before\x00after", "before\uFFFDafter"},
		{"partial multibyte output", "progress \xff\xe2\x82", "progress \uFFFD"},
		{"chunked message", strings.Repeat("é", model.MaxAgentActivityTextBytes) + "\x00done", strings.Repeat("é", model.MaxAgentActivityTextBytes) + "\uFFFDdone"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			activity := agentActivityBase(codingagent.Assignment{Role: codingagent.Plan}, &codingagent.Result{ThreadID: "thread", TurnID: "turn"}, "item", model.AgentActivityMessage, model.AgentActivityCompleted)
			activity.Text = fixture.input
			var observed strings.Builder
			var last model.AgentActivityStatus
			err := publishAgentActivityText(activity, func(part model.AgentActivity) error {
				if err := part.Validate(); err != nil {
					return err
				}
				if len(part.Text) > model.MaxAgentActivityTextBytes {
					t.Fatal("display text was not chunked")
				}
				observed.WriteString(part.Text)
				last = part.Status
				return nil
			})
			if err != nil || observed.String() != fixture.want || last != model.AgentActivityCompleted {
				t.Fatalf("display lost content or completion: %q, %s, %v", observed.String(), last, err)
			}
			if activity.Text != fixture.input {
				t.Fatal("display conversion changed retained input")
			}
		})
	}
}

func TestBinaryCommandOutputPreservesEvidenceAndCompletion(t *testing.T) {
	var frames []json.RawMessage
	var activities []model.AgentActivity
	stream := newAgentActivityStream(codingagent.Assignment{Role: codingagent.Plan}, &codingagent.Result{ThreadID: "thread", TurnID: "turn"}, func(event codingagent.NativeEvent) error {
		if len(event.Frame) > 0 {
			frames = append(frames, append(json.RawMessage(nil), event.Frame...))
		}
		if event.Activity != nil {
			activities = append(activities, *event.Activity)
		}
		return nil
	})
	inputs := []json.RawMessage{
		json.RawMessage(`{"method":"item/started","params":{"item":{"id":"command","type":"commandExecution","command":"inspect fixture","cwd":"/workspace"}}}`),
		json.RawMessage(`{"method":"item/commandExecution/outputDelta","params":{"itemId":"command","delta":"before\u0000after"}}`),
		json.RawMessage(`{"method":"item/completed","params":{"item":{"id":"command","type":"commandExecution","command":"inspect fixture","cwd":"/workspace","status":"completed","exitCode":0}}}`),
		json.RawMessage(`{"method":"item/commandExecution/outputDelta","params":{"itemId":"command","delta":"late\u0000output"}}`),
	}
	for _, frame := range inputs {
		if err := stream.observe(codingagent.NativeEvent{Frame: frame}); err != nil {
			t.Fatalf("display output interrupted execution: %v", err)
		}
	}
	if len(frames) != len(inputs) || len(activities) != 4 || len(stream.items) != 0 {
		t.Fatalf("lost evidence or command lifecycle: frames=%d activities=%d pending=%d", len(frames), len(activities), len(stream.items))
	}
	for i := range inputs {
		if !bytes.Equal(frames[i], inputs[i]) {
			t.Fatal("raw evidence changed")
		}
	}
	if activities[1].Text != "before\uFFFDafter" || activities[2].Status != model.AgentActivityCompleted || *activities[2].ExitCode != 0 {
		t.Fatalf("command output or completion lost: %+v", activities)
	}
	if activities[3].Kind != model.AgentActivityCommandOutput || activities[3].Text != "late\uFFFDoutput" {
		t.Fatalf("late command output lost: %+v", activities[3])
	}
	blocked := errors.New("evidence sink unavailable")
	stream.sink = func(codingagent.NativeEvent) error { return blocked }
	if err := stream.text("next", model.AgentActivityMessage, model.AgentActivityCompleted, "usable\x00text"); !errors.Is(err, blocked) {
		t.Fatalf("genuine publication failure was hidden: %v", err)
	}
}

func TestDisplayBookkeepingCannotRejectNativeWork(t *testing.T) {
	stream := newAgentActivityStream(codingagent.Assignment{Role: codingagent.Execute}, &codingagent.Result{ThreadID: "thread", TurnID: "turn"}, nil)
	for _, raw := range []string{
		`{"method":"item/completed","params":{"item":{"id":"unpaired","type":"commandExecution","command":"verify","cwd":"/workspace","status":"completed","exitCode":0}}}`,
		`{"method":"item/started","params":{"item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/reasoning/summaryTextDelta","params":{"itemId":"summary","summaryIndex":4,"delta":"Public progress."}}`,
		`{"method":"item/completed","params":{"item":{"id":"summary","type":"reasoning","summary":["Revised public progress."]}}}`,
	} {
		if err := stream.observe(codingagent.NativeEvent{Frame: json.RawMessage(raw)}); err != nil {
			t.Fatalf("display bookkeeping interrupted native work: %v", err)
		}
	}
}
