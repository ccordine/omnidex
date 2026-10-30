package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/codingagent"
)

func validateAgentItems(frame rpcFrame, role codingagent.Role) error {
	if agentDeltaItemKind(frame.Method) == "commandExecution" && !role.IsNative() {
		return &agentInferenceFailure{message: "task agent attempted native command execution", retryable: false}
	}
	var params struct {
		Item struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return err
	}
	if !role.IsNative() && (params.Item.Type == "commandExecution" || params.Item.Type == "fileChange") {
		return &agentInferenceFailure{message: "task agent attempted a native side effect", retryable: false}
	}
	if params.Item.Type == "dynamicToolCall" && !role.IsNative() {
		return &agentInferenceFailure{message: "agent attempted an ungranted dynamic tool", retryable: false}
	}
	if params.Item.Type == "plan" && role != codingagent.Plan && !role.IsNative() {
		return fmt.Errorf("non-planning agent emitted a native plan")
	}
	if !role.IsNative() && (params.Item.Type == "enteredReviewMode" || params.Item.Type == "exitedReviewMode") {
		return &agentInferenceFailure{message: "agent attempted repository review mode outside its task packet", retryable: false}
	}
	return nil
}

func recordAgentFrame(result *codingagent.Result, raw json.RawMessage, event func(codingagent.NativeEvent) error) error {
	owned := append(json.RawMessage(nil), raw...)
	if event != nil {
		if err := event(codingagent.NativeEvent{Frame: owned}); err != nil {
			return err
		}
		return nil
	}
	result.Evidence = append(result.Evidence, owned)
	return nil
}

func consumeAgentFrame(frame rpcFrame, raw json.RawMessage, result *codingagent.Result, role codingagent.Role, event func(codingagent.NativeEvent) error) (bool, error) {
	if err := validateAgentFrameBinding(frame, result); err != nil {
		return false, err
	}
	if err := validateAgentItems(frame, role); err != nil {
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
		return fmt.Errorf("agent event belongs to another assignment: method=%s thread=%q turn=%q expected_thread=%q expected_turn=%q", frame.Method, params.ThreadID, params.TurnID, result.ThreadID, result.TurnID)
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
			Type     string `json:"type"`
			Phase    string `json:"phase"`
			Text     string `json:"text"`
			Delivery string `json:"delivery"`
		} `json:"item"`
		Turn struct {
			ID     string            `json:"id"`
			Status string            `json:"status"`
			Error  *agentNativeError `json:"error"`
			Items  []struct {
				Type     string `json:"type"`
				Phase    string `json:"phase"`
				Text     string `json:"text"`
				Delivery string `json:"delivery"`
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
		if frame.Method == "item/completed" {
			if err := collectAgentResultItem(result, role, params.Item.Type, params.Item.Phase, params.Item.Delivery, params.Item.Text); err != nil {
				return false, err
			}
		}
	case "turn/completed":
		if params.Turn.ID == result.TurnID && params.Turn.Status == "failed" {
			return false, nativeAgentFailure(params.Turn.Error)
		}
		if params.Turn.ID == result.TurnID && params.Turn.Status == "completed" && len(result.Content) == 0 {
			for _, item := range params.Turn.Items {
				if err := collectAgentResultItem(result, role, item.Type, item.Phase, item.Delivery, item.Text); err != nil {
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

// Only the terminal response is semantic parser input. Earlier messages remain
// native evidence and activity, including asynchronous questions or blockers.
func collectAgentResultItem(result *codingagent.Result, role codingagent.Role, itemType, phase, delivery, text string) error {
	if delivery == "async" {
		return nil
	}
	var content string
	switch {
	case itemType == "agentMessage" && (phase == "final_answer" || (role == codingagent.Plan || role == codingagent.Execute || role == codingagent.Review) && phase == ""):
		content = text
	default:
		return nil
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}
	if role == codingagent.Plan || role == codingagent.Execute || role == codingagent.Review {
		encoded, err := json.Marshal(content)
		result.Content = encoded
		return err
	}
	if !json.Valid([]byte(content)) {
		return fmt.Errorf("agent returned an invalid structured result")
	}
	result.Content = json.RawMessage(content)
	return nil
}

func nativeAgentFailure(detail *agentNativeError) error {
	if detail != nil && contextWindowExceeded(detail.CodexErrorInfo) {
		return &codingagent.IncompleteTaskError{Reason: "Native context window exhausted; the task remains incomplete and requires owner-approved continuation planning."}
	}
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
