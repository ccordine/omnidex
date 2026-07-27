package worker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

const maxPromptToolRecords = 3

func parseSubtaskToolDecision(raw, expectedRole string) (subtaskToolDecision, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var decision subtaskToolDecision
	if err := decoder.Decode(&decision); err != nil {
		return subtaskToolDecision{}, fmt.Errorf("decode subtask tool decision: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return subtaskToolDecision{}, fmt.Errorf("decode subtask tool decision: %w", err)
	}
	if strings.TrimSpace(decision.RoleID) != strings.TrimSpace(expectedRole) {
		return decision, fmt.Errorf("subtask specialist role drift: expected %q, received %q", expectedRole, decision.RoleID)
	}
	switch decision.Status {
	case "continue":
		if len(decision.ToolCalls) != 1 {
			return decision, fmt.Errorf("subtask specialist %s must return exactly one tool call before receiving fresh state and feedback", expectedRole)
		}
		if strings.TrimSpace(decision.Final) != "" || strings.TrimSpace(decision.Error) != "" {
			return decision, fmt.Errorf("subtask specialist %s continue response requires empty final and error fields", expectedRole)
		}
	case "success":
		if len(decision.ToolCalls) != 0 || strings.TrimSpace(decision.Final) == "" || strings.TrimSpace(decision.Error) != "" {
			return decision, fmt.Errorf("subtask specialist %s success response requires no tool calls, a concrete final result, and an empty error", expectedRole)
		}
	case "blocked", "fail":
		if len(decision.ToolCalls) != 0 || strings.TrimSpace(decision.Final) != "" || strings.TrimSpace(decision.Error) == "" {
			return decision, fmt.Errorf("subtask specialist %s %s response requires no tool calls, an empty final result, and an explicit error", expectedRole, decision.Status)
		}
	default:
		return decision, fmt.Errorf("subtask specialist %s returned invalid status %q", expectedRole, decision.Status)
	}
	return decision, nil
}

func marshalToolSpecs(specs []toolruntime.Spec) (string, error) {
	if len(specs) == 0 {
		return "[]", nil
	}
	return marshalPrettyJSON(specs)
}

func marshalToolRecords(records []subtaskToolRecord) (string, error) {
	if len(records) == 0 {
		return "[]", nil
	}
	return marshalPrettyJSON(compactPromptToolRecords(records))
}

func compactPromptToolRecords(records []subtaskToolRecord) []map[string]any {
	if len(records) > maxPromptToolRecords {
		records = records[len(records)-maxPromptToolRecords:]
	}
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		item := map[string]any{
			"tool":     record.Result.Tool,
			"accepted": record.Result.Accepted,
			"summary":  record.Result.Summary,
			"output":   compactPromptToolOutput(record.Result),
			"warnings": record.Result.Warnings,
			"error":    record.Result.Error,
			"attempts": 1,
		}
		if len(out) > 0 && !record.Result.Accepted {
			previous := out[len(out)-1]
			if previous["accepted"] == false && previous["tool"] == item["tool"] && previous["error"] == item["error"] {
				previous["attempts"] = previous["attempts"].(int) + 1
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func compactPromptToolOutput(result artifacts.ToolResultArtifact) map[string]any {
	if len(result.Output) == 0 {
		return map[string]any{}
	}
	out := copyToolInput(result.Output)
	for _, key := range []string{"stdout", "stderr"} {
		if text, ok := out[key].(string); ok {
			out[key] = trimForBudget(text, 2000)
		}
	}
	return out
}

func flattenToolRecords(records []subtaskToolRecord) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"tool":     record.Result.Tool,
			"input":    record.Call.Input,
			"accepted": record.Result.Accepted,
			"summary":  record.Result.Summary,
			"output":   record.Result.Output,
			"warnings": record.Result.Warnings,
			"error":    record.Result.Error,
		})
	}
	return out
}

func sortedSourceKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		if clean := strings.TrimSpace(value); clean != "" {
			out = append(out, clean)
		}
	}
	sort.Strings(out)
	return out
}

func normalizeToolSource(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	if clean == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(clean, "workspace."):
		return "workspace"
	case strings.HasPrefix(clean, "memory."):
		return "memory"
	case strings.HasPrefix(clean, "web."):
		return "web_search"
	case strings.HasPrefix(clean, "evidence."):
		return "evidence_store"
	case strings.HasPrefix(clean, "tool."):
		return "tool_registry"
	default:
		return clean
	}
}

func marshalPrettyJSON(value any) (string, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal v3 tool context: %w", err)
	}
	return string(raw), nil
}
