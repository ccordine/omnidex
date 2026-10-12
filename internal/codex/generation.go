package codex

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

type threadResult struct {
	Model  string `json:"model"`
	Thread struct {
		ID    string `json:"id"`
		Turns []struct {
			ID string `json:"id"`
		} `json:"turns"`
	} `json:"thread"`
}

func (connection *connection) generate(ctx context.Context, request llm.CodexStationRequest) (llm.CodexResponseCapture, bool, error) {
	var capture llm.CodexResponseCapture
	if _, err := connection.call("initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "omnidex", "title": "Omnidex", "version": "0.5.0"},
		"capabilities": map[string]bool{"experimentalApi": true}}, nil); err != nil {
		return capture, false, err
	}
	if err := connection.send("initialized", map[string]any{}, nil); err != nil {
		return capture, false, err
	}
	configuration, err := connection.disabledToolsConfiguration()
	if err != nil {
		return capture, false, err
	}
	parameters := map[string]any{"model": request.Model, "allowProviderModelFallback": false,
		"sandbox": "read-only", "approvalPolicy": "never", "environments": []any{},
		"runtimeWorkspaceRoots": []string{}, "dynamicTools": []any{}, "config": configuration,
		"baseInstructions": "Answer the supplied semantic question using its supplied context. Return only the requested result.",
		"serviceName":      "omnidex", "ephemeral": !request.RetainContext}
	method := "thread/start"
	if request.Continuation != nil {
		method = "thread/resume"
		parameters["threadId"] = request.Continuation.ThreadID
	}
	var thread threadResult
	capture.Thread, err = connection.call(method, parameters, &thread)
	if err != nil {
		return capture, false, err
	}
	if thread.Model != request.Model {
		return capture, false, fmt.Errorf("Codex changed the immutable model route")
	}
	if err := validateContinuedThread(request.Continuation, thread); err != nil {
		return capture, false, err
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	capture.Turn, err = connection.call("turn/start", map[string]any{"threadId": thread.Thread.ID,
		"input": []any{map[string]string{"type": "text", "text": request.Prompt}}, "model": request.Model,
		"environments": []any{}, "runtimeWorkspaceRoots": []string{}, "effort": "low", "summary": "none",
		"approvalPolicy": "never", "sandboxPolicy": map[string]any{"type": "readOnly", "networkAccess": false}}, &turn)
	if err != nil {
		return capture, true, err
	}
	if thread.Thread.ID == "" || turn.Turn.ID == "" {
		return capture, true, fmt.Errorf("Codex did not identify its original request")
	}
	for ctx.Err() == nil {
		frame, err := connection.next()
		if err != nil {
			return capture, true, err
		}
		finished, err := collectGenerationFrame(frame, connection.lastFrame, &capture, thread.Thread.ID, turn.Turn.ID)
		if err != nil {
			return capture, true, err
		}
		if finished {
			return capture, true, nil
		}
	}
	return capture, true, ctx.Err()
}

func validateContinuedThread(parent *llm.CodexContinuation, thread threadResult) error {
	if parent == nil {
		return nil
	}
	if thread.Thread.ID != parent.ThreadID || thread.Model != parent.Model || len(thread.Thread.Turns) == 0 ||
		thread.Thread.Turns[len(thread.Thread.Turns)-1].ID != parent.TurnID {
		return fmt.Errorf("Codex continuation lost its original native model context")
	}
	return nil
}

func (connection *connection) disabledToolsConfiguration() (map[string]any, error) {
	var current struct {
		Config struct {
			MCPServers map[string]any `json:"mcp_servers"`
		} `json:"config"`
	}
	if _, err := connection.call("config/read", map[string]bool{"includeLayers": false}, &current); err != nil {
		return nil, err
	}
	servers := make(map[string]any, len(current.Config.MCPServers))
	for name := range current.Config.MCPServers {
		servers[name] = map[string]bool{"enabled": false}
	}
	return map[string]any{"mcp_servers": servers, "project_doc_max_bytes": 0, "web_search": "disabled",
		"agents": map[string]bool{"enabled": false}, "features": map[string]bool{
			"shell_tool": false, "unified_exec": false, "apps": false, "plugins": false, "multi_agent": false, "hooks": false}}, nil
}

func collectGenerationFrame(frame rpcFrame, raw json.RawMessage, capture *llm.CodexResponseCapture, threadID, turnID string) (bool, error) {
	var params struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		Item     struct {
			Type  string `json:"type"`
			Phase string `json:"phase"`
		} `json:"item"`
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return false, err
	}
	if params.ThreadID != "" && params.ThreadID != threadID {
		return false, fmt.Errorf("Codex notification belongs to another thread")
	}
	if params.TurnID != "" && params.TurnID != turnID {
		return false, fmt.Errorf("Codex notification belongs to another turn")
	}
	switch frame.Method {
	case "item/started", "item/completed":
		if params.Item.Type != "agentMessage" && params.Item.Type != "reasoning" && params.Item.Type != "userMessage" {
			return false, fmt.Errorf("Codex attempted an effect outside semantic generation: %s", params.Item.Type)
		}
		if frame.Method == "item/completed" && params.Item.Type == "agentMessage" && params.Item.Phase == "final_answer" {
			if len(capture.Message) != 0 {
				return false, fmt.Errorf("Codex returned more than one semantic result")
			}
			capture.Message = append(json.RawMessage(nil), raw...)
		}
	case "thread/tokenUsage/updated":
		capture.Usage = append(json.RawMessage(nil), raw...)
	case "turn/completed":
		if params.Turn.ID != turnID || params.Turn.Status != "completed" {
			return false, fmt.Errorf("Codex did not complete the original semantic request")
		}
		capture.Completion = append(json.RawMessage(nil), raw...)
		return true, nil
	case "error":
		return false, fmt.Errorf("Codex native inference reported an error")
	}
	return false, nil
}
