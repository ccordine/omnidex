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
		var activity *model.AgentActivity
		receipts := 0
		stream := newAgentActivityStream(scope, &result, func(event codingagent.NativeEvent) error {
			if len(event.Frame) != 0 {
				receipts++
			}
			if event.Activity != nil {
				copied := *event.Activity
				activity = &copied
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
			if activity != nil {
				t.Fatal("rejected write became a published edit")
			}
			continue
		}
		if activity == nil || activity.Kind != model.AgentActivityFileEdit || activity.Status != model.AgentActivityCompleted || len(activity.Paths) != 1 || activity.Paths[0] != "source.go" || activity.Text != "" {
			t.Fatalf("confirmed write lost its public path-only projection: %+v", activity)
		}
	}
}
