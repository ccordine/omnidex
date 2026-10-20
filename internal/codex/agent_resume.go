package codex

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Resume emits the completed prior turn's cumulative usage before new-turn
// events. It is history, not a new-turn receipt or an execution effect. Nothing
// else from a different turn or thread may bypass assignment binding.
func resumedUsageSnapshot(call codingagent.Assignment, frame rpcFrame) bool {
	if (call.Role != codingagent.Execute && call.Role != codingagent.Review) || call.Session == nil || (call.Session.State != "idle" && call.Session.State != "interrupted") || call.Session.TurnID == "" || frame.Method != "thread/tokenUsage/updated" {
		return false
	}
	var params struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
	}
	return json.Unmarshal(frame.Params, &params) == nil && params.ThreadID == call.Session.ThreadID && params.TurnID == call.Session.TurnID
}
