package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/codingagent"
)

func (connection *connection) handleNativeInput(frame rpcFrame, result *codingagent.Result, event func(codingagent.NativeEvent) error) error {
	request, err := nativeInputRequest(frame)
	if err != nil {
		return err
	}
	if request.ThreadID == "" || request.ThreadID != result.ThreadID ||
		request.TurnID != "" && result.TurnID != "" && request.TurnID != result.TurnID || event == nil {
		return fmt.Errorf("native input request changed its task thread or turn")
	}
	if result.TurnID == "" {
		result.TurnID = request.TurnID
	}
	if err := recordAgentFrame(result, connection.lastFrame, event); err != nil {
		return err
	}
	input := codingagent.NativeInput{Request: request}
	if err := event(codingagent.NativeEvent{Input: &input}); err != nil {
		return err
	}
	if !json.Valid(input.Response) {
		return fmt.Errorf("native request lacks the owner's response")
	}
	response, err := json.Marshal(map[string]any{"id": frame.ID, "result": input.Response})
	if err != nil {
		return err
	}
	if err := recordAgentFrame(result, response, event); err != nil {
		return err
	}
	n, err := connection.input.Write(append(response, '\n'))
	if err == nil && n != len(response)+1 {
		return io.ErrShortWrite
	}
	return err
}

func nativeInputRequest(frame rpcFrame) (codingagent.NativeInputRequest, error) {
	var params struct {
		ThreadID           string            `json:"threadId"`
		TurnID             string            `json:"turnId"`
		AvailableDecisions []json.RawMessage `json:"availableDecisions"`
		Permissions        json.RawMessage   `json:"permissions"`
		Mode               string            `json:"mode"`
		RequestedSchema    json.RawMessage   `json:"requestedSchema"`
		Questions          []struct {
			ID       string `json:"id"`
			Question string `json:"question"`
			Options  []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	request := codingagent.NativeInputRequest{ID: append(json.RawMessage(nil), frame.ID...), Method: frame.Method}
	if err := json.Unmarshal(frame.Params, &params); err != nil || frame.ID == nil {
		return request, fmt.Errorf("native request lacks its protocol identity or parameters")
	}
	request.ThreadID, request.TurnID = params.ThreadID, params.TurnID
	details, err := json.MarshalIndent(json.RawMessage(frame.Params), "", "  ")
	if err != nil {
		return request, err
	}
	request.Prompt = "Codex requests owner input (" + frame.Method + "):\n" + string(details)
	choice := func(label string, response any) {
		encoded, _ := json.Marshal(response)
		request.Choices = append(request.Choices, codingagent.NativeInputChoice{Label: label, Response: encoded})
	}
	switch frame.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		decisions := params.AvailableDecisions
		if len(decisions) == 0 {
			decisions = []json.RawMessage{json.RawMessage(`"accept"`), json.RawMessage(`"acceptForSession"`), json.RawMessage(`"decline"`), json.RawMessage(`"cancel"`)}
		}
		for index, decision := range decisions {
			var value string
			_ = json.Unmarshal(decision, &value)
			label := map[string]string{"accept": "approve", "acceptForSession": "approve for session", "decline": "deny", "cancel": "cancel"}[value]
			if label == "" {
				label = fmt.Sprintf("choice %d", index+1)
				request.Prompt += fmt.Sprintf("\n%s: %s", label, decision)
			}
			choice(label, map[string]any{"decision": decision})
		}
	case "item/permissions/requestApproval":
		if !json.Valid(params.Permissions) || string(params.Permissions) == "null" {
			return request, fmt.Errorf("native permission request lacks requested permissions")
		}
		choice("approve", map[string]any{"permissions": params.Permissions, "scope": "turn"})
		choice("deny", map[string]any{"permissions": map[string]any{}, "scope": "turn"})
	case "item/tool/requestUserInput":
		seen := map[string]bool{}
		for _, question := range params.Questions {
			if question.ID == "" || seen[question.ID] {
				return request, fmt.Errorf("native questions require distinct IDs")
			}
			seen[question.ID] = true
			item := codingagent.NativeInputQuestion{ID: question.ID, Text: question.Question}
			for _, option := range question.Options {
				item.Options = append(item.Options, option.Label)
			}
			request.Questions = append(request.Questions, item)
		}
		if len(request.Questions) == 0 {
			return request, fmt.Errorf("native input request contains no questions")
		}
		if len(request.Questions) == 1 {
			request.Prompt += "\nReply with your answer."
		} else {
			request.Prompt += "\nReply with a JSON object mapping each question ID to its answer."
		}
	case "mcpServer/elicitation/request":
		choice("deny", map[string]any{"action": "decline", "content": nil})
		choice("cancel", map[string]any{"action": "cancel", "content": nil})
		switch params.Mode {
		case "url":
			choice("done", map[string]any{"action": "accept", "content": nil})
		case "form", "openai/form":
			if len(params.RequestedSchema) == 0 {
				return request, fmt.Errorf("native form lacks its requested schema")
			}
			request.Form = params.RequestedSchema
			request.Prompt += "\nReply with the requested fields as a JSON object."
		default:
			return request, fmt.Errorf("unsupported native elicitation mode %q", params.Mode)
		}
	default:
		return request, fmt.Errorf("unsupported native client request %q", frame.Method)
	}
	if len(request.Choices) > 0 {
		request.Prompt += "\nReply with one exact choice:"
		for _, item := range request.Choices {
			request.Prompt += "\n  " + item.Label
		}
	}
	return request, nil
}

func (connection *connection) bindAgentRequests(ctx context.Context, call codingagent.Assignment, result *codingagent.Result, event func(codingagent.NativeEvent) error) {
	connection.serverRequest = func(frame rpcFrame) error {
		if call.Role.IsNative() {
			if call.Role == codingagent.Plan && (frame.Method == "item/commandExecution/requestApproval" ||
				frame.Method == "item/fileChange/requestApproval" || frame.Method == "item/permissions/requestApproval") {
				return fmt.Errorf("planning cannot request execution permissions before ledger approval")
			}
			// A new thread can elicit input before thread/start is acknowledged.
			// This connection owns exactly one assignment; bind that provisional
			// identity and require the later acknowledgement to confirm it.
			if result.ThreadID == "" {
				var params struct {
					ThreadID string `json:"threadId"`
				}
				if err := json.Unmarshal(frame.Params, &params); err != nil {
					return err
				}
				if call.Session != nil && params.ThreadID != call.Session.ThreadID {
					return fmt.Errorf("native request changed the resumed task thread")
				}
				result.ThreadID = params.ThreadID
			}
			return connection.handleNativeInput(frame, result, event)
		}
		return fmt.Errorf("semantic parser cannot request tools")
	}
}
