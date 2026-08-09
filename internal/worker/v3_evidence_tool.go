package worker

import (
	"context"
	"fmt"

	toolruntime "github.com/gryph/omnidex/internal/tools"
)

func registerV3EvidenceTool(registry *toolruntime.Registry, service *Service) {
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
							"tool_name":       {Type: "string"},
							"specialist_id":   {Type: "string"},
							"subtask_id":      {Type: "string"},
							"objective_id":    {Type: "string"},
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
		Examples: []toolruntime.Example{{When: "You need to inspect evidence already captured for this job.", Input: map[string]any{"job_id": 123}}},
	}, func(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
		jobID := int64(toolInputInt(call.Input, "job_id", 0))
		if jobID <= 0 {
			return toolruntime.Result{}, fmt.Errorf("evidence.inspect job_id is required")
		}
		limit := toolInputInt(call.Input, "limit", 200)
		records, err := service.repo.ListCurrentEvidenceByJob(ctx, jobID, limit)
		if err != nil {
			return toolruntime.Result{}, err
		}
		items := make([]map[string]any, 0, len(records))
		for _, record := range records {
			items = append(items, evidenceRecordMap(record))
		}
		summary := fmt.Sprintf("evidence_records=%d", len(records))
		return toolruntime.Result{
			Summary:  summary,
			Output:   map[string]any{"summary": summary, "records": items},
			Warnings: toolWarnings(len(records) == 0, "no evidence has been captured for this job"),
		}, nil
	})
}
