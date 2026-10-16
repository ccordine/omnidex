package codex

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestAgentNativeFileEditProjectsOnlyAConfirmedScopedWrite(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		scope := agentTestScope(t)
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		var activities []model.AgentActivity
		receipts := 0
		stream := newAgentActivityStream(scope, &result, func(event codingagent.NativeEvent) error {
			if len(event.Frame) != 0 {
				receipts++
			}
			if event.Activity != nil {
				activities = append(activities, *event.Activity)
			}
			return nil
		})
		raw := json.RawMessage(`{"id":1,"method":"item/tool/call","params":{"threadId":"thread","turnId":"turn","callId":"edit","tool":"omnidex_write_file","arguments":{"path":"source.go","content":"private source bytes","expected_content":"fixture\n"}}}`)
		connection := &connection{input: &agentTestToolInput{}, lastFrame: raw}
		writer := agentTestFileWriter(func(context.Context, codingagent.FileWrite) error {
			if rejected {
				return errors.New("scoped write rejected")
			}
			return nil
		})
		if err := connection.handleAgentTool(context.Background(), decodeAgentTestFrame(t, string(raw)), &result, scope, writer, stream.observe); err != nil {
			t.Fatal(err)
		}
		if receipts != 2 {
			t.Fatal("write request and result lost original durable receipts")
		}
		if rejected {
			if len(activities) != 2 || activities[0].Kind != model.AgentActivityCommand ||
				activities[0].Status != model.AgentActivityStarted || activities[1].Kind != model.AgentActivityCommand ||
				activities[1].Status != model.AgentActivityFailed {
				t.Fatalf("rejected write lost its exact tool lifecycle: %+v", activities)
			}
			continue
		}
		if len(activities) != 3 || activities[0].Kind != model.AgentActivityCommand ||
			activities[0].Status != model.AgentActivityStarted || activities[0].Command != "omnidex_write_file" ||
			activities[1].Kind != model.AgentActivityCommand || activities[1].Status != model.AgentActivityCompleted ||
			activities[2].Kind != model.AgentActivityFileEdit || activities[2].Status != model.AgentActivityCompleted ||
			len(activities[2].Paths) != 1 || activities[2].Paths[0] != "source.go" || activities[2].Text != "" {
			t.Fatalf("confirmed write lost its tool and path-only lifecycle: %+v", activities)
		}
	}
}
