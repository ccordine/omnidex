package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	toolruntime "github.com/gryph/omnidex/internal/tools"
	"strings"
)

func schemaMap(schema toolruntime.Schema) map[string]any {
	value := map[string]any{}
	if strings.TrimSpace(schema.Type) != "" {
		value["type"] = schema.Type
	}
	if strings.TrimSpace(schema.Description) != "" {
		value["description"] = schema.Description
	}
	if len(schema.Required) > 0 {
		value["required"] = append([]string(nil), schema.Required...)
	}
	if len(schema.Enum) > 0 {
		value["enum"] = append([]string(nil), schema.Enum...)
	}
	if schema.AdditionalProperties {
		value["additional_properties"] = true
	}
	if len(schema.Properties) > 0 {
		props := make(map[string]any, len(schema.Properties))
		for key, property := range schema.Properties {
			props[key] = schemaMap(property)
		}
		value["properties"] = props
	}
	if schema.Items != nil {
		value["items"] = schemaMap(*schema.Items)
	}
	return value
}

func defaultV3SkillForAction(action string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(action)), "v3_") {
	case "workspace_research":
		return "workspace_researcher"
	case "memory_retrieval":
		return "memory_retriever"
	case "external_research":
		return "web_researcher"
	case "analysis":
		return "analysis_specialist"
	case "response_draft":
		return "response_composer"
	case "verification":
		return "verifier"
	case "memory_review":
		return "memory_reviewer"
	case "planning":
		return "executive_planner"
	case "intent_parse":
		return "prompt_interpreter"
	case "capability_audit":
		return "capability_auditor"
	case "subtask":
		return "subtask_executor"
	case "coding":
		return "subtask_executor"
	default:
		return ""
	}
}

func (s *Service) executeV3Tool(ctx context.Context, claim *model.ClaimedStep, skillID string, call toolruntime.Call) (toolruntime.Result, error) {
	if s == nil || s.v3Tools == nil {
		return toolruntime.Result{}, fmt.Errorf("v3 tool registry unavailable")
	}
	if claim == nil {
		return toolruntime.Result{}, fmt.Errorf("v3 tool execution requires a claimed step")
	}
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		skillID = defaultV3SkillForAction(claim.Step.Action)
	}
	allowed := []string(nil)
	forbidden := []string(nil)
	requireListed := false
	if spec, ok := s.skillSpec(skillID); ok {
		allowed = append([]string(nil), spec.AllowedTools...)
		forbidden = append([]string(nil), spec.ForbiddenTools...)
		requireListed = true
	}
	callArtifact := artifacts.ToolCallArtifact{
		Tool:        call.Name,
		Skill:       skillID,
		Input:       copyToolInput(call.Input),
		AllowedBy:   append([]string(nil), allowed...),
		Forbidden:   append([]string(nil), forbidden...),
		RequestedBy: claim.Step.Action,
	}
	s.emitStepEvent(claim.Authority, "tool_call_begin", fmt.Sprintf("tool=%s skill=%s", safeLine(call.Name, "unknown"), safeLine(skillID, "unknown")))

	executionCtx := ctx
	var bindErr error
	if v3ToolRequiresWorkspaceScope(call.Name) {
		scope, err := s.workspaceScopeForV3Job(claim.Job)
		if err != nil {
			bindErr = err
		} else {
			executionCtx, bindErr = withV3WorkspaceScope(ctx, scope)
			if bindErr == nil {
				s.emitStepEvent(claim.Authority, "workspace_scope_bound", fmt.Sprintf("source=%s root=%s", scope.Source, safeLine(scope.Root, "unknown")))
			}
		}
	}
	if bindErr == nil && v3ToolRequiresMemoryAuthority(call.Name) {
		authority, err := s.memoryAuthorityForV3Job(ctx, claim.Job)
		if err != nil {
			bindErr = err
		} else {
			executionCtx, bindErr = withV3MemoryAuthority(executionCtx, authority)
			if bindErr == nil {
				s.emitStepEvent(claim.Authority, "memory_authority_bound", fmt.Sprintf("mode=%s project_scope=%s session_scope=%s", authority.Intent.MemoryMode, safeLine(authority.ProjectScope, "global"), safeLine(authority.SessionScope, "none")))
			}
		}
	}
	stopHeartbeat := s.startProgressHeartbeat(executionCtx, claim.Authority, "tool:"+safeLine(call.Name, "unknown"))
	var result toolruntime.Result
	err := bindErr
	if err == nil {
		result, err = s.v3Tools.Execute(executionCtx, call, toolruntime.ExecuteOptions{
			Allowed:       allowed,
			Forbidden:     forbidden,
			RequireListed: requireListed,
		})
		if err == nil {
			err = executionCtx.Err()
		}
	}
	stopHeartbeat()
	if err != nil {
		callArtifact.Allowed = toolruntime.IsCallRejected(err)
		callEnvelope, marshalErr := marshalStepArtifact(claim, artifacts.KindToolCall, callArtifact)
		if marshalErr == nil {
			marshalErr = s.repo.WriteArtifact(ctx, claim.Authority, callEnvelope)
		}
		resultEnvelope, resultMarshalErr := marshalStepArtifact(claim, artifacts.KindToolResult, artifacts.ToolResultArtifact{
			Tool:     call.Name,
			Skill:    skillID,
			Accepted: false,
			Summary:  strings.TrimSpace(err.Error()),
			Error:    strings.TrimSpace(err.Error()),
		})
		if resultMarshalErr == nil {
			resultMarshalErr = s.repo.WriteArtifact(ctx, claim.Authority, resultEnvelope)
		}
		s.emitStepEvent(claim.Authority, "tool_call_rejected", fmt.Sprintf("tool=%s recoverable=%t reason=%s", safeLine(call.Name, "unknown"), toolruntime.IsCallRejected(err), safeLine(err.Error(), "rejected")))
		if persistErr := errors.Join(marshalErr, resultMarshalErr); persistErr != nil {
			return toolruntime.Result{}, fmt.Errorf("persist rejected tool call %q after %v: %w", call.Name, err, persistErr)
		}
		return toolruntime.Result{}, err
	}
	callArtifact.Allowed = true
	callEnvelope, err := marshalStepArtifact(claim, artifacts.KindToolCall, callArtifact)
	if err != nil {
		return toolruntime.Result{}, err
	}
	if err := s.repo.WriteArtifact(ctx, claim.Authority, callEnvelope); err != nil {
		return toolruntime.Result{}, err
	}

	for _, record := range result.Evidence {
		record.JobID = claim.Job.ID
		record.StepID = claim.Step.ID
		if strings.TrimSpace(record.ToolName) == "" {
			record.ToolName = result.Tool
		}
		record.Metadata = v3EvidenceMetadata(record.Metadata, claim, skillID)
		if err := s.repo.WriteEvidence(ctx, claim.Authority, record); err != nil {
			return toolruntime.Result{}, err
		}
	}

	resultArtifact := artifacts.ToolResultArtifact{
		Tool:     result.Tool,
		Skill:    skillID,
		Accepted: result.Accepted,
		Summary:  result.Summary,
		Output:   copyToolInput(result.Output),
		Warnings: append([]string(nil), result.Warnings...),
	}
	resultEnvelope, err := marshalStepArtifact(claim, artifacts.KindToolResult, resultArtifact)
	if err != nil {
		return toolruntime.Result{}, err
	}
	if err := s.repo.WriteArtifact(ctx, claim.Authority, resultEnvelope); err != nil {
		return toolruntime.Result{}, err
	}
	s.emitStepEvent(claim.Authority, "tool_call_complete", fmt.Sprintf("tool=%s accepted=%t", safeLine(result.Tool, call.Name), result.Accepted))
	return result, nil
}

func marshalStepArtifact(claim *model.ClaimedStep, kind string, payload any) (artifacts.Envelope, error) {
	if claim == nil {
		return artifacts.Envelope{}, fmt.Errorf("marshal %s artifact: claimed step is required", kind)
	}
	env, err := artifacts.MarshalPayload(kind, "1", payload)
	if err != nil {
		return artifacts.Envelope{}, err
	}
	env.JobID = claim.Job.ID
	env.StepID = claim.Step.ID
	return env, nil
}

func v3EvidenceMetadata(existing map[string]any, claim *model.ClaimedStep, skillID string) map[string]any {
	metadata := make(map[string]any, len(existing)+5)
	for key, value := range existing {
		metadata[key] = value
	}
	metadata["specialist_id"] = strings.TrimSpace(skillID)
	metadata["step_action"] = strings.TrimSpace(claim.Step.Action)
	for _, item := range claim.Contexts {
		switch strings.TrimSpace(item.Key) {
		case "subtask_id", "subtask_objective_id", "subtask_role_id":
			metadata[strings.TrimSpace(item.Key)] = strings.TrimSpace(item.Value)
		}
	}
	return metadata
}

func copyToolInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
