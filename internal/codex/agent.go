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
	requiresFileAccess := call.Role == codingagent.Execute || call.Role == codingagent.Review || call.Role.IsPlanning() && len(call.ReadFiles) > 0
	if requiresFileAccess != (writer != nil) {
		return result, fmt.Errorf("agent assignment changed its exact scoped file access")
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
	connection, err := startConfiguredConnection(ctx, client.command, call.Root, true, nil)
	if err != nil {
		return result, retryableAgentSessionError(err)
	}
	defer connection.close()
	thread, raw, err := connection.startAgentThread(call, scratch)
	if err != nil {
		return result, retryableAgentSessionError(err)
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
		return result, retryableAgentSessionError(err)
	}
	result.TurnID = turnID
	if result.TurnID == "" {
		return result, fmt.Errorf("agent turn lacks its native identity")
	}
	items := map[string]string{}
	closedItems := map[string]string{}
	finished := false
	for _, notification := range pending {
		var done bool
		done, err = consumeAgentFrameTracked(notification.frame, notification.raw, &result, call.Role, items, closedItems, event)
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
			return result, retryableAgentSessionError(err)
		}
		if frame.ID != nil {
			if frame.Method == "item/tool/requestUserInput" {
				return result, fmt.Errorf("frozen task agents cannot request owner input")
			}
			if err := connection.handleAgentTool(ctx, frame, &result, call, writer, event); err != nil {
				return result, err
			}
			continue
		}
		finished, err = consumeAgentFrameTracked(frame, connection.lastFrame, &result, call.Role, items, closedItems, event)
		if err != nil {
			return result, err
		}
	}
	if !finished {
		err := ctx.Err()
		if err == nil {
			err = fmt.Errorf("agent session ended before its terminal frame")
		}
		return result, retryableAgentSessionError(err)
	}
	if len(items) > 0 {
		return result, fmt.Errorf("agent ended with incomplete native items")
	}
	mapped, err := mapAgentReviewRunResult(call, result.Content)
	if err != nil {
		return result, err
	}
	result.Content = mapped
	return result, nil
}

// Provider processes and their streams are disposable. Losing one never proves
// the durable task failed; the worker must open a fresh session against the same
// frozen task and persisted evidence.
func retryableAgentSessionError(err error) error {
	if err == nil {
		return nil
	}
	return &agentInferenceFailure{message: err.Error(), retryable: true}
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

func (connection *connection) startAgentThread(call codingagent.Assignment, scratch string) (threadResult, json.RawMessage, error) {
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
	baseInstructions := "Perform only the supplied assignment. Omnidex owns workflow and side effects. Use only explicitly supplied file tools and return the requested structured result."
	configuration["shell_environment_policy"] = map[string]any{"inherit": "none", "set": agentToolEnvironment(scratch)}
	parameters := map[string]any{"model": call.Route.Model, "allowProviderModelFallback": false,
		"cwd": call.Root, "sandbox": "read-only", "approvalPolicy": "never", "config": configuration,
		"baseInstructions": baseInstructions,
		"serviceName":      "omnidex-agents", "ephemeral": true}
	applyAgentInferenceOptions(parameters, call.Route)
	if call.Role == codingagent.Execute {
		parameters["dynamicTools"] = []any{agentReadTool(), agentWriteTool(), agentDeleteTool()}
	} else if len(call.ReadFiles) > 0 {
		parameters["dynamicTools"] = []any{agentReadTool()}
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
	case codingagent.PlanningInitial, codingagent.PlanningTarget, codingagent.PlanningMutation,
		codingagent.PlanningCoverage, codingagent.PlanningAudit,
		codingagent.Execute, codingagent.Review:
	default:
		return fmt.Errorf("unknown agent assignment role")
	}
	if call.Role != codingagent.Execute && (len(call.WriteFiles) != 0 || len(call.DeleteFiles) != 0) {
		return fmt.Errorf("planning and review agents cannot receive write access")
	}
	for _, file := range append(append(append([]string{}, call.ReadFiles...), call.WriteFiles...), call.DeleteFiles...) {
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
