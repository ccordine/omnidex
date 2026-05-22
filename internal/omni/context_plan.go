package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const defaultContextPlannerTimeout = 30 * time.Second

type ContextToolPlan struct {
	NeedsWebResearch bool
	NeedsMemory      bool
	NeedsDocuments   bool
	NeedsShell       bool
	AllowClarify     bool
	RequireEvidence  bool
	Tools            []string
	Reason           string
}

func DefaultContextToolPlan() ContextToolPlan {
	return ContextToolPlan{
		NeedsShell: true,
		Tools:      []string{"shell"},
		Reason:     "default shell evidence",
	}
}

func AugmentContextToolPlan(input string, plan ContextToolPlan) ContextToolPlan {
	if len(InferDocumentationResearchTarget(input).Sources) > 0 {
		if !plan.NeedsDocuments {
			plan.NeedsDocuments = true
			plan.Tools = append(plan.Tools, "documentation")
		}
		if !plan.NeedsShell {
			plan.NeedsShell = true
			plan.Tools = append(plan.Tools, "shell")
		}
		if strings.TrimSpace(plan.Reason) == "" || plan.Reason == "default shell evidence" {
			plan.Reason = "language/toolchain build task needs documentation and shell evidence"
		}
	}
	plan.Tools = dedupeStrings(plan.Tools)
	return plan
}

func PlanContextTools(ctx context.Context, client *OllamaClient, input string) (ContextToolPlan, error) {
	if client == nil {
		return DefaultContextToolPlan(), nil
	}
	ctx, cancel := context.WithTimeout(ctx, defaultContextPlannerTimeout)
	defer cancel()

	resp, err := client.ChatRaw(ctx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: buildContextPlannerPrompt()},
			{Role: "user", Content: strings.TrimSpace(input)},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 180,
		},
	})
	if err != nil {
		return DefaultContextToolPlan(), err
	}
	return ParseContextToolPlan(resp.Content)
}

func buildContextPlannerPrompt() string {
	return strings.Join(withMinimalOutputContract(
		"Role: context tool planner.",
		"Decide tools needed before answering.",
		"Output JSON only. No markdown.",
		"Allowed tools: web_research,memory,documentation,document_processing,shell.",
		"Schema: {\"tools\":[\"shell\"],\"allow_clarify\":false,\"require_evidence\":true,\"reason\":\"...\"}",
		"Use web_research for external/current facts.",
		"Use memory to store/retrieve reusable research.",
		"Use documentation for SDK/API/library docs across any language or tool.",
		"Use document_processing for long docs/scraping.",
		"Use shell for terminal evidence/actions.",
	), "\n")
}

func ParseContextToolPlan(raw string) (ContextToolPlan, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)
	var decoded struct {
		Tools           []string `json:"tools"`
		AllowClarify    bool     `json:"allow_clarify"`
		RequireEvidence bool     `json:"require_evidence"`
		Reason          string   `json:"reason"`
	}
	if err := json.Unmarshal([]byte(clean), &decoded); err != nil {
		return DefaultContextToolPlan(), fmt.Errorf("parse context plan JSON: %w", err)
	}
	plan := ContextToolPlan{Reason: strings.TrimSpace(decoded.Reason)}
	plan.AllowClarify = decoded.AllowClarify
	plan.RequireEvidence = decoded.RequireEvidence
	if plan.Reason == "" {
		plan.Reason = "context planner selected tools"
	}
	for _, tool := range decoded.Tools {
		switch strings.TrimSpace(tool) {
		case "web_research":
			plan.NeedsWebResearch = true
			plan.Tools = append(plan.Tools, "web_research")
		case "memory":
			plan.NeedsMemory = true
			plan.Tools = append(plan.Tools, "memory")
		case "documentation", "docs", "document_processing":
			plan.NeedsDocuments = true
			plan.Tools = append(plan.Tools, "documentation")
		case "shell":
			plan.NeedsShell = true
			plan.Tools = append(plan.Tools, "shell")
		}
	}
	if len(plan.Tools) == 0 {
		plan = DefaultContextToolPlan()
	}
	plan.Tools = dedupeStrings(plan.Tools)
	return plan, nil
}
