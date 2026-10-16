package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/codingagent"
)

func (client *Client) Run(parent context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	result := codingagent.Result{Provider: "codex", Model: call.Route.Model, Evidence: []json.RawMessage{}}
	if err := validateAgentAssignment(call); err != nil {
		return result, err
	}
	activity := newAgentActivityStream(call, &result, event)
	event = activity.observe
	ctx, cancel := context.WithCancel(parent)
	if !call.Role.IsNative() {
		cancel()
		ctx, cancel = context.WithTimeout(parent, client.timeout)
	}
	defer cancel()
	var scratch string
	var err error
	if !call.Role.IsNative() {
		var cleanup func()
		scratch, cleanup, err = client.agentScratch()
		if err != nil {
			return result, err
		}
		defer cleanup()
	}
	var connection *connection
	if call.Role.IsNative() {
		connection, err = startExecutionConnection(ctx, client.command, call.Root)
	} else {
		connection, err = startConfiguredConnection(ctx, client.command, call.Root, true, nil)
	}
	if err != nil {
		return result, retryableAgentSessionError(err)
	}
	defer connection.close()
	connection.bindAgentRequests(ctx, call, &result, event)
	thread, raw, err := connection.startAgentThread(call, scratch)
	if err != nil {
		return result, retryableAgentSessionError(err)
	}
	if result.ThreadID != "" && result.ThreadID != thread.Thread.ID {
		return result, fmt.Errorf("native thread start differs from its early request")
	}
	result.ThreadID = thread.Thread.ID
	if err := recordAgentFrame(&result, raw, event); err != nil {
		return result, err
	}
	if call.Role == codingagent.Execute {
		if call.Session != nil && call.Session.State != "idle" {
			return connection.recoverExecutionResult(call, result, event)
		}
	}
	prompt, err := renderAgentAssignment(call)
	if err != nil {
		return result, err
	}
	contextReceipt, err := connection.injectAgentContext(call, thread.Thread.ID)
	if err != nil {
		return result, retryableAgentSessionError(err)
	}
	if len(contextReceipt) != 0 {
		if err := recordAgentFrame(&result, contextReceipt, event); err != nil {
			return result, err
		}
	}
	if call.Role == codingagent.Execute {
		session := &codingagent.ExecutionSession{ThreadID: result.ThreadID, State: "dispatching"}
		if call.Session != nil {
			session.PreviousTurnID = call.Session.TurnID
		}
		if err := event(codingagent.NativeEvent{Session: session}); err != nil {
			return result, err
		}
	}
	turnID, raw, pending, err := connection.startAgentTurn(call, thread, prompt)
	if err != nil {
		return result, retryableAgentSessionError(err)
	}
	if result.TurnID != "" && result.TurnID != turnID {
		return result, fmt.Errorf("native turn start differs from its early request")
	}
	result.TurnID = turnID
	if result.TurnID == "" {
		return result, fmt.Errorf("agent turn lacks its native identity")
	}
	if call.Role == codingagent.Execute {
		if err := event(codingagent.NativeEvent{Session: &codingagent.ExecutionSession{ThreadID: result.ThreadID, TurnID: result.TurnID, State: "running"}}); err != nil {
			return result, err
		}
	}
	items := map[string]string{}
	closedItems := map[string]string{}
	messages := &agentResponseMessages{}
	consume := func(frame rpcFrame, raw json.RawMessage) (bool, error) {
		if resumedUsageSnapshot(call, frame) {
			return false, nil
		}
		return consumeAgentFrameTracked(frame, raw, &result, call.Role, items, closedItems, messages, event)
	}
	finished := false
	for _, notification := range pending {
		var done bool
		done, err = consume(notification.frame, notification.raw)
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
			if err := connection.serverRequest(frame); err != nil {
				return result, err
			}
			continue
		}
		finished, err = consume(frame, connection.lastFrame)
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
	mapped, err := mapAgentReviewResponse(call, result.Content)
	if err != nil {
		return result, err
	}
	result.Content = mapped
	return result, nil
}

// Pure inference may be retried. Effectful execution is never blindly retried;
// its persisted native thread is the only continuation authority.
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
	if call.Role.IsNative() {
		parameters := map[string]any{"model": call.Route.Model, "allowProviderModelFallback": false,
			"cwd": call.Root, "sandbox": "danger-full-access", "approvalPolicy": "never", "ephemeral": call.Role != codingagent.Execute}
		applyAgentInferenceOptions(parameters, call.Route)
		method := "thread/start"
		if call.Role == codingagent.Execute && call.Session != nil {
			method = "thread/resume"
			parameters["threadId"] = call.Session.ThreadID
			delete(parameters, "ephemeral")
			parameters["excludeTurns"] = true
		}
		raw, err := connection.call(method, parameters, &thread)
		if err == nil && (thread.Model != call.Route.Model || thread.Thread.ID == "") {
			err = fmt.Errorf("agent thread changed its immutable model or lacks identity")
		}
		if err == nil && call.Session != nil && thread.Thread.ID != call.Session.ThreadID {
			err = fmt.Errorf("agent continuation changed its exact task thread")
		}
		return thread, raw, err
	}
	configuration, err := connection.disabledToolsConfiguration()
	if err != nil {
		return thread, nil, err
	}
	baseInstructions := "Parse only the supplied response into the requested result."
	configuration["shell_environment_policy"] = map[string]any{"inherit": "none", "set": agentToolEnvironment(scratch)}
	parameters := map[string]any{"model": call.Route.Model, "allowProviderModelFallback": false,
		"cwd": call.Root, "sandbox": "read-only", "approvalPolicy": "never", "config": configuration,
		"baseInstructions": baseInstructions,
		"serviceName":      "omnidex-agents", "ephemeral": true}
	applyAgentInferenceOptions(parameters, call.Route)
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
	case codingagent.Plan, codingagent.PlanningInitial, codingagent.PlanningMutation, codingagent.ExecutionResponse, codingagent.ReviewResponse,
		codingagent.Execute, codingagent.Review:
	default:
		return fmt.Errorf("unknown agent assignment role")
	}
	if call.Role != codingagent.Execute && (len(call.WriteFiles) != 0 || len(call.DeleteFiles) != 0) {
		return fmt.Errorf("planning and review agents cannot receive write access")
	}
	if call.Role.IsNative() {
		return nil
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
