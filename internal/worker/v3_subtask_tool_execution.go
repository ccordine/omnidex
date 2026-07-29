package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/specialists"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

const (
	maxSubtaskToolTurns    = 24
	maxSubtaskToolCalls    = 24
	maxToolCallsPerTurn    = 1
	subtaskToolScopePrefix = "v3_subtask_tool"
)

type subtaskToolDecision struct {
	RoleID    string             `json:"role_id"`
	Status    string             `json:"status"`
	ToolCalls []toolruntime.Call `json:"tool_calls,omitempty"`
	Final     string             `json:"final,omitempty"`
	Error     string             `json:"error,omitempty"`
}

type subtaskToolRecord struct {
	Call   artifacts.ToolCallArtifact
	Result artifacts.ToolResultArtifact
}

func (r *nativeRuntimeV3) runSubtaskWithTools(assignment v3SubtaskAssignment, authoritativeObjective artifacts.Objective) (string, []string, error) {
	if r == nil || r.svc == nil || r.svc.v3Tools == nil {
		return "", nil, fmt.Errorf("tool runtime unavailable")
	}
	spec, ok := r.svc.skillSpec(assignment.RoleID)
	if !ok {
		return "", nil, fmt.Errorf("assigned specialist %q is not registered", assignment.RoleID)
	}
	authoritativeDescription := strings.TrimSpace(authoritativeObjective.Description)
	availableTools := r.availableToolSpecs(assignment.RoleID, assignment.RequiredCapabilities)
	allowedToolNames := toolSpecNames(availableTools)
	modelName := r.svc.v3SpecialistModel(r.claim.Job, r.routing, assignment.RoleID, assignment.RoleID, r.routing.Analyze)
	records := make([]subtaskToolRecord, 0, maxSubtaskToolCalls)
	sources := map[string]struct{}{}
	totalCalls := 0
	for turn := 1; turn <= maxSubtaskToolTurns; turn++ {
		contextPayload, err := r.subtaskToolContext(authoritativeDescription, authoritativeObjective)
		if err != nil {
			return "", nil, err
		}
		inputPayload := map[string]any{
			"subtask_id": assignment.ID,
			"kind":       assignment.Kind,
			"objective":  authoritativeDescription,
			"context":    contextPayload,
		}
		if err := spec.ValidateInputPayload(inputPayload); err != nil {
			return "", nil, err
		}
		prompt, err := r.buildSubtaskToolPrompt(spec, availableTools, assignment, authoritativeObjective, inputPayload, records)
		if err != nil {
			return "", nil, err
		}
		r.svc.emitStepEvent(r.claim.Step.ID, "specialist_dispatched", fmt.Sprintf("role=%s objective=%s tool_turn=%d", assignment.RoleID, assignment.ObjectiveID, turn))
		raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, fmt.Sprintf("%s_%d", subtaskToolScopePrefix, turn), modelName, prompt)
		if err != nil {
			return "", nil, err
		}
		decision, err := parseSubtaskToolDecision(raw, assignment.RoleID)
		if err != nil {
			records = append(records, rejectedSubtaskDecisionRecord(decision, err, assignment.RoleID, authoritativeDescription))
			r.svc.emitStepEvent(r.claim.Step.ID, "subtask_decision_rejected", fmt.Sprintf("role=%s turn=%d reason=%s next=direct_feedback", safeLine(assignment.RoleID, "unknown"), turn, safeLine(err.Error(), "invalid decision")))
			continue
		}
		if decision.Status == "blocked" || decision.Status == "fail" {
			if strings.TrimSpace(decision.Error) == "" {
				return "", nil, fmt.Errorf("specialist %s returned %s without an error", assignment.RoleID, decision.Status)
			}
			return "", nil, fmt.Errorf("specialist %s %s: %s", assignment.RoleID, decision.Status, decision.Error)
		}
		if len(decision.ToolCalls) == 0 {
			if decision.Status != "success" {
				return "", nil, fmt.Errorf("specialist %s returned status %q without tool calls", assignment.RoleID, decision.Status)
			}
			final := strings.TrimSpace(decision.Final)
			if final == "" {
				return "", nil, fmt.Errorf("tool decision contained neither tool calls nor final")
			}
			liveWorkspace, _ := contextPayload["current_workspace"].(string)
			if err := validateSubtaskCompletionEvidence(authoritativeObjective, records, liveWorkspace); err != nil {
				records = append(records, subtaskToolRecord{
					Call: artifacts.ToolCallArtifact{Tool: subtaskCompletionGateTool, Skill: assignment.RoleID, RequestedBy: authoritativeDescription},
					Result: artifacts.ToolResultArtifact{
						Tool: subtaskCompletionGateTool, Skill: assignment.RoleID, Accepted: false,
						Summary: err.Error(), Error: err.Error(),
					},
				})
				r.svc.emitStepEvent(r.claim.Step.ID, "subtask_completion_deferred", "role="+safeLine(assignment.RoleID, "unknown")+" reason="+safeLine(err.Error(), "missing evidence"))
				continue
			}
			if len(records) == 0 {
				for _, source := range r.inferSubtaskContextSources() {
					sources[source] = struct{}{}
				}
			}
			if spec.RequireEvidence && len(sources) == 0 {
				return "", nil, fmt.Errorf("specialist %s returned final output without evidence", assignment.RoleID)
			}
			if err := spec.ValidateOutputPayload(map[string]any{
				"summary":    final,
				"sources":    sortedSourceKeys(sources),
				"tool_calls": flattenToolRecords(records),
			}); err != nil {
				return "", nil, err
			}
			return final, sortedSourceKeys(sources), nil
		}
		if decision.Status != "continue" {
			return "", nil, fmt.Errorf("specialist %s returned tool calls with status %q", assignment.RoleID, decision.Status)
		}
		if len(decision.ToolCalls) > maxToolCallsPerTurn {
			return "", nil, fmt.Errorf("specialist %s exceeded %d tool calls in one turn", assignment.RoleID, maxToolCallsPerTurn)
		}
		rejectedCall := false
		for _, call := range decision.ToolCalls {
			if totalCalls >= maxSubtaskToolCalls {
				return "", nil, fmt.Errorf("specialist %s exceeded the %d-call tool budget", assignment.RoleID, maxSubtaskToolCalls)
			}
			record, err := r.executeSubtaskToolCall(spec, authoritativeDescription, allowedToolNames, call)
			records = append(records, record)
			totalCalls++
			if err != nil {
				if toolruntime.IsCallRejected(err) {
					r.svc.emitStepEvent(r.claim.Step.ID, "tool_correction_requested", fmt.Sprintf("tool=%s remaining_calls=%d", safeLine(call.Name, "unknown"), maxSubtaskToolCalls-totalCalls))
					rejectedCall = true
					break
				}
				return "", nil, err
			}
			if source := normalizeToolSource(record.Result.Tool); source != "" {
				sources[source] = struct{}{}
			}
		}
		if rejectedCall {
			continue
		}
	}
	return "", nil, fmt.Errorf("specialist %s exhausted its tool turns without an explicit final result", assignment.RoleID)
}

func (r *nativeRuntimeV3) executeSubtaskToolCall(spec specialists.Spec, objective string, allowedTools []string, call toolruntime.Call) (subtaskToolRecord, error) {
	record := subtaskToolRecord{
		Call: artifacts.ToolCallArtifact{
			Tool:        strings.TrimSpace(call.Name),
			Skill:       spec.ID,
			Input:       copyToolInput(call.Input),
			AllowedBy:   append([]string(nil), spec.AllowedTools...),
			Forbidden:   append([]string(nil), spec.ForbiddenTools...),
			RequestedBy: objective,
		},
		Result: artifacts.ToolResultArtifact{
			Tool:  strings.TrimSpace(call.Name),
			Skill: spec.ID,
		},
	}
	if !containsString(allowedTools, call.Name) {
		err := fmt.Errorf("tool %q is outside the subtask capability contract", call.Name)
		record.Result.Error = err.Error()
		record.Result.Summary = err.Error()
		callWriteErr := r.writeArtifact(artifacts.KindToolCall, record.Call)
		resultWriteErr := r.writeArtifact(artifacts.KindToolResult, record.Result)
		r.svc.emitStepEvent(r.claim.Step.ID, "tool_call_rejected", fmt.Sprintf("tool=%s reason=outside_subtask_capability_contract", safeLine(call.Name, "unknown")))
		return record, errors.Join(err, callWriteErr, resultWriteErr)
	}
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, spec.ID, call)
	if err != nil {
		if toolruntime.IsCallRejected(err) {
			record.Call.Allowed = true
		}
		record.Result.Accepted = false
		record.Result.Error = err.Error()
		record.Result.Summary = strings.TrimSpace(err.Error())
		return record, err
	}
	record.Call.Allowed = true
	record.Result.Tool = result.Tool
	record.Result.Accepted = result.Accepted
	record.Result.Summary = result.Summary
	record.Result.Output = copyToolInput(result.Output)
	record.Result.Warnings = append([]string(nil), result.Warnings...)
	return record, nil
}
