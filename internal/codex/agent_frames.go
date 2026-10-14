package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
)

const nativeItemStateSeparator = "\x00"

func nativeItemState(kind, phase string) string {
	if phase == "" {
		return kind
	}
	return kind + nativeItemStateSeparator + phase
}

func nativeItemKind(state string) string {
	kind, _, _ := strings.Cut(state, nativeItemStateSeparator)
	return kind
}

func nativeItemPhase(state string) string {
	_, phase, _ := strings.Cut(state, nativeItemStateSeparator)
	return phase
}

func validateAgentItems(frame rpcFrame, role codingagent.Role, open map[string]string) error {
	if kind := agentDeltaItemKind(frame.Method); kind != "" {
		if kind == "plan" && role != codingagent.Plan {
			return fmt.Errorf("non-planning agent emitted a native plan")
		}
		var params struct {
			ItemID string `json:"itemId"`
		}
		if err := json.Unmarshal(frame.Params, &params); err != nil {
			return err
		}
		if params.ItemID == "" || nativeItemKind(open[params.ItemID]) != kind {
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
			Phase  string `json:"phase"`
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
	if item.Type == "plan" && role != codingagent.Plan {
		return fmt.Errorf("non-planning agent emitted a native plan")
	}
	if (item.Type == "enteredReviewMode" || item.Type == "exitedReviewMode") && role != codingagent.Review && role != codingagent.OverallReview {
		return fmt.Errorf("non-reviewing agent emitted a native review lifecycle")
	}
	if frame.Method == "item/started" {
		if _, exists := open[item.ID]; exists {
			return fmt.Errorf("native item identity reused")
		}
		if item.Type == "agentMessage" && item.Phase == "final_answer" {
			// The native server can abandon an output-limited structured final
			// answer and start its complete replacement in the same turn without
			// emitting item/completed for the superseded item. Preserve every
			// receipt, but let exactly one explicit replacement own completion.
			superseded := ""
			for id, state := range open {
				if nativeItemKind(state) != "agentMessage" || nativeItemPhase(state) != "final_answer" {
					continue
				}
				if superseded != "" {
					return fmt.Errorf("native agent left multiple final answers open")
				}
				superseded = id
			}
			delete(open, superseded)
		}
		open[item.ID] = nativeItemState(item.Type, item.Phase)
		return nil
	}
	if nativeItemKind(open[item.ID]) != item.Type {
		return fmt.Errorf("native agent completed an unbound item")
	}
	if item.Status == "inProgress" || item.Status == "in_progress" {
		return fmt.Errorf("native agent item is unfinished")
	}
	delete(open, item.ID)
	if item.Type == "exitedReviewMode" {
		answerPlaceholder := ""
		commentaryPlaceholders := make([]string, 0)
		for id, state := range open {
			if nativeItemKind(state) != "agentMessage" {
				continue
			}
			switch nativeItemPhase(state) {
			case "commentary":
				// Native Review emits complete progress messages as starts without
				// matching completions. They remain distinct from the one review
				// answer placeholder carried by exitedReviewMode.
				commentaryPlaceholders = append(commentaryPlaceholders, id)
			case "", "final_answer":
				if answerPlaceholder != "" {
					return fmt.Errorf("native review left multiple message placeholders open")
				}
				answerPlaceholder = id
			default:
				return fmt.Errorf("native review left an unfinished message phase open")
			}
		}
		for _, id := range commentaryPlaceholders {
			delete(open, id)
		}
		// Native Review starts one answer placeholder, then returns that body on
		// exitedReviewMode instead of completing the placeholder. The exited
		// receipt is its authoritative terminal event.
		delete(open, answerPlaceholder)
	}
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
	return collectAgentFrame(frame, raw, result, role)
}

func agentDeltaItemKind(method string) string {
	switch method {
	case "item/agentMessage/delta":
		return "agentMessage"
	case "item/plan/delta":
		return "plan"
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

type agentInferenceFailure struct {
	message   string
	retryable bool
}

func (failure *agentInferenceFailure) Error() string   { return failure.message }
func (failure *agentInferenceFailure) Retryable() bool { return failure.retryable }

func collectAgentFrame(frame rpcFrame, raw json.RawMessage, result *codingagent.Result, role codingagent.Role) (bool, error) {
	var params struct {
		ThreadID  string            `json:"threadId"`
		TurnID    string            `json:"turnId"`
		WillRetry *bool             `json:"willRetry"`
		Error     *agentNativeError `json:"error"`
		Item      struct {
			Type   string `json:"type"`
			Phase  string `json:"phase"`
			Text   string `json:"text"`
			Review string `json:"review"`
		} `json:"item"`
		Turn struct {
			ID     string            `json:"id"`
			Status string            `json:"status"`
			Error  *agentNativeError `json:"error"`
			Items  []struct {
				Type   string `json:"type"`
				Phase  string `json:"phase"`
				Text   string `json:"text"`
				Review string `json:"review"`
			} `json:"items"`
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
		case "userMessage", "reasoning", "agentMessage", "plan", "enteredReviewMode", "exitedReviewMode", "commandExecution", "fileChange", "dynamicToolCall":
		default:
			return false, fmt.Errorf("agent attempted an ungranted capability %s", params.Item.Type)
		}
		if frame.Method == "item/completed" {
			if err := collectAgentResultItem(result, role, params.Item.Type, params.Item.Phase, params.Item.Text, params.Item.Review); err != nil {
				return false, err
			}
		}
	case "turn/completed":
		if params.Turn.ID == result.TurnID && params.Turn.Status == "failed" {
			return false, nativeAgentFailure(params.Turn.Error)
		}
		if params.Turn.ID == result.TurnID && params.Turn.Status == "completed" && len(result.Content) == 0 {
			for _, item := range params.Turn.Items {
				if err := collectAgentResultItem(result, role, item.Type, item.Phase, item.Text, item.Review); err != nil {
					return false, err
				}
			}
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

func collectAgentResultItem(result *codingagent.Result, role codingagent.Role, itemType, phase, text, review string) error {
	var content string
	switch {
	case itemType == "agentMessage" && phase == "final_answer":
		content = text
	case itemType == "plan" && role == codingagent.Plan:
		content = text
	case itemType == "exitedReviewMode" && (role == codingagent.Review || role == codingagent.OverallReview):
		content = review
	default:
		return nil
	}
	if len(result.Content) != 0 || strings.TrimSpace(content) == "" {
		return fmt.Errorf("agent returned missing or duplicate structured result")
	}
	if json.Valid([]byte(content)) {
		result.Content = json.RawMessage(content)
		return nil
	}
	if itemType != "exitedReviewMode" {
		return fmt.Errorf("agent returned an invalid structured result")
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	result.Content = encoded
	return nil
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
	retryable := false
	if detail != nil && len(detail.CodexErrorInfo) != 0 {
		var kinds map[string]json.RawMessage
		if json.Unmarshal(detail.CodexErrorInfo, &kinds) == nil {
			for _, kind := range []string{"serverOverloaded", "httpConnectionFailed", "rateLimitExceeded"} {
				if _, present := kinds[kind]; present {
					retryable = true
				}
			}
		}
	}
	return &agentInferenceFailure{message: message, retryable: retryable}
}
