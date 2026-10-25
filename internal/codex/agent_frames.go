package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/codingagent"
)

const nativeItemStateSeparator = "\x00"
const nativeItemProvisionalPhase = "__delta_before_start__"

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
	return validateAgentItemsMode(frame, role, open, false)
}

func validateAgentItemsMode(frame rpcFrame, role codingagent.Role, open map[string]string, allowProvisional bool) error {
	return validateAgentItemsTracked(frame, role, open, nil, allowProvisional)
}

func validateAgentItemsTracked(frame rpcFrame, role codingagent.Role, open, closed map[string]string, allowProvisional bool) error {
	if frame.Method == "turn/completed" {
		for id := range closed {
			delete(closed, id)
		}
		return nil
	}
	if kind := agentDeltaItemKind(frame.Method); kind != "" {
		if kind == "commandExecution" && !role.IsNative() {
			return &agentInferenceFailure{message: "task agent attempted native command execution", retryable: false}
		}
		if kind == "plan" && !role.IsNative() {
			return fmt.Errorf("non-planning agent emitted a native plan")
		}
		var params struct {
			ItemID string `json:"itemId"`
		}
		if err := json.Unmarshal(frame.Params, &params); err != nil {
			return err
		}
		if params.ItemID == "" {
			return fmt.Errorf("native agent delta lacks its item identity")
		}
		state, exists := open[params.ItemID]
		if !exists {
			if !allowProvisional {
				return fmt.Errorf("native agent delta belongs to an unbound item")
			}
			// Codex app-server can emit the first delta immediately before its
			// item/started notification. Bind that delta provisionally to this
			// already-validated thread and turn; the later start must confirm the
			// same kind, and an unfinished provisional item still fails the turn.
			open[params.ItemID] = nativeItemState(kind, nativeItemProvisionalPhase)
			return nil
		}
		if nativeItemKind(state) != kind {
			return fmt.Errorf("native agent delta kind differs from its bound item")
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
	if !role.IsNative() && (item.Type == "commandExecution" || item.Type == "fileChange") {
		return &agentInferenceFailure{message: "task agent attempted a native side effect", retryable: false}
	}
	if item.Type == "dynamicToolCall" && !role.IsNative() {
		return &agentInferenceFailure{message: "agent attempted an ungranted dynamic tool", retryable: false}
	}
	if item.Type == "plan" && role != codingagent.Plan && !role.IsNative() {
		return fmt.Errorf("non-planning agent emitted a native plan")
	}
	if !role.IsNative() && (item.Type == "enteredReviewMode" || item.Type == "exitedReviewMode") {
		return &agentInferenceFailure{message: "agent attempted repository review mode outside its task packet", retryable: false}
	}
	if frame.Method == "item/started" {
		if _, exists := closed[item.ID]; exists {
			return fmt.Errorf("native item identity reused")
		}
		if state, exists := open[item.ID]; exists {
			if nativeItemPhase(state) != nativeItemProvisionalPhase || nativeItemKind(state) != item.Type {
				return fmt.Errorf("native item identity reused")
			}
			open[item.ID] = nativeItemState(item.Type, item.Phase)
			return nil
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
	state, exists := open[item.ID]
	if !exists || nativeItemKind(state) != item.Type {
		return fmt.Errorf("native agent completed an unbound item")
	}
	if item.Status == "inProgress" || item.Status == "in_progress" {
		return fmt.Errorf("native agent item is unfinished")
	}
	delete(open, item.ID)
	if closed != nil {
		closed[item.ID] = item.Type
	}
	return nil
}

func recordAgentFrame(result *codingagent.Result, raw json.RawMessage, event func(codingagent.NativeEvent) error) error {
	count := len(raw)
	if event == nil {
		for _, frame := range result.Evidence {
			count += len(frame)
		}
	}

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

func consumeAgentFrame(frame rpcFrame, raw json.RawMessage, result *codingagent.Result, role codingagent.Role, items map[string]string, event func(codingagent.NativeEvent) error) (bool, error) {
	return consumeAgentFrameTracked(frame, raw, result, role, items, nil, event)
}

func consumeAgentFrameTracked(frame rpcFrame, raw json.RawMessage, result *codingagent.Result, role codingagent.Role, items, closed map[string]string, event func(codingagent.NativeEvent) error) (bool, error) {
	if err := validateAgentFrameBinding(frame, result); err != nil {
		return false, err
	}
	if err := validateAgentItemsTracked(frame, role, items, closed, true); err != nil {
		return false, err
	}
	if agentEvidenceMethod(frame.Method) {
		if err := recordAgentFrame(result, raw, event); err != nil {
			return false, err
		}
	}
	done, err := collectAgentFrame(frame, raw, result, role)
	if done && err == nil {
		// A completed native turn can omit the final notification for an
		// internal reasoning item. Its unmatched observation does not imply
		// unfinished work. Retain the receipts and require all effectful items
		// to close normally; only the validated terminal turn closes reasoning.
		for id, state := range items {
			if nativeItemKind(state) == "reasoning" {
				delete(items, id)
			}
		}
	}
	return done, err
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
	if len(result.Content) != 0 || strings.TrimSpace(content) == "" {
		return fmt.Errorf("agent returned a missing or duplicate final response")
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
