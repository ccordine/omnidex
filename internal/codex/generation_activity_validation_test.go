package codex

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestSemanticPublicActivityRejectsForeignMissingAndUnboundItemsBeforePublication(t *testing.T) {
	for _, raw := range []string{
		`{"method":"item/started","params":{"threadId":"foreign","turnId":"turn","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"foreign","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/started","params":{"turnId":"turn","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/started","params":{"threadId":"thread","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"type":"reasoning"}}}`,
		`{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"thread","turnId":"turn","itemId":"unstarted","delta":"unbound public text"}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"unstarted","type":"reasoning","summary":["unbound public text"]}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution","command":"cat private.txt","cwd":"/project"}}}`,
	} {
		published := 0
		stream := newSemanticActivityStream("thread", "turn", func(model.AgentActivity) error { published++; return nil })
		frame := decodeAgentTestFrame(t, raw)
		var capture llm.CodexResponseCapture
		_, err := collectGenerationFrame(frame, json.RawMessage(raw), &capture, "thread", "turn")
		if err == nil {
			err = stream.consume(frame, json.RawMessage(raw))
		}
		if err == nil || published != 0 {
			t.Fatalf("foreign, unbound or effectful semantic activity escaped: published=%d error=%v frame=%s", published, err, raw)
		}
	}
}

func TestSemanticPublicActivityRejectsWrongDeltaKindAndIncompleteTurn(t *testing.T) {
	for _, invalid := range []string{
		`{"method":"item/agentMessage/delta","params":{"threadId":"thread","turnId":"turn","itemId":"summary","delta":"wrong item kind"}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`,
	} {
		published := 0
		stream := newSemanticActivityStream("thread", "turn", func(model.AgentActivity) error { published++; return nil })
		start := `{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"summary","type":"reasoning"}}}`
		if err := stream.consume(decodeAgentTestFrame(t, start), json.RawMessage(start)); err != nil {
			t.Fatal(err)
		}
		if err := stream.consume(decodeAgentTestFrame(t, invalid), json.RawMessage(invalid)); err == nil || published != 0 {
			t.Fatalf("invalid semantic item lifecycle escaped: published=%d error=%v", published, err)
		}
	}
}
