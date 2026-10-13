package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
)

func validateAgentItems(frame rpcFrame, role codingagent.Role, open map[string]string) error {
	if kind := agentDeltaItemKind(frame.Method); kind != "" {
		var params struct {
			ItemID string `json:"itemId"`
		}
		if err := json.Unmarshal(frame.Params, &params); err != nil {
			return err
		}
		if params.ItemID == "" || open[params.ItemID] != kind {
			return fmt.Errorf("native agent delta belongs to an unbound item")
		}
		return nil
	}
	if frame.Method != "item/started" && frame.Method != "item/completed" {
		return nil
	}
	var params struct {
		Item struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"item"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return err
	}
	item := params.Item
	if item.ID == "" {
		return fmt.Errorf("native agent item lacks identity")
	}
	if (item.Type == "fileChange" || item.Type == "dynamicToolCall") && role != codingagent.Execute {
		return fmt.Errorf("read-only agent attempted a source edit")
	}
	if frame.Method == "item/started" {
		if _, exists := open[item.ID]; exists {
			return fmt.Errorf("native item identity reused")
		}
		open[item.ID] = item.Type
		return nil
	}
	if open[item.ID] != item.Type {
		return fmt.Errorf("native agent completed an unbound item")
	}
	if item.Status == "inProgress" || item.Status == "in_progress" {
		return fmt.Errorf("native agent item is unfinished")
	}
	delete(open, item.ID)
	return nil
}

func recordAgentFrame(result *codingagent.Result, raw json.RawMessage, event func(codingagent.NativeEvent) error) error {
	count := len(raw)
	for _, frame := range result.Evidence {
		count += len(frame)
	}
	if count > llm.MaxExactPreparedProviderResponseBytes {
		return fmt.Errorf("native agent evidence exceeds its transport boundary")
	}
	owned := append(json.RawMessage(nil), raw...)
	if event != nil {
		if err := event(codingagent.NativeEvent{Frame: owned}); err != nil {
			return err
		}
	}
	result.Evidence = append(result.Evidence, owned)
	return nil
}

func consumeAgentFrame(frame rpcFrame, raw json.RawMessage, result *codingagent.Result, role codingagent.Role, items map[string]string, event func(codingagent.NativeEvent) error) (bool, error) {
	if err := validateAgentFrameBinding(frame, result); err != nil {
		return false, err
	}
	if err := validateAgentItems(frame, role, items); err != nil {
		return false, err
	}
	if agentEvidenceMethod(frame.Method) {
		if err := recordAgentFrame(result, raw, event); err != nil {
			return false, err
		}
	}
	return collectAgentFrame(frame, raw, result)
}

func agentDeltaItemKind(method string) string {
	switch method {
	case "item/agentMessage/delta":
		return "agentMessage"
	case "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded":
		return "reasoning"
	case "item/commandExecution/outputDelta":
		return "commandExecution"
	default:
		return ""
	}
}

func validateAgentFrameBinding(frame rpcFrame, result *codingagent.Result) error {
	var params struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return err
	}
	if (params.ThreadID != "" && params.ThreadID != result.ThreadID) || (params.TurnID != "" && params.TurnID != result.TurnID) {
		return fmt.Errorf("agent event belongs to another assignment")
	}
	if frame.Method == "item/started" || frame.Method == "item/completed" || agentDeltaItemKind(frame.Method) != "" {
		if params.ThreadID != result.ThreadID || params.TurnID != result.TurnID || result.ThreadID == "" || result.TurnID == "" {
			return fmt.Errorf("native activity lacks its exact thread and turn")
		}
	}
	return nil
}

type agentNativeError struct {
	Message           string          `json:"message"`
	AdditionalDetails *string         `json:"additionalDetails"`
	CodexErrorInfo    json.RawMessage `json:"codexErrorInfo"`
}

func collectAgentFrame(frame rpcFrame, raw json.RawMessage, result *codingagent.Result) (bool, error) {
	var params struct {
		ThreadID  string            `json:"threadId"`
		TurnID    string            `json:"turnId"`
		WillRetry *bool             `json:"willRetry"`
		Error     *agentNativeError `json:"error"`
		Item      struct {
			Type  string `json:"type"`
			Phase string `json:"phase"`
			Text  string `json:"text"`
		} `json:"item"`
		Turn struct {
			ID     string            `json:"id"`
			Status string            `json:"status"`
			Error  *agentNativeError `json:"error"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return false, err
	}
	if params.ThreadID != "" && params.ThreadID != result.ThreadID || params.TurnID != "" && params.TurnID != result.TurnID {
		return false, fmt.Errorf("agent event belongs to another assignment")
	}
	switch frame.Method {
	case "item/started", "item/completed":
		switch params.Item.Type {
		case "userMessage", "reasoning", "agentMessage", "commandExecution", "fileChange", "dynamicToolCall":
		default:
			return false, fmt.Errorf("agent attempted an ungranted capability %s", params.Item.Type)
		}
		if frame.Method == "item/completed" && params.Item.Type == "agentMessage" && params.Item.Phase == "final_answer" {
			if len(result.Content) != 0 || !json.Valid([]byte(params.Item.Text)) {
				return false, fmt.Errorf("agent returned missing, duplicate or invalid structured result")
			}
			result.Content = json.RawMessage(params.Item.Text)
		}
	case "turn/completed":
		if params.Turn.ID == result.TurnID && params.Turn.Status == "failed" {
			return false, nativeAgentFailure(params.Turn.Error)
		}
		if params.Turn.ID != result.TurnID || params.Turn.Status != "completed" || len(result.Content) == 0 {
			return false, fmt.Errorf("agent did not complete the exact assignment")
		}
		result.Completed = true
		return true, nil
	case "error":
		if params.ThreadID != result.ThreadID || params.TurnID != result.TurnID || params.WillRetry == nil || params.Error == nil || strings.TrimSpace(params.Error.Message) == "" {
			return false, fmt.Errorf("native agent error lacks its exact turn, retry status or diagnostic")
		}
		if *params.WillRetry {
			return false, nil
		}
		return false, nativeAgentFailure(params.Error)
	}
	return false, nil
}

func nativeAgentFailure(detail *agentNativeError) error {
	message := "native agent reported an inference failure"
	if detail != nil {
		if text := strings.TrimSpace(detail.Message); text != "" {
			message += ": " + text
		}
		if detail.AdditionalDetails != nil && strings.TrimSpace(*detail.AdditionalDetails) != "" {
			message += "; " + strings.TrimSpace(*detail.AdditionalDetails)
		}
		if len(detail.CodexErrorInfo) != 0 && string(detail.CodexErrorInfo) != "null" {
			message += "; codexErrorInfo=" + string(detail.CodexErrorInfo)
		}
	}
	message = strings.ReplaceAll(strings.ToValidUTF8(message, "\uFFFD"), "\x00", "\uFFFD")
	const limit = 8192 // Same diagnostic boundary as exact-station evidence.
	if len(message) > limit {
		message = message[:limit-len("...[truncated]")]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
		message += "...[truncated]"
	}
	return fmt.Errorf("%s", message)
}
