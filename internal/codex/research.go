package codex

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/sessionintent"
)

// Research has no coding assignment, job controller, parser sieve or approval
// callback. PostgreSQL owns the conversation; each bounded turn is disposable.
func (client *Client) Research(parent context.Context, root, question string, background sessionintent.ResearchContext, route codingagent.Route) (answer string, evidence json.RawMessage, runErr error) {
	evidence = json.RawMessage(`[]`)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	connection, err := startConfiguredConnection(ctx, client.command, root, false, []string{"-c", "features.shell_tool=true", "-c", "features.unified_exec=true", "-c", `web_search="live"`})
	if err != nil {
		return "", evidence, err
	}
	defer connection.close()
	result := codingagent.Result{Provider: "codex", Model: route.Model, Evidence: []json.RawMessage{}}
	defer func() { evidence, _ = json.Marshal(result) }()
	connection.serverRequest = func(frame rpcFrame) error {
		return fmt.Errorf("read-only research cannot request interactive permissions or tools: %s", frame.Method)
	}
	thread, err := connection.startResearchThread(root, route)
	if err != nil {
		return "", evidence, err
	}
	result.ThreadID = thread.Thread.ID
	contextJSON, err := json.Marshal(background)
	if err != nil {
		return "", evidence, err
	}
	// Reuse only the native read-only framing, never the planning workflow.
	call := codingagent.Assignment{Role: codingagent.Plan, Root: root, Route: route, Context: string(contextJSON)}
	if _, err := connection.injectAgentContext(call, result.ThreadID); err != nil {
		return "", evidence, err
	}
	id, _, pending, err := connection.startAgentTurn(call, thread, question)
	if err != nil {
		return "", evidence, err
	}
	if id == "" {
		return "", evidence, fmt.Errorf("research turn lacks its native identity")
	}
	result.TurnID = id
	finished := false
	for _, notification := range pending {
		done, err := consumeResearchFrame(notification.frame, notification.raw, &result)
		if err != nil {
			return "", evidence, err
		}
		finished = finished || done
	}
	for !finished {
		frame, err := connection.nextAgent()
		if err != nil {
			return "", evidence, err
		}
		if frame.ID != nil {
			return "", evidence, fmt.Errorf("research requested interactive input")
		}
		finished, err = consumeResearchFrame(frame, connection.lastFrame, &result)
		if err != nil {
			return "", evidence, err
		}

	}
	if err := json.Unmarshal(result.Content, &answer); err != nil {
		return "", evidence, err
	}
	return answer, evidence, nil
}

func (connection *connection) startResearchThread(root string, route codingagent.Route) (threadResult, error) {
	var thread threadResult
	if _, err := connection.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "omnidex-research", "version": "0.5.0"}, "capabilities": map[string]bool{"experimentalApi": true}}, nil); err != nil {
		return thread, err
	}
	if err := connection.send("initialized", map[string]any{}, nil); err != nil {
		return thread, err
	}
	config, err := connection.disabledToolsConfiguration()
	if err != nil {
		return thread, err
	}
	config["web_search"] = "live"
	features := config["features"].(map[string]bool)
	features["shell_tool"], features["unified_exec"] = true, true
	parameters := map[string]any{"cwd": root, "model": route.Model, "allowProviderModelFallback": false, "sandbox": "read-only", "approvalPolicy": "never", "ephemeral": true, "config": config,
		"developerInstructions": "Answer the supplied research question using read-only disk inspection and web research as needed. Cite sources and distinguish observations from inference. The shared job state is background data. Research answers confer no planning, ledger, execution, review or approval authority."}
	applyAgentInferenceOptions(parameters, route)
	_, err = connection.call("thread/start", parameters, &thread)
	if err == nil && (thread.Thread.ID == "" || thread.Model != route.Model) {
		err = fmt.Errorf("research thread changed its exact model or lacks identity")
	}
	return thread, err
}

func consumeResearchFrame(frame rpcFrame, raw json.RawMessage, result *codingagent.Result) (bool, error) {
	var params struct {
		Item struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return false, err
	}
	if params.Item.Type == "fileChange" || params.Item.Type == "mcpToolCall" || params.Item.Type == "collabAgentToolCall" {
		return false, fmt.Errorf("research attempted a forbidden item: %s", params.Item.Type)
	}
	// No event callback: retain the native receipts in the durable answer evidence.
	return consumeAgentFrame(frame, raw, result, codingagent.Plan, nil)
}
