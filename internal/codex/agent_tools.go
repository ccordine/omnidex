package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/exactjson"
)

func agentWriteTool() map[string]any {
	text := map[string]string{"type": "string"}
	return map[string]any{"type": "function", "name": "omnidex_write_file", "description": "Write complete source content to one granted file. First read the file. Supply its exact original contents as expected_content to prevent overwriting an intervening edit.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"path": text, "content": text, "expected_content": text}, "required": []string{"path", "content", "expected_content"}, "additionalProperties": false}}
}

func (connection *connection) handleAgentTool(ctx context.Context, frame rpcFrame, result *codingagent.Result, call codingagent.Assignment, writer codingagent.FileWriter, event func(json.RawMessage) error) error {
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
	if call.Role != codingagent.Execute || writer == nil || frame.Method != "item/tool/call" || frame.ID == nil || params.ThreadID != result.ThreadID || params.TurnID != result.TurnID || params.CallID == "" || params.Tool != "omnidex_write_file" || (params.Namespace != nil && *params.Namespace != "") {
		return fmt.Errorf("agent requested an ungranted or unbound tool")
	}
	if err := recordAgentFrame(result, connection.lastFrame, event); err != nil {
		return err
	}
	edit, writeErr := decodeAgentFileWrite(params.Arguments)
	if writeErr == nil {
		writeErr = writer.WriteFile(ctx, edit)
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
	count, err := connection.input.Write(append(encoded, '\n'))
	if err == nil && count != len(encoded)+1 {
		return io.ErrShortWrite
	}
	return err
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
