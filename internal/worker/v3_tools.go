package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
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
				"root":             {Type: "string"},
				"files_considered": {Type: "integer"},
				"languages":        {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
				"summary":          {Type: "string"},
				"missing_context":  {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
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
		scope, err := v3WorkspaceScopeFromContext(ctx)
		if err != nil {
			return toolruntime.Result{}, err
		}
		query := strings.TrimSpace(toolInputString(call.Input, "query"))
		if query == "" {
			return toolruntime.Result{}, fmt.Errorf("workspace.research query is required")
		}
		research, err := scope.Scanner.Research(query)
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

	registerV3MemoryTools(registry, s)

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
		report, err := s.webSearch.SearchAllDetailed(ctx, query)
		if err != nil {
			return toolruntime.Result{}, err
		}
		results := report.Results
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

	registerV3EvidenceTool(registry, s)
	registerV3ExecutionTools(registry)

	return registry
}
