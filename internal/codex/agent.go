package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/codingagent"
)

func (client *Client) Run(parent context.Context, call codingagent.Assignment, writer codingagent.FileWriter, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	result := codingagent.Result{Provider: "codex", Model: call.Route.Model, Evidence: []json.RawMessage{}}
	if err := validateAgentAssignment(call); err != nil {
		return result, err
	}
	if (call.Role == codingagent.Execute) != (writer != nil) {
		return result, fmt.Errorf("execution requires its scoped file writer; other roles cannot receive one")
	}
	activity := newAgentActivityStream(call, &result, event)
	event = activity.observe
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	scratch, cleanup, err := client.agentScratch()
	if err != nil {
		return result, err
	}
	defer cleanup()
	profile, err := client.executionPermissionProfile(call, scratch)
	if err != nil {
		return result, err
	}
	connection, err := startConfiguredConnection(ctx, client.command, call.Root, true, agentProfileFlags(profile))
	if err != nil {
		return result, err
	}
	defer connection.close()
	thread, raw, err := connection.startAgentThread(call, scratch, profile)
	if err != nil {
		return result, err
	}
	result.ThreadID = thread.Thread.ID
	if err := recordAgentFrame(&result, raw, event); err != nil {
		return result, err
	}
	prompt, err := renderAgentAssignment(call)
	if err != nil {
		return result, err
	}
	turnID, raw, pending, err := connection.startAgentTurn(call, thread, prompt)
	if err != nil {
		return result, err
	}
	result.TurnID = turnID
	if result.TurnID == "" {
		return result, fmt.Errorf("agent turn lacks its native identity")
	}
	items := map[string]string{}
	finished := false
	for _, notification := range pending {
		var done bool
		done, err = consumeAgentFrame(notification.frame, notification.raw, &result, call.Role, items, event)
		if err != nil {
			return result, err
		}
		finished = finished || done
	}
	if err := recordAgentFrame(&result, raw, event); err != nil {
		return result, err
	}
	for !finished && ctx.Err() == nil {
		frame, err := connection.nextAgent()
		if err != nil {
			return result, err
		}
		if frame.ID != nil {
			if err := connection.handleAgentTool(ctx, frame, &result, call, writer, event); err != nil {
				return result, err
			}
			continue
		}
		finished, err = consumeAgentFrame(frame, connection.lastFrame, &result, call.Role, items, event)
		if err != nil {
			return result, err
		}
	}
	if !finished {
		return result, ctx.Err()
	}
	if len(items) > 0 {
		return result, fmt.Errorf("agent ended with incomplete native items")
	}
	mapped, err := mapAgentReviewResult(call, result.Content)
	if err != nil {
		return result, err
	}
	result.Content = mapped
	return result, nil
}

func agentEvidenceMethod(method string) bool {
	switch method {
	case "item/started", "item/completed", "turn/completed", "thread/tokenUsage/updated", "error",
		"item/agentMessage/delta", "item/plan/delta", "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded", "item/commandExecution/outputDelta":
		return true
	default:
		return false
	}
}

func (connection *connection) startAgentThread(call codingagent.Assignment, scratch string, profile map[string]any) (threadResult, json.RawMessage, error) {
	var thread threadResult
	if _, err := connection.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "omnidex", "title": "Omnidex", "version": "0.5.0"}, "capabilities": map[string]bool{"experimentalApi": true}}, nil); err != nil {
		return thread, nil, err
	}
	if err := connection.send("initialized", map[string]any{}, nil); err != nil {
		return thread, nil, err
	}
	configuration, err := connection.disabledToolsConfiguration()
	if err != nil {
		return thread, nil, err
	}
	features := configuration["features"].(map[string]bool)
	features["shell_tool"], features["unified_exec"] = true, true
	features["shell_snapshot"] = false
	features["fast_mode"] = true
	configuration["permissions"] = map[string]any{"omnidex-task": profile}
	configuration["shell_environment_policy"] = map[string]any{"inherit": "none", "set": agentToolEnvironment(scratch)}
	parameters := map[string]any{"model": call.Route.Model, "allowProviderModelFallback": false,
		"cwd": call.Root, "permissions": "omnidex-task", "approvalPolicy": "never", "config": configuration,
		"baseInstructions": "Perform only the supplied scoped assignment. Use the granted files and tools. Return the requested structured result.",
		"serviceName":      "omnidex-agents", "ephemeral": true}
	applyAgentInferenceOptions(parameters, call.Route)
	if call.Role == codingagent.Execute {
		parameters["dynamicTools"] = []any{agentWriteTool()}
	}
	raw, err := connection.call("thread/start", parameters, &thread)
	if err == nil && (thread.Model != call.Route.Model || thread.Thread.ID == "") {
		err = fmt.Errorf("agent thread changed its immutable model or lacks identity")
	}
	return thread, raw, err
}

func validateAgentAssignment(call codingagent.Assignment) error {
	if err := call.ValidateTaskContext(); err != nil {
		return err
	}
	if call.JobID < 1 || call.Generation < 1 || !filepath.IsAbs(call.Root) || filepath.Clean(call.Root) != call.Root || call.Route.Provider != "codex" || call.Route.Model == "" {
		return fmt.Errorf("agent assignment lacks exact authority, workspace or route")
	}
	switch call.Role {
	case codingagent.Plan, codingagent.Execute, codingagent.Review, codingagent.OverallReview:
	default:
		return fmt.Errorf("unknown agent assignment role")
	}
	if call.Role != codingagent.Execute && len(call.WriteFiles) != 0 {
		return fmt.Errorf("planning and review agents cannot receive write access")
	}
	for _, file := range append(append([]string{}, call.ReadFiles...), call.WriteFiles...) {
		if err := codingagent.ValidatePath(file); err != nil {
			return err
		}
		absolute := filepath.Join(call.Root, filepath.FromSlash(file))
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil || resolved != absolute {
			return fmt.Errorf("agent file scope must be an existing physical file: %s", file)
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("agent file scope must name a regular file: %s", file)
		}
	}
	return nil
}
