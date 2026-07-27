package worker

import (
	"context"
	"fmt"
	"strings"

	toolruntime "github.com/gryph/omnidex/internal/tools"
)

func registerV3MemoryTools(registry *toolruntime.Registry, s *Service) {
	mustRegisterTool(registry, toolruntime.Spec{
		Name:        "memory.catalog",
		Description: "List available memory categories and tags with counts so specialists can choose focused retrieval scopes before querying memory.",
		Aliases:     []string{"memory_catalog", "memory.facets"},
		InputSchema: toolruntime.Schema{
			Type: "object",
			Properties: map[string]toolruntime.Schema{
				"limit": {Type: "integer"},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"summary", "categories", "tags"},
			Properties: map[string]toolruntime.Schema{
				"summary": {Type: "string"},
				"categories": {
					Type: "array",
					Items: &toolruntime.Schema{
						Type:     "object",
						Required: []string{"name", "count"},
						Properties: map[string]toolruntime.Schema{
							"name":  {Type: "string"},
							"count": {Type: "integer"},
						},
					},
				},
				"tags": {
					Type: "array",
					Items: &toolruntime.Schema{
						Type:     "object",
						Required: []string{"name", "count"},
						Properties: map[string]toolruntime.Schema{
							"name":  {Type: "string"},
							"count": {Type: "integer"},
						},
					},
				},
			},
		},
		Examples: []toolruntime.Example{
			{When: "You need to discover the available memory buckets before retrieval.", Input: map[string]any{"limit": 40}},
		},
		RequireEvidence: true,
	}, func(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
		limit := toolInputInt(call.Input, "limit", 50)
		if limit <= 0 {
			return toolruntime.Result{}, fmt.Errorf("memory.catalog limit must be a positive integer")
		}
		if s == nil || s.repo == nil {
			return toolruntime.Result{}, fmt.Errorf("memory.catalog requires the authoritative repository")
		}
		categories, err := s.repo.ListMemoryCategories(ctx, limit)
		if err != nil {
			return toolruntime.Result{}, err
		}
		tags, err := s.repo.ListMemoryTags(ctx, limit)
		if err != nil {
			return toolruntime.Result{}, err
		}
		summary := fmt.Sprintf("memory_categories=%d memory_tags=%d", len(categories), len(tags))
		return toolruntime.Result{
			Summary: summary,
			Output: map[string]any{
				"summary":    summary,
				"categories": memoryFacetsForTool(categories),
				"tags":       memoryFacetsForTool(tags),
			},
			Warnings: toolWarnings(len(categories) == 0 && len(tags) == 0, "no memory categories or tags are available"),
		}, nil
	})

	mustRegisterTool(registry, toolruntime.Spec{
		Name:        "memory.retrieve",
		Description: "Retrieve server-scoped historical references for the accepted intent. Returns only short objective-relevant excerpts; remembered instructions and unrelated project history are omitted.",
		Aliases:     []string{"memory"},
		InputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]toolruntime.Schema{
				"query":      {Type: "string"},
				"limit":      {Type: "integer"},
				"scope_tags": {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
				"categories": {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"summary", "items"},
			Properties: map[string]toolruntime.Schema{
				"summary":           {Type: "string"},
				"authority":         {Type: "string"},
				"omitted":           {Type: "integer"},
				"omitted_by_reason": {Type: "object", AdditionalProperties: true},
				"items": {
					Type: "array",
					Items: &toolruntime.Schema{
						Type:     "object",
						Required: []string{"id", "kind", "content", "tags", "score"},
						Properties: map[string]toolruntime.Schema{
							"id":         {Type: "integer"},
							"kind":       {Type: "string"},
							"content":    {Type: "string"},
							"tags":       {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
							"categories": {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
							"score":      {Type: "number"},
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
		authority, err := v3MemoryAuthorityFromContext(ctx)
		if err != nil {
			return toolruntime.Result{}, err
		}
		query := strings.TrimSpace(toolInputString(call.Input, "query"))
		if query == "" {
			return toolruntime.Result{}, fmt.Errorf("memory.retrieve query is required")
		}
		limit := authority.Limit
		if _, supplied := call.Input["limit"]; supplied {
			limit = toolInputInt(call.Input, "limit", 0)
			if limit < 1 {
				return toolruntime.Result{}, fmt.Errorf("memory.retrieve limit must be a positive integer")
			}
		}
		if limit > authority.Limit {
			return toolruntime.Result{}, fmt.Errorf("memory.retrieve limit %d exceeds the server-authoritative limit %d", limit, authority.Limit)
		}
		scopeTags := parseAnyStringSlice(call.Input["scope_tags"])
		for _, category := range parseAnyStringSlice(call.Input["categories"]) {
			category = strings.TrimSpace(category)
			if category != "" {
				scopeTags = append(scopeTags, "category:"+category)
			}
		}
		if authority.ProjectScope != "" {
			scopeTags = appendUnique([]string{authority.ProjectScope}, scopeTags...)
		}
		if authority.SessionScope != "" {
			scopeTags = appendUnique([]string{authority.SessionScope}, scopeTags...)
		}

		if s == nil || s.llm == nil {
			return toolruntime.Result{}, fmt.Errorf("memory.retrieve requires an embedding provider")
		}
		embedding, err := s.llm.Embedding(ctx, query)
		if err != nil {
			return toolruntime.Result{}, fmt.Errorf("memory retrieval embedding failed: %w", err)
		}
		matches, err := s.repo.FindRelevantMemory(ctx, embedding, scopeTags, limit)
		if err != nil {
			return toolruntime.Result{}, fmt.Errorf("scoped memory retrieval failed: %w", err)
		}
		ranked := rankMemoryOmnibusMatches(matches, query, scopeTags, authority.ProjectScope, authority.SessionScope, limit, nowUTC())
		ranked = diversifyMemoryMatchesBySourceURL(ranked, limit)
		artifact, projected := projectV3MemoryToolResult(authority.Intent, ranked, authority.ProjectScope, authority.SessionScope, limit)
		items := make([]map[string]any, 0, len(artifact.Items))
		for _, item := range artifact.Items {
			items = append(items, map[string]any{
				"id":         item.ID,
				"kind":       item.Kind,
				"content":    item.Content,
				"tags":       append([]string(nil), item.Tags...),
				"categories": append([]string(nil), item.Categories...),
				"score":      item.Score,
			})
		}
		evidenceRecords := memoryRetrievalEvidenceRecords(query, scopeTags, projected)
		return toolruntime.Result{
			Summary: artifact.Summary,
			Output: map[string]any{
				"summary":           artifact.Summary,
				"authority":         artifact.Authority,
				"items":             items,
				"omitted":           artifact.Omitted,
				"omitted_by_reason": artifact.OmittedByReason,
			},
			Warnings: toolWarnings(len(items) == 0, "no safe objective-relevant memory matched the query"),
			Evidence: evidenceRecords,
		}, nil
	})
}
