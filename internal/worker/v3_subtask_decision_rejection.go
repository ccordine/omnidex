package worker

import (
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

const subtaskDecisionContractTool = "decision.contract"

func rejectedSubtaskDecisionRecord(decision subtaskToolDecision, decisionErr error, roleID, objective string) subtaskToolRecord {
	toolNames := make([]string, 0, len(decision.ToolCalls))
	for _, call := range decision.ToolCalls {
		if name := strings.TrimSpace(call.Name); name != "" {
			toolNames = append(toolNames, name)
		}
	}
	details := map[string]any{
		"status":          strings.TrimSpace(decision.Status),
		"tool_call_count": len(decision.ToolCalls),
		"tool_names":      toolNames,
	}
	reason := "invalid subtask decision"
	if decisionErr != nil && strings.TrimSpace(decisionErr.Error()) != "" {
		reason = strings.TrimSpace(decisionErr.Error())
	}
	return subtaskToolRecord{
		Call: artifacts.ToolCallArtifact{
			Tool:        subtaskDecisionContractTool,
			Skill:       strings.TrimSpace(roleID),
			Input:       details,
			RequestedBy: strings.TrimSpace(objective),
		},
		Result: artifacts.ToolResultArtifact{
			Tool:     subtaskDecisionContractTool,
			Skill:    strings.TrimSpace(roleID),
			Accepted: false,
			Summary:  reason,
			Output:   details,
			Error:    reason,
		},
	}
}

func rejectedDecisionToolNames(record subtaskToolRecord) []string {
	values, ok := record.Result.Output["tool_names"]
	if !ok {
		return nil
	}
	switch names := values.(type) {
	case []string:
		return append([]string(nil), names...)
	case []any:
		out := make([]string, 0, len(names))
		for _, value := range names {
			if name, ok := value.(string); ok && strings.TrimSpace(name) != "" {
				out = append(out, strings.TrimSpace(name))
			}
		}
		return out
	default:
		return nil
	}
}
