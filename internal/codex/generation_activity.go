package codex

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

// Semantic calls retain their original response capture. This task-local
// observer reuses the native public item projection without receiving raw
// receipts, station output, prompts or execution capabilities.
type semanticActivityStream struct {
	identity codingagent.Result
	public   *agentActivityStream
}

func newSemanticActivityStream(threadID, turnID string, observer func(model.AgentActivity) error) *semanticActivityStream {
	stream := &semanticActivityStream{identity: codingagent.Result{ThreadID: threadID, TurnID: turnID}}
	if observer == nil {
		return stream
	}
	stream.public = newAgentActivityStream(codingagent.Assignment{Role: codingagent.Plan}, &stream.identity, func(event codingagent.NativeEvent) error {
		if event.Activity != nil {
			return observer(*event.Activity)
		}
		return nil
	})
	return stream
}

func (stream *semanticActivityStream) consume(frame rpcFrame, raw json.RawMessage) error {
	if stream.public == nil {
		return nil
	}
	if err := validateAgentFrameBinding(frame, &stream.identity); err != nil {
		return err
	}
	if err := validateAgentItems(frame, codingagent.Plan); err != nil {
		return err
	}
	return stream.public.observe(codingagent.NativeEvent{Frame: raw})
}
