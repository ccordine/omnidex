package worker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/specialists"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

const (
	maxSubtaskToolTurns    = 3
	maxSubtaskToolCalls    = 4
	maxToolCallsPerTurn    = 2
	subtaskToolScopePrefix = "v3_subtask_tool"
)

type subtaskToolDecision struct {
	ToolCalls []toolruntime.Call `json:"tool_calls,omitempty"`
	Final     string             `json:"final,omitempty"`
	Sources   []string           `json:"sources,omitempty"`
}

type subtaskToolRecord struct {
	Call   artifacts.ToolCallArtifact
	Result artifacts.ToolResultArtifact
}

func (r *nativeRuntimeV3) runSubtaskWithTools() (string, []string, error) {
	if r == nil || r.svc == nil || r.svc.v3Tools == nil {
		return "", nil, fmt.Errorf("tool runtime unavailable")
	}
	spec, ok := r.svc.skillSpec("subtask_executor")
	if !ok {
		return "", nil, fmt.Errorf("subtask_executor skill missing")
	}
	subtaskID := strings.TrimSpace(r.contexts["subtask_id"])
	kind := strings.TrimSpace(r.contexts["subtask_kind"])
	objective := strings.TrimSpace(r.contexts["subtask_objective"])
	if objective == "" {
		objective = "execute delegated subtask"
	}
	inputPayload := map[string]any{
		"subtask_id": subtaskID,
		"kind":       kind,
		"objective":  objective,
		"context":    r.subtaskToolContext(objective),
	}
	if err := spec.ValidateInputPayload(inputPayload); err != nil {
		return "", nil, err
	}
	availableTools := r.availableToolSpecs("subtask_executor")
	if len(availableTools) == 0 {
		return "", nil, fmt.Errorf("no concrete tools available for %s", spec.ID)
	}
	modelName := r.svc.skillPreferredModel("subtask_executor", r.svc.specialistModel(r.claim.Job, specialist.RoleAnalysisSpecialist, r.svc.models.Fast))
	records := make([]subtaskToolRecord, 0, maxSubtaskToolCalls)
	sources := map[string]struct{}{}
	totalCalls := 0
	for turn := 1; turn <= maxSubtaskToolTurns; turn++ {
		raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, fmt.Sprintf("%s_%d", subtaskToolScopePrefix, turn), modelName, r.buildSubtaskToolPrompt(spec, availableTools, objective, records))
		if err != nil {
			return "", nil, err
		}
		decision, ok := parseSubtaskToolDecision(raw)
		if !ok {
			if final := strings.TrimSpace(raw); final != "" && !genericNonAnswer(final) {
				return final, r.inferSubtaskContextSources(), nil
			}
			return "", nil, fmt.Errorf("tool decision parse failed")
		}
		if len(decision.ToolCalls) == 0 {
			final := strings.TrimSpace(decision.Final)
			if final == "" {
				return "", nil, fmt.Errorf("tool decision contained neither tool calls nor final")
			}
			for _, source := range decision.Sources {
				if clean := normalizeToolSource(source); clean != "" {
					sources[clean] = struct{}{}
				}
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
		toolCalls := decision.ToolCalls
		if len(toolCalls) > maxToolCallsPerTurn {
			toolCalls = toolCalls[:maxToolCallsPerTurn]
		}
		for _, call := range toolCalls {
			if totalCalls >= maxSubtaskToolCalls {
				break
			}
			record, err := r.executeSubtaskToolCall(spec, objective, call)
			records = append(records, record)
			if err != nil {
				return "", nil, err
			}
			totalCalls++
			if source := normalizeToolSource(record.Result.Tool); source != "" {
				sources[source] = struct{}{}
			}
		}
		if totalCalls >= maxSubtaskToolCalls {
			break
		}
	}
	if len(records) == 0 {
		return "", nil, fmt.Errorf("tool loop produced no usable output")
	}
	final := summarizeSubtaskToolRecords(records)
	if err := spec.ValidateOutputPayload(map[string]any{
		"summary":    final,
		"sources":    sortedSourceKeys(sources),
		"tool_calls": flattenToolRecords(records),
	}); err != nil {
		return "", nil, err
	}
	return final, sortedSourceKeys(sources), nil
}

func (r *nativeRuntimeV3) executeSubtaskToolCall(spec specialists.Spec, objective string, call toolruntime.Call) (subtaskToolRecord, error) {
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
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, spec.ID, call)
	if err != nil {
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

func (r *nativeRuntimeV3) buildSubtaskToolPrompt(spec specialists.Spec, definitions []toolruntime.Spec, objective string, records []subtaskToolRecord) string {
	remaining := maxSubtaskToolCalls - len(records)
	if remaining < 0 {
		remaining = 0
	}
	return strings.Join([]string{
		"You are the Omnidex v3 subtask executor.",
		strings.TrimSpace(spec.Instructions),
		`Return JSON only with schema: {"tool_calls":[{"name":"tool.name","input":{}}],"final":"...","sources":["workspace|memory|web_search"]}`,
		"Use tools when you need evidence. Only return `final` when the current tool results are sufficient.",
		"Do not invent tool names or parameters. Match the input schema exactly.",
		fmt.Sprintf("Maximum tool calls remaining in this run: %d.", remaining),
		promptBlock("User Instruction", r.claim.Job.Instruction),
		promptBlock("Subtask Objective", objective),
		promptBlock("Context", r.subtaskToolContext(objective)),
		promptBlock("Available Tools", marshalToolSpecs(definitions)),
		promptBlock("Previous Tool Results", marshalToolRecords(records)),
	}, "\n\n")
}

func (r *nativeRuntimeV3) subtaskToolContext(objective string) string {
	workspaceArtifact, _ := r.readWorkspaceArtifact()
	retrievalArtifact, _ := r.readRetrievalArtifact()
	webArtifact, _ := r.readWebArtifact()
	sections := []string{
		promptBlock("Job Instruction", r.claim.Job.Instruction),
		promptBlock("Objective", objective),
	}
	if strings.TrimSpace(workspaceArtifact.Summary) != "" {
		sections = append(sections, promptBlock("Workspace Artifact", trimForBudget(workspaceArtifact.Summary, 1800)))
	}
	if strings.TrimSpace(retrievalArtifact.Summary) != "" {
		sections = append(sections, promptBlock("Retrieval Artifact", trimForBudget(retrievalArtifact.Summary, 1400)))
	}
	if strings.TrimSpace(webArtifact.Summary) != "" {
		sections = append(sections, promptBlock("Existing Web Evidence", trimForBudget(webArtifact.Summary, 1400)))
	}
	return strings.Join(sections, "\n\n")
}

func (r *nativeRuntimeV3) inferSubtaskContextSources() []string {
	sources := map[string]struct{}{}
	workspaceArtifact, _ := r.readWorkspaceArtifact()
	retrievalArtifact, _ := r.readRetrievalArtifact()
	webArtifact, _ := r.readWebArtifact()
	if strings.TrimSpace(workspaceArtifact.Summary) != "" {
		sources["workspace"] = struct{}{}
	}
	if strings.TrimSpace(retrievalArtifact.Summary) != "" {
		sources["memory"] = struct{}{}
	}
	if strings.TrimSpace(webArtifact.Summary) != "" && !strings.EqualFold(strings.TrimSpace(webArtifact.Summary), "external research skipped") {
		sources["web_search"] = struct{}{}
	}
	return sortedSourceKeys(sources)
}

func (r *nativeRuntimeV3) availableToolSpecs(skillID string) []toolruntime.Spec {
	spec, ok := r.svc.skillSpec(skillID)
	if !ok || r.svc.v3Tools == nil {
		return nil
	}
	return r.svc.v3Tools.SpecsFor(toolruntime.ExecuteOptions{
		Allowed:       append([]string(nil), spec.AllowedTools...),
		Forbidden:     append([]string(nil), spec.ForbiddenTools...),
		RequireListed: true,
	})
}

func (r *nativeRuntimeV3) availableToolNames(skillID string) []string {
	specs := r.availableToolSpecs(skillID)
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.Name)
	}
	sort.Strings(out)
	return out
}

func parseSubtaskToolDecision(raw string) (subtaskToolDecision, bool) {
	raw = bestEffortJSONObject(raw)
	if raw == "" {
		return subtaskToolDecision{}, false
	}
	var decision subtaskToolDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return subtaskToolDecision{}, false
	}
	return decision, true
}

func marshalToolSpecs(specs []toolruntime.Spec) string {
	if len(specs) == 0 {
		return "[]"
	}
	return marshalPrettyJSON(specs)
}

func marshalToolRecords(records []subtaskToolRecord) string {
	if len(records) == 0 {
		return "[]"
	}
	return marshalPrettyJSON(flattenToolRecords(records))
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

func summarizeSubtaskToolRecords(records []subtaskToolRecord) string {
	parts := make([]string, 0, len(records))
	for _, record := range records {
		if record.Result.Accepted && strings.TrimSpace(record.Result.Summary) != "" {
			parts = append(parts, fmt.Sprintf("[%s] %s", record.Result.Tool, strings.TrimSpace(record.Result.Summary)))
			continue
		}
		if strings.TrimSpace(record.Result.Error) != "" {
			parts = append(parts, fmt.Sprintf("[%s] error: %s", record.Result.Tool, strings.TrimSpace(record.Result.Error)))
		}
	}
	return strings.Join(parts, "\n")
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

func marshalPrettyJSON(value any) string {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(raw)
}
