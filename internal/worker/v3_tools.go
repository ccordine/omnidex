package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	toolruntime "github.com/gryph/omnidex/internal/tools"
	"github.com/gryph/omnidex/internal/websearch"
)

func newV3ToolRegistry(s *Service) *toolruntime.Registry {
	registry := toolruntime.NewRegistry()

	mustRegisterTool(registry, toolruntime.Spec{
		Name:        "tool.registry",
		Description: "List the exact v3 tools available in this runtime, including names, aliases, descriptions, and schemas. Use this when you need to confirm the callable tool surface instead of guessing.",
		Aliases:     []string{"tool_registry"},
		InputSchema: toolruntime.Schema{
			Type: "object",
			Properties: map[string]toolruntime.Schema{
				"names": {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"summary", "tools"},
			Properties: map[string]toolruntime.Schema{
				"summary": {Type: "string"},
				"tools": {
					Type: "array",
					Items: &toolruntime.Schema{
						Type:     "object",
						Required: []string{"name", "description", "input_schema"},
						Properties: map[string]toolruntime.Schema{
							"name":         {Type: "string"},
							"description":  {Type: "string"},
							"aliases":      {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
							"input_schema": {Type: "object", AdditionalProperties: true},
						},
					},
				},
			},
		},
		Examples: []toolruntime.Example{
			{When: "You need the exact tool names and schemas before choosing one.", Input: map[string]any{}},
			{When: "You only need details for specific tools.", Input: map[string]any{"names": []string{"workspace.research", "web.search"}}},
		},
	}, func(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
		_ = ctx
		filter := map[string]struct{}{}
		for _, name := range parseAnyStringSlice(call.Input["names"]) {
			filter[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
		}
		tools := make([]map[string]any, 0, len(registry.Names()))
		for _, name := range registry.Names() {
			spec, ok := registry.Spec(name)
			if !ok {
				continue
			}
			if len(filter) > 0 {
				if _, ok := filter[strings.ToLower(strings.TrimSpace(spec.Name))]; !ok {
					continue
				}
			}
			tools = append(tools, map[string]any{
				"name":         spec.Name,
				"description":  spec.Description,
				"aliases":      append([]string(nil), spec.Aliases...),
				"input_schema": schemaMap(spec.InputSchema),
			})
		}
		return toolruntime.Result{
			Summary: fmt.Sprintf("registered_tools=%d", len(tools)),
			Output: map[string]any{
				"summary": fmt.Sprintf("registered_tools=%d", len(tools)),
				"tools":   tools,
			},
		}, nil
	})

	mustRegisterTool(registry, toolruntime.Spec{
		Name:        "workspace.research",
		Description: "Research the local workspace and return grounded file excerpts.",
		Aliases:     []string{"workspace"},
		InputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]toolruntime.Schema{
				"query":        {Type: "string"},
				"max_excerpts": {Type: "integer"},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"summary", "files_considered", "relevant_files"},
			Properties: map[string]toolruntime.Schema{
				"root":            {Type: "string"},
				"files_considered": {Type: "integer"},
				"languages":       {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
				"summary":         {Type: "string"},
				"missing_context": {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
				"relevant_files": {
					Type: "array",
					Items: &toolruntime.Schema{
						Type:     "object",
						Required: []string{"path"},
						Properties: map[string]toolruntime.Schema{
							"path":     {Type: "string"},
							"reason":   {Type: "string"},
							"excerpt":  {Type: "string"},
							"score":    {Type: "number"},
							"language": {Type: "string"},
							"symbols":  {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
						},
					},
				},
			},
		},
		Examples: []toolruntime.Example{
			{When: "You need code evidence from the local repository.", Input: map[string]any{"query": "find the native v3 runtime execution path", "max_excerpts": 4}},
		},
		RequireEvidence: true,
	}, func(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
		if s.workspace == nil || !s.workspace.Enabled() {
			return toolruntime.Result{}, fmt.Errorf("workspace service disabled")
		}
		query := strings.TrimSpace(toolInputString(call.Input, "query"))
		if query == "" {
			return toolruntime.Result{}, fmt.Errorf("workspace.research query is required")
		}
		research, err := s.workspace.Research(query)
		if err != nil {
			return toolruntime.Result{}, err
		}
		maxExcerpts := toolInputInt(call.Input, "max_excerpts", 0)
		excerpts := research.Excerpts
		if maxExcerpts > 0 && maxExcerpts < len(excerpts) {
			excerpts = excerpts[:maxExcerpts]
		}
		relevantFiles := make([]map[string]any, 0, len(excerpts))
		evidenceRecords := make([]evidence.Record, 0, len(excerpts))
		for _, excerpt := range excerpts {
			relevantFiles = append(relevantFiles, map[string]any{
				"path":     excerpt.Path,
				"reason":   excerpt.Reason,
				"excerpt":  excerpt.Excerpt,
				"score":    excerpt.Score,
				"language": excerpt.Language,
				"symbols":  append([]string(nil), excerpt.Symbols...),
			})
			evidenceRecords = append(evidenceRecords, evidence.Record{
				Kind:       evidence.KindFileExcerpt,
				SourceType: "workspace",
				SourceRef:  excerpt.Path,
				FilePaths:  []string{excerpt.Path},
				Excerpt:    excerpt.Excerpt,
				Summary:    excerpt.Reason,
				Confidence: excerpt.Score,
				Metadata: map[string]any{
					"language": excerpt.Language,
					"symbols":  excerpt.Symbols,
				},
			})
		}
		summary := strings.TrimSpace(research.Summary)
		if summary == "" {
			summary = "Workspace research completed."
		}
		return toolruntime.Result{
			Summary: summary,
			Output: map[string]any{
				"root":             research.Root,
				"files_considered": research.FilesConsidered,
				"languages":        append([]string(nil), research.Languages...),
				"summary":          summary,
				"missing_context":  []string{},
				"relevant_files":   relevantFiles,
			},
			Warnings: toolWarnings(len(relevantFiles) == 0, "no relevant workspace files matched the query"),
			Evidence: evidenceRecords,
		}, nil
	})

	mustRegisterTool(registry, toolruntime.Spec{
		Name:        "memory.retrieve",
		Description: "Retrieve and rank relevant memory for the current job context.",
		Aliases:     []string{"memory"},
		InputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]toolruntime.Schema{
				"query":       {Type: "string"},
				"limit":       {Type: "integer"},
				"scope_tags":  {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
				"project_tag": {Type: "string"},
				"session_tag": {Type: "string"},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"summary", "items"},
			Properties: map[string]toolruntime.Schema{
				"summary": {Type: "string"},
				"items": {
					Type: "array",
					Items: &toolruntime.Schema{
						Type:     "object",
						Required: []string{"id", "kind", "content", "tags", "score"},
						Properties: map[string]toolruntime.Schema{
							"id":      {Type: "integer"},
							"kind":    {Type: "string"},
							"content": {Type: "string"},
							"tags":    {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
							"score":   {Type: "number"},
						},
					},
				},
			},
		},
		Examples: []toolruntime.Example{
			{When: "You need prior project or session memory relevant to the current job.", Input: map[string]any{"query": "previous approved architecture decisions", "limit": 5}},
		},
		RequireEvidence: true,
	}, func(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
		query := strings.TrimSpace(toolInputString(call.Input, "query"))
		if query == "" {
			return toolruntime.Result{}, fmt.Errorf("memory.retrieve query is required")
		}
		limit := toolInputInt(call.Input, "limit", s.retrievalLimit)
		if limit <= 0 {
			limit = s.retrievalLimit
		}
		if limit > maxMemoryRetrievalLimit {
			limit = maxMemoryRetrievalLimit
		}
		scopeTags := parseAnyStringSlice(call.Input["scope_tags"])
		projectScope := strings.TrimSpace(toolInputString(call.Input, "project_tag"))
		sessionTag := strings.TrimSpace(toolInputString(call.Input, "session_tag"))

		var embedding []float64
		vector, err := s.llm.Embedding(ctx, query)
		if err == nil {
			embedding = vector
		}
		matches, err := s.repo.FindRelevantMemory(ctx, embedding, scopeTags, limit)
		if err != nil && len(embedding) > 0 {
			matches, err = s.repo.FindRelevantMemory(ctx, nil, scopeTags, limit)
		}
		if err != nil {
			return toolruntime.Result{}, err
		}
		ranked := rankMemoryOmnibusMatches(matches, query, scopeTags, projectScope, sessionTag, limit, nowUTC())
		summary := strings.TrimSpace(buildRetrievalContext(ranked, s.contextBudget))
		if summary == "" {
			summary = "No relevant memory matched for this step."
		}

		items := make([]map[string]any, 0, len(ranked))
		evidenceRecords := make([]evidence.Record, 0, minInt(len(ranked), 8))
		for idx, match := range ranked {
			items = append(items, map[string]any{
				"id":      match.ID,
				"kind":    match.Kind,
				"content": match.Content,
				"tags":    append([]string(nil), match.Tags...),
				"score":   match.Score,
			})
			if idx >= 8 {
				continue
			}
			evidenceRecords = append(evidenceRecords, evidence.Record{
				Kind:       evidence.KindMemoryExcerpt,
				SourceType: "memory",
				SourceRef:  fmt.Sprintf("memory:%d", match.ID),
				Excerpt:    trimForBudget(match.Content, 800),
				Summary:    match.Kind,
				Confidence: match.Score,
				Metadata: map[string]any{
					"tags": match.Tags,
				},
			})
		}
		return toolruntime.Result{
			Summary: summary,
			Output: map[string]any{
				"summary": summary,
				"items":   items,
			},
			Warnings: toolWarnings(len(items) == 0, "no relevant memory matched the query"),
			Evidence: evidenceRecords,
		}, nil
	})

	mustRegisterTool(registry, toolruntime.Spec{
		Name:        "web.search",
		Description: "Search the web, fetch source pages, and return source-grounded documents.",
		Aliases:     []string{"web_search"},
		InputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]toolruntime.Schema{
				"query": {Type: "string"},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"query", "summary", "documents"},
			Properties: map[string]toolruntime.Schema{
				"query":   {Type: "string"},
				"summary": {Type: "string"},
				"documents": {
					Type: "array",
					Items: &toolruntime.Schema{
						Type:     "object",
						Required: []string{"provider", "url", "content"},
						Properties: map[string]toolruntime.Schema{
							"provider":   {Type: "string"},
							"search_url": {Type: "string"},
							"url":        {Type: "string"},
							"title":      {Type: "string"},
							"snippet":    {Type: "string"},
							"content":    {Type: "string"},
						},
					},
				},
			},
		},
		Examples: []toolruntime.Example{
			{When: "The task depends on current external information.", Input: map[string]any{"query": "Anthropic tool use overview"}},
		},
		RequireEvidence: true,
	}, func(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
		if s.webSearch == nil {
			return toolruntime.Result{}, fmt.Errorf("web search service disabled")
		}
		query := strings.TrimSpace(toolInputString(call.Input, "query"))
		if query == "" {
			return toolruntime.Result{}, fmt.Errorf("web.search query is required")
		}
		results, err := s.webSearch.SearchAll(ctx, query)
		if err != nil {
			return toolruntime.Result{}, err
		}
		documents := make([]map[string]any, 0, len(results))
		evidenceRecords := make([]evidence.Record, 0, len(results))
		for _, result := range results {
			documents = append(documents, map[string]any{
				"provider":   result.Provider,
				"search_url": result.SearchURL,
				"url":        result.URL,
				"title":      result.Title,
				"snippet":    result.Snippet,
				"content":    result.Content,
			})
			evidenceRecords = append(evidenceRecords, evidence.Record{
				Kind:       evidence.KindWebPage,
				SourceType: result.Provider,
				SourceRef:  result.URL,
				Summary:    safeLine(result.Title, result.URL),
				Excerpt:    trimForBudget(result.Content, 1200),
				Confidence: 0.8,
				Metadata: map[string]any{
					"search_url": result.SearchURL,
					"snippet":    result.Snippet,
				},
			})
		}
		summary := strings.TrimSpace(websearch.BuildContext(results, s.contextBudget))
		if summary == "" {
			summary = "No usable web documents were retrieved."
		}
		return toolruntime.Result{
			Summary: summary,
			Output: map[string]any{
				"query":     query,
				"summary":   summary,
				"documents": documents,
			},
			Warnings: toolWarnings(len(documents) == 0, "no usable web documents were retrieved"),
			Evidence: evidenceRecords,
		}, nil
	})

	mustRegisterTool(registry, toolruntime.Spec{
		Name:        "evidence.inspect",
		Description: "Inspect evidence already captured for the current job.",
		Aliases:     []string{"evidence_store", "evidence"},
		InputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"job_id"},
			Properties: map[string]toolruntime.Schema{
				"job_id": {Type: "integer"},
				"limit":  {Type: "integer"},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"summary", "records"},
			Properties: map[string]toolruntime.Schema{
				"summary": {Type: "string"},
				"records": {
					Type: "array",
					Items: &toolruntime.Schema{
						Type:     "object",
						Required: []string{"id", "kind", "source_type", "source_ref"},
						Properties: map[string]toolruntime.Schema{
							"id":              {Type: "integer"},
							"kind":            {Type: "string"},
							"source_type":     {Type: "string"},
							"source_ref":      {Type: "string"},
							"summary":         {Type: "string"},
							"excerpt":         {Type: "string"},
							"command":         {Type: "string"},
							"file_paths":      {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
							"confidence":      {Type: "number"},
							"supports_claims": {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
							"warnings":        {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
						},
					},
				},
			},
		},
		Examples: []toolruntime.Example{
			{When: "You need to inspect evidence already captured for this job.", Input: map[string]any{"job_id": 123}},
		},
	}, func(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
		jobID := int64(toolInputInt(call.Input, "job_id", 0))
		if jobID <= 0 {
			return toolruntime.Result{}, fmt.Errorf("evidence.inspect job_id is required")
		}
		records, err := s.repo.ListEvidenceByJob(ctx, jobID)
		if err != nil {
			return toolruntime.Result{}, err
		}
		limit := toolInputInt(call.Input, "limit", 0)
		if limit > 0 && limit < len(records) {
			records = records[:limit]
		}
		items := make([]map[string]any, 0, len(records))
		for _, record := range records {
			items = append(items, evidenceRecordMap(record))
		}
		return toolruntime.Result{
			Summary: fmt.Sprintf("evidence_records=%d", len(records)),
			Output: map[string]any{
				"summary": fmt.Sprintf("evidence_records=%d", len(records)),
				"records": items,
			},
			Warnings: toolWarnings(len(records) == 0, "no evidence has been captured for this job"),
		}, nil
	})

	return registry
}

func mustRegisterTool(registry *toolruntime.Registry, spec toolruntime.Spec, handler toolruntime.Handler) {
	if err := registry.Register(spec, handler); err != nil {
		panic(err)
	}
}

func toolInputString(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, ok := input[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func toolInputInt(input map[string]any, key string, fallback int) int {
	if input == nil {
		return fallback
	}
	value, ok := input[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func toolWarnings(add bool, warning string) []string {
	if !add || strings.TrimSpace(warning) == "" {
		return nil
	}
	return []string{strings.TrimSpace(warning)}
}

func evidenceRecordMap(record evidence.Record) map[string]any {
	return map[string]any{
		"id":              record.ID,
		"kind":            record.Kind,
		"source_type":     record.SourceType,
		"source_ref":      record.SourceRef,
		"summary":         record.Summary,
		"excerpt":         record.Excerpt,
		"command":         record.Command,
		"file_paths":      append([]string(nil), record.FilePaths...),
		"confidence":      record.Confidence,
		"supports_claims": append([]string(nil), record.SupportsClaims...),
		"warnings":        append([]string(nil), record.Warnings...),
	}
}

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

func evidenceRecordsFromMaps(items []map[string]any) []evidence.Record {
	records := make([]evidence.Record, 0, len(items))
	for _, item := range items {
		record := evidence.Record{
			ID:             int64(toolInputInt(item, "id", 0)),
			Kind:           toolInputString(item, "kind"),
			SourceType:     toolInputString(item, "source_type"),
			SourceRef:      toolInputString(item, "source_ref"),
			Summary:        toolInputString(item, "summary"),
			Excerpt:        toolInputString(item, "excerpt"),
			Command:        toolInputString(item, "command"),
			FilePaths:      parseAnyStringSlice(item["file_paths"]),
			Confidence:     toolInputFloat(item, "confidence"),
			SupportsClaims: parseAnyStringSlice(item["supports_claims"]),
			Warnings:       parseAnyStringSlice(item["warnings"]),
		}
		records = append(records, record)
	}
	return records
}

func toolInputFloat(input map[string]any, key string) float64 {
	if input == nil {
		return 0
	}
	value, ok := input[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float32:
		return float64(typed)
	case float64:
		return typed
	case int:
		return float64(typed)
	case int8:
		return float64(typed)
	case int16:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint8:
		return float64(typed)
	case uint16:
		return float64(typed)
	case uint32:
		return float64(typed)
	case uint64:
		return float64(typed)
	default:
		return 0
	}
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
	case "planning", "intent_parse":
		return "executive_planner"
	case "capability_audit":
		return "capability_auditor"
	case "subtask":
		return "subtask_executor"
	default:
		return ""
	}
}

func (s *Service) executeV3Tool(ctx context.Context, claim *model.ClaimedStep, skillID string, call toolruntime.Call) (toolruntime.Result, error) {
	if s == nil || s.v3Tools == nil {
		return toolruntime.Result{}, fmt.Errorf("v3 tool registry unavailable")
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
	if err := s.repo.WriteArtifact(ctx, mustMarshalArtifact(claim, artifacts.KindToolCall, callArtifact)); err != nil {
		return toolruntime.Result{}, err
	}
	s.emitStepEvent(claim.Step.ID, "tool_call_begin", fmt.Sprintf("tool=%s skill=%s", safeLine(call.Name, "unknown"), safeLine(skillID, "unknown")))

	result, err := s.v3Tools.Execute(ctx, call, toolruntime.ExecuteOptions{
		Allowed:       allowed,
		Forbidden:     forbidden,
		RequireListed: requireListed,
	})
	if err != nil {
		callArtifact.Allowed = false
		_ = s.repo.WriteArtifact(ctx, mustMarshalArtifact(claim, artifacts.KindToolResult, artifacts.ToolResultArtifact{
			Tool:     call.Name,
			Skill:    skillID,
			Accepted: false,
			Summary:  strings.TrimSpace(err.Error()),
			Error:    strings.TrimSpace(err.Error()),
		}))
		s.emitStepEvent(claim.Step.ID, "tool_call_rejected", fmt.Sprintf("tool=%s reason=%s", safeLine(call.Name, "unknown"), safeLine(err.Error(), "rejected")))
		return toolruntime.Result{}, err
	}

	for _, record := range result.Evidence {
		record.JobID = claim.Job.ID
		record.StepID = claim.Step.ID
		if err := s.repo.WriteEvidence(ctx, record); err != nil {
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
	if err := s.repo.WriteArtifact(ctx, mustMarshalArtifact(claim, artifacts.KindToolResult, resultArtifact)); err != nil {
		return toolruntime.Result{}, err
	}
	s.emitStepEvent(claim.Step.ID, "tool_call_complete", fmt.Sprintf("tool=%s accepted=%t", safeLine(result.Tool, call.Name), result.Accepted))
	return result, nil
}

func mustMarshalArtifact(claim *model.ClaimedStep, kind string, payload any) artifacts.Envelope {
	env, err := artifacts.MarshalPayload(kind, "1", payload)
	if err != nil {
		panic(err)
	}
	env.JobID = claim.Job.ID
	env.StepID = claim.Step.ID
	return env
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
