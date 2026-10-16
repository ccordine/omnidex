package codex

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
)

func agentWriteTool() map[string]any {
	text := map[string]string{"type": "string"}
	return map[string]any{"type": "function", "name": "omnidex_write_file", "description": "Write complete source content to one file explicitly authorized by this task packet. First read an existing file and supply its exact original contents as expected_content; use an empty expected_content to create a missing authorized file.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"path": text, "content": text, "expected_content": text}, "required": []string{"path", "content", "expected_content"}, "additionalProperties": false}}
}

func agentDeleteTool() map[string]any {
	text := map[string]string{"type": "string"}
	return map[string]any{"type": "function", "name": "omnidex_delete_file", "description": "Delete one file explicitly authorized by this task packet. First read the file and supply its exact original contents as expected_content.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"path": text, "expected_content": text}, "required": []string{"path", "expected_content"}, "additionalProperties": false}}
}

func agentReadTool() map[string]any {
	text := map[string]string{"type": "string"}
	return map[string]any{"type": "function", "name": "omnidex_read_file", "description": "Read one exact file from the assignment's explicit file inventory.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"path": text}, "required": []string{"path"}, "additionalProperties": false}}
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
	read := params.Tool == "omnidex_read_file"
	mutate := params.Tool == "omnidex_write_file" || params.Tool == "omnidex_delete_file"
	if writer == nil || frame.Method != "item/tool/call" || frame.ID == nil || params.ThreadID != result.ThreadID || params.TurnID != result.TurnID || params.CallID == "" ||
		(!read && !mutate) || mutate && call.Role != codingagent.Execute || read && call.Role != codingagent.Execute && call.Role != codingagent.Review && !call.Role.IsPlanning() ||
		(params.Namespace != nil && *params.Namespace != "") {
		return fmt.Errorf("agent requested an ungranted or unbound tool")
	}
	if err := recordAgentFrame(result, connection.lastFrame, event); err != nil {
		return err
	}
	if event != nil {
		activity := agentActivityBase(call, result, params.CallID, model.AgentActivityCommand, model.AgentActivityStarted)
		activity.Command, activity.CWD = params.Tool, call.Root
		if err := event(codingagent.NativeEvent{Activity: &activity}); err != nil {
			return err
		}
	}
	path := ""
	content := ""
	var writeErr error
	if read {
		var request struct {
			Path string `json:"path"`
		}
		writeErr = exactjson.ValidateObject(params.Arguments, &request, "agent file read")
		if writeErr == nil {
			writeErr = json.Unmarshal(params.Arguments, &request)
		}
		path = request.Path
		if writeErr == nil {
			content, writeErr = writer.ReadFile(ctx, path)
		}
	} else if params.Tool == "omnidex_write_file" {
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
	if read {
		message = content
	}
	if writeErr != nil {
		message = writeErr.Error()
	}
	encoded, err := json.Marshal(map[string]any{"id": *frame.ID, "result": map[string]any{"contentItems": []any{map[string]string{"type": "inputText", "text": message}}, "success": writeErr == nil}})
	if err != nil {
		return err
	}
	receipt := encoded
	if read && writeErr == nil {
		digest := sha256.Sum256([]byte(content))
		receipt, err = json.Marshal(map[string]any{"id": *frame.ID, "result": map[string]any{"contentItems": []any{map[string]string{"type": "inputText", "text": fmt.Sprintf("Read %d bytes (sha256 %x).", len(content), digest)}}, "success": true}})
		if err != nil {
			return err
		}
	}
	if err := recordAgentFrame(result, receipt, event); err != nil {
		return err
	}
	if event != nil {
		status := model.AgentActivityCompleted
		if writeErr != nil {
			status = model.AgentActivityFailed
		}
		activity := agentActivityBase(call, result, params.CallID, model.AgentActivityCommand, status)
		activity.Command, activity.CWD = params.Tool, call.Root
		if err := event(codingagent.NativeEvent{Activity: &activity}); err != nil {
			return err
		}
	}
	if writeErr == nil && event != nil {
		kind := model.AgentActivityFileEdit
		if read {
			kind = model.AgentActivityFileRead
		}
		activity := agentActivityBase(call, result, params.CallID, kind, model.AgentActivityCompleted)
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
