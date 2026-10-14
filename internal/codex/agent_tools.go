package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
)

type agentUserInputOption struct {
	Description string `json:"description"`
	Label       string `json:"label"`
}

type agentUserInputQuestion struct {
	Header   string                 `json:"header"`
	ID       string                 `json:"id"`
	IsOther  bool                   `json:"isOther"`
	IsSecret bool                   `json:"isSecret"`
	Options  []agentUserInputOption `json:"options"`
	Question string                 `json:"question"`
}

type agentUserInputRequest struct {
	AutoResolutionMs *uint64                  `json:"autoResolutionMs"`
	IsBlocking       bool                     `json:"isBlocking"`
	ItemID           string                   `json:"itemId"`
	Questions        []agentUserInputQuestion `json:"questions"`
	ThreadID         string                   `json:"threadId"`
	TurnID           string                   `json:"turnId"`
}

// Native Codex modes may ask a user question before they can emit their
// structured result. Convert that bounded request into the same durable
// Omnidex question flow used by explicit result questions. Closing the native
// call then preserves the fresh-session boundary when the answer resumes work.
func mapAgentUserInputRequest(call codingagent.Assignment, frame rpcFrame, result *codingagent.Result) ([]string, bool, error) {
	if frame.Method != "item/tool/requestUserInput" {
		return nil, false, nil
	}
	if frame.ID == nil {
		return nil, true, fmt.Errorf("native user-input request lacks an RPC identity")
	}
	var request agentUserInputRequest
	if err := exactjson.ValidateObject(frame.Params, &request, "native user-input request"); err != nil {
		return nil, true, err
	}
	if err := json.Unmarshal(frame.Params, &request); err != nil {
		return nil, true, err
	}
	if !request.IsBlocking || request.ItemID == "" || request.ThreadID != result.ThreadID || request.TurnID != result.TurnID || len(request.Questions) < 1 || len(request.Questions) > 3 {
		return nil, true, fmt.Errorf("native user-input request lacks exact call binding or bounded questions")
	}
	seen := map[string]bool{}
	questions := make([]string, len(request.Questions))
	for index, question := range request.Questions {
		if !agentQuestionText(question.ID, 128) || !agentQuestionText(question.Header, 64) || !agentQuestionText(question.Question, 4096) || question.IsSecret || seen[question.ID] || len(question.Options) > 3 {
			return nil, true, fmt.Errorf("native user-input request contains an invalid question")
		}
		seen[question.ID] = true
		formatted := question.Question
		if len(question.Options) > 0 {
			options := make([]string, len(question.Options))
			for optionIndex, option := range question.Options {
				if !agentQuestionText(option.Label, 256) || !agentQuestionText(option.Description, 1024) {
					return nil, true, fmt.Errorf("native user-input request contains an invalid option")
				}
				options[optionIndex] = option.Label + " — " + option.Description
			}
			formatted += " Options: " + strings.Join(options, "; ")
		}
		questions[index] = formatted
	}
	return questions, true, nil
}

func agentQuestionText(value string, maximum int) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && len(value) <= maximum && !strings.ContainsAny(value, "\x00\r\n")
}

func agentQuestionResult(role codingagent.Role, questions []string) (json.RawMessage, error) {
	var value any
	switch role {
	case codingagent.Plan:
		value = codingagent.Workload{Tasks: []codingagent.Task{}, Checks: []codingagent.Check{}, Questions: questions}
	case codingagent.Execute:
		value = codingagent.ExecutionResult{Questions: questions}
	case codingagent.Review, codingagent.OverallReview:
		value = codingagent.ReviewResult{Findings: []codingagent.Finding{}, Questions: questions}
	default:
		return nil, fmt.Errorf("native user-input request belongs to an unknown agent role")
	}
	return json.Marshal(value)
}

func agentWriteTool() map[string]any {
	text := map[string]string{"type": "string"}
	return map[string]any{"type": "function", "name": "omnidex_write_file", "description": "Write complete source content to one granted file. First read the file. Supply its exact original contents as expected_content to prevent overwriting an intervening edit.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"path": text, "content": text, "expected_content": text}, "required": []string{"path", "content", "expected_content"}, "additionalProperties": false}}
}

func agentDeleteTool() map[string]any {
	text := map[string]string{"type": "string"}
	return map[string]any{"type": "function", "name": "omnidex_delete_file", "description": "Delete one granted file. First read the file. Supply its exact original contents as expected_content to prevent deleting an intervening edit.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"path": text, "expected_content": text}, "required": []string{"path", "expected_content"}, "additionalProperties": false}}
}

func (connection *connection) handleAgentTool(ctx context.Context, frame rpcFrame, result *codingagent.Result, call codingagent.Assignment, writer codingagent.FileWriter, event func(codingagent.NativeEvent) error) error {
	var params struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		CallID    string          `json:"callId"`
		Tool      string          `json:"tool"`
		Namespace *string         `json:"namespace"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return err
	}
	if call.Role != codingagent.Execute || writer == nil || frame.Method != "item/tool/call" || frame.ID == nil || params.ThreadID != result.ThreadID || params.TurnID != result.TurnID || params.CallID == "" || (params.Tool != "omnidex_write_file" && params.Tool != "omnidex_delete_file") || (params.Namespace != nil && *params.Namespace != "") {
		return fmt.Errorf("agent requested an ungranted or unbound tool")
	}
	if err := recordAgentFrame(result, connection.lastFrame, event); err != nil {
		return err
	}
	path := ""
	var writeErr error
	if params.Tool == "omnidex_write_file" {
		var edit codingagent.FileWrite
		edit, writeErr = decodeAgentFileWrite(params.Arguments)
		path = edit.Path
		if writeErr == nil {
			writeErr = writer.WriteFile(ctx, edit)
		}
	} else {
		var deletion codingagent.FileDelete
		deletion, writeErr = decodeAgentFileDelete(params.Arguments)
		path = deletion.Path
		if writeErr == nil {
			writeErr = writer.DeleteFile(ctx, deletion)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	message := "Saved."
	if writeErr != nil {
		message = writeErr.Error()
	}
	encoded, err := json.Marshal(map[string]any{"id": *frame.ID, "result": map[string]any{"contentItems": []any{map[string]string{"type": "inputText", "text": message}}, "success": writeErr == nil}})
	if err != nil {
		return err
	}
	if err := recordAgentFrame(result, encoded, event); err != nil {
		return err
	}
	if writeErr == nil && event != nil {
		activity := agentActivityBase(call, result, params.CallID, model.AgentActivityFileEdit, model.AgentActivityCompleted)
		activity.Paths = []string{path}
		if err := event(codingagent.NativeEvent{Activity: &activity}); err != nil {
			return err
		}
	}
	count, err := connection.input.Write(append(encoded, '\n'))
	if err == nil && count != len(encoded)+1 {
		return io.ErrShortWrite
	}
	return err
}

func decodeAgentFileDelete(raw json.RawMessage) (codingagent.FileDelete, error) {
	var deletion codingagent.FileDelete
	if err := exactjson.ValidateObject(raw, &deletion, "agent file deletion"); err != nil {
		return deletion, err
	}
	var fields struct {
		Path            *string `json:"path"`
		ExpectedContent *string `json:"expected_content"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return deletion, err
	}
	if fields.Path == nil || fields.ExpectedContent == nil {
		return deletion, fmt.Errorf("agent file deletion requires path and expected_content strings")
	}
	return codingagent.FileDelete{Path: *fields.Path, ExpectedContent: *fields.ExpectedContent}, nil
}

func decodeAgentFileWrite(raw json.RawMessage) (codingagent.FileWrite, error) {
	var edit codingagent.FileWrite
	if err := exactjson.ValidateObject(raw, &edit, "agent file write"); err != nil {
		return edit, err
	}
	var fields struct {
		Path            *string `json:"path"`
		Content         *string `json:"content"`
		ExpectedContent *string `json:"expected_content"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return edit, err
	}
	if fields.Path == nil || fields.Content == nil || fields.ExpectedContent == nil {
		return edit, fmt.Errorf("agent file write requires path, content and expected_content strings")
	}
	return codingagent.FileWrite{Path: *fields.Path, Content: *fields.Content, ExpectedContent: *fields.ExpectedContent}, nil
}
