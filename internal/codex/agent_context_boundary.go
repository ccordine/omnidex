package codex

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/codingagent"
)

func nativeContextBoundary(frame rpcFrame, role codingagent.Role, result *codingagent.Result) error {
	if !role.IsNative() {
		return nil
	}
	var params struct {
		Error *agentNativeError `json:"error"`
		Item  struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return err
	}
	if frame.Method == "item/started" && params.Item.Type == "contextCompaction" || frame.Method == "thread/compacted" {
		return &codingagent.IncompleteTaskError{Reason: "Native context is exhausted; continue through owner-approved planning, not compacted history.", ThreadID: result.ThreadID, TurnID: result.TurnID}
	}
	if frame.Method == "error" && params.Error != nil && contextWindowExceeded(params.Error.CodexErrorInfo) {
		return &codingagent.IncompleteTaskError{Reason: "Native context window exhausted; return incomplete work to owner-approved planning.", ThreadID: result.ThreadID, TurnID: result.TurnID}
	}
	return nil
}

func contextWindowExceeded(raw json.RawMessage) bool {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name == "contextWindowExceeded" || name == "ContextWindowExceeded"
	}
	var variants map[string]json.RawMessage
	if json.Unmarshal(raw, &variants) != nil {
		return false
	}
	_, camel := variants["contextWindowExceeded"]
	_, pascal := variants["ContextWindowExceeded"]
	return camel || pascal
}
