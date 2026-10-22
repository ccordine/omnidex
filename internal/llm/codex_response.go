package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

// These are the original selected JSON-RPC frames, not an Ollama-shaped
// synthetic response. Delta/heartbeat frames are not completion evidence.
type CodexResponseCapture struct {
	Thread     json.RawMessage `json:"thread"`
	Turn       json.RawMessage `json:"turn"`
	Message    json.RawMessage `json:"message"`
	Usage      json.RawMessage `json:"usage"`
	Completion json.RawMessage `json:"completion"`
}

type codexThreadFrame struct {
	Result struct {
		Model  string `json:"model"`
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	} `json:"result"`
}

type codexTurnFrame struct {
	Result struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	} `json:"result"`
}

type codexItemFrame struct {
	Method string `json:"method"`
	Params struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		Item     struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Phase string `json:"phase"`
		} `json:"item"`
	} `json:"params"`
}

type codexUsageFrame struct {
	Method string `json:"method"`
	Params struct {
		ThreadID   string `json:"threadId"`
		TurnID     string `json:"turnId"`
		TokenUsage struct {
			Last struct {
				InputTokens           int `json:"inputTokens"`
				OutputTokens          int `json:"outputTokens"`
				ReasoningOutputTokens int `json:"reasoningOutputTokens"`
				TotalTokens           int `json:"totalTokens"`
			} `json:"last"`
		} `json:"tokenUsage"`
	} `json:"params"`
}

type codexCompletionFrame struct {
	Method string `json:"method"`
	Params struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	} `json:"params"`
}

func decodeCodexFrame(encoded []byte, value any) error {
	if !utf8.Valid(encoded) {
		return fmt.Errorf("Codex frame is not UTF-8")
	}
	if err := exactjson.ValidateCompatibleObject(encoded, value, "Codex native frame"); err != nil {
		return err
	}
	return json.Unmarshal(encoded, value)
}

func decodeCodexCapture(encoded []byte) (CodexResponseCapture, CodexContinuation, error) {
	var capture CodexResponseCapture
	var thread codexThreadFrame
	var turn codexTurnFrame

	if err := decodeCodexFrame(encoded, &capture); err != nil {
		return capture, CodexContinuation{}, err
	}
	if err := decodeCodexFrame(capture.Thread, &thread); err != nil {
		return capture, CodexContinuation{}, err
	}
	if err := decodeCodexFrame(capture.Turn, &turn); err != nil {
		return capture, CodexContinuation{}, err
	}
	continuation := CodexContinuation{ThreadID: thread.Result.Thread.ID, TurnID: turn.Result.Turn.ID, Model: thread.Result.Model}
	encodedContext, err := json.Marshal(continuation)
	if err != nil {
		return capture, continuation, err
	}
	_, err = ParseCodexContinuation(encodedContext)
	return capture, continuation, err
}

func decodeCodexPreparedResponse(status int, encoded []byte) (ExactPreparedResponse, error) {
	invalid := ExactPreparedResponse{Disposition: ProviderResponseInvalidJSON}
	if status != 0 {
		return invalid, fmt.Errorf("Codex JSON-RPC receipt claims an HTTP status")
	}
	capture, context, err := decodeCodexCapture(encoded)
	if err != nil {
		return invalid, err
	}
	var message codexItemFrame
	var usage codexUsageFrame
	var completion codexCompletionFrame
	for _, frame := range []struct {
		encoded []byte
		target  any
	}{
		{capture.Message, &message}, {capture.Usage, &usage}, {capture.Completion, &completion},
	} {
		if err := decodeCodexFrame(frame.encoded, frame.target); err != nil {
			return invalid, err
		}
	}
	if message.Method != "item/completed" || message.Params.Item.Type != "agentMessage" || message.Params.Item.Phase != "final_answer" ||
		message.Params.ThreadID != context.ThreadID || message.Params.TurnID != context.TurnID ||
		usage.Method != "thread/tokenUsage/updated" || usage.Params.ThreadID != context.ThreadID || usage.Params.TurnID != context.TurnID ||
		completion.Method != "turn/completed" || completion.Params.ThreadID != context.ThreadID || completion.Params.Turn.ID != context.TurnID ||
		completion.Params.Turn.Status != "completed" {
		return invalid, fmt.Errorf("Codex native completion does not belong to the original turn")
	}
	counts := usage.Params.TokenUsage.Last
	if counts.InputTokens < 1 || counts.OutputTokens < 1 || counts.ReasoningOutputTokens < 0 ||
		counts.ReasoningOutputTokens >= counts.OutputTokens || counts.TotalTokens != counts.InputTokens+counts.OutputTokens ||
		strings.TrimSpace(message.Params.Item.Text) == "" {
		return invalid, fmt.Errorf("Codex native usage or final output is incomplete")
	}
	return ExactPreparedResponse{Disposition: ProviderResponseSucceeded, Content: message.Params.Item.Text,
		DonePresent: true, Done: true, DoneReason: "completed", UsagePresent: true,
		Usage: ProviderGenerationUsage{PromptEvalCount: counts.InputTokens,
			EvalCount: counts.OutputTokens - counts.ReasoningOutputTokens, ReasoningEvalCount: counts.ReasoningOutputTokens}}, nil
}

func DecodeCodexRetainedContext(encoded []byte) (json.RawMessage, error) {
	if _, err := decodeCodexPreparedResponse(0, encoded); err != nil {
		return nil, err
	}
	_, continuation, err := decodeCodexCapture(encoded)
	if err != nil {
		return nil, err
	}
	return json.Marshal(continuation)
}
