package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/omni"
)

const maxTextSnippetChars = 220

type TextInsight struct {
	Field     string         `json:"field"`
	Table     string         `json:"table,omitempty"`
	Samples   int            `json:"samples"`
	Summary   string         `json:"summary"`
	Themes    []string       `json:"themes,omitempty"`
	Sentiment map[string]int `json:"sentiment,omitempty"`
}

type textSentimentPayload struct {
	SampleCount int            `json:"sample_count"`
	Sentiment   map[string]int `json:"sentiment"`
	Themes      []string       `json:"themes"`
	Summary     string         `json:"summary"`
}

func analyzeTextResults(ctx context.Context, llm omni.DBManagerLLMClient, profile Profile, purpose, table string, textCols []string, rows []map[string]any) ([]TextInsight, []string, []map[string]any) {
	if llm == nil || len(textCols) == 0 || len(rows) == 0 {
		return nil, nil, redactTextColumns(rows, textCols)
	}
	insights := make([]TextInsight, 0, len(textCols))
	factLines := make([]string, 0, len(textCols)*4)
	for _, col := range textCols {
		samples := collectTextSamples(rows, col, MaxTextSampleRows)
		if len(samples) == 0 {
			continue
		}
		payload, err := runTextSentimentAnalysis(ctx, llm, profile, purpose, table, col, samples)
		if err != nil {
			factLines = append(factLines, fmt.Sprintf("text_field=%s samples=%d analysis=unavailable", col, len(samples)))
			continue
		}
		insight := TextInsight{
			Field:     col,
			Table:     table,
			Samples:   payload.SampleCount,
			Summary:   strings.TrimSpace(payload.Summary),
			Themes:    payload.Themes,
			Sentiment: payload.Sentiment,
		}
		if insight.Samples <= 0 {
			insight.Samples = len(samples)
		}
		insights = append(insights, insight)
		factLines = append(factLines, textInsightFacts(insight)...)
	}
	return insights, factLines, redactTextColumns(rows, textCols)
}

func collectTextSamples(rows []map[string]any, column string, max int) []string {
	if max <= 0 {
		max = MaxTextSampleRows
	}
	out := make([]string, 0, max)
	for _, row := range rows {
		if len(out) >= max {
			break
		}
		raw := strings.TrimSpace(fmt.Sprint(stringifyCell(row[column])))
		if raw == "" || raw == "<nil>" {
			continue
		}
		out = append(out, cavemanText(raw, maxTextSnippetChars))
	}
	return out
}

func runTextSentimentAnalysis(ctx context.Context, llm omni.DBManagerLLMClient, profile Profile, purpose, table, field string, samples []string) (textSentimentPayload, error) {
	payload, _ := json.Marshal(map[string]any{
		"purpose": purpose,
		"table":   table,
		"field":   field,
		"domain":  profile.Domain,
		"samples": samples,
	})
	resp, err := llm.ChatRaw(ctx, omni.OllamaChatRequest{
		Messages: []omni.OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"Return JSON only.",
					`Schema: {"sample_count":0,"sentiment":{"positive":0,"neutral":0,"negative":0},"themes":["theme"],"summary":"aggregate staff-facing summary"}`,
					"You analyze de-identified patient/staff text samples for sentiment and themes.",
					"Never quote full comments verbatim. Never invent patient names or identifiers.",
					"Output aggregate counts and short theme labels only.",
					"summary must be safe for staff dashboards — no PHI, no direct quotes longer than 6 words.",
					TextAnalysisGuidance(profile.Domain),
				}, "\n"),
			},
			{Role: "user", Content: string(payload)},
		},
		Format: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sample_count": map[string]any{"type": "integer"},
				"sentiment": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"positive": map[string]any{"type": "integer"},
						"neutral":  map[string]any{"type": "integer"},
						"negative": map[string]any{"type": "integer"},
					},
				},
				"themes": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
				"summary": map[string]any{"type": "string"},
			},
			"required": []string{"sample_count", "sentiment", "themes", "summary"},
		},
		Options: map[string]any{"temperature": 0, "num_predict": 350},
	})
	if err != nil {
		return textSentimentPayload{}, err
	}
	var parsed textSentimentPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &parsed); err != nil {
		return textSentimentPayload{}, err
	}
	if parsed.SampleCount <= 0 {
		parsed.SampleCount = len(samples)
	}
	return parsed, nil
}

func textInsightFacts(insight TextInsight) []string {
	lines := []string{
		fmt.Sprintf("text_field=%s samples=%d", insight.Field, insight.Samples),
	}
	if insight.Summary != "" {
		lines = append(lines, "text_summary="+cavemanText(insight.Summary, 180))
	}
	if len(insight.Themes) > 0 {
		lines = append(lines, "themes="+cavemanText(strings.Join(insight.Themes, ","), 120))
	}
	if len(insight.Sentiment) > 0 {
		parts := make([]string, 0, len(insight.Sentiment))
		for label, count := range insight.Sentiment {
			parts = append(parts, fmt.Sprintf("%s=%d", label, count))
		}
		sortStrings(parts)
		lines = append(lines, "sentiment="+strings.Join(parts, ","))
	}
	return lines
}

func redactTextColumns(rows []map[string]any, textCols []string) []map[string]any {
	if len(rows) == 0 || len(textCols) == 0 {
		return rows
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		next := map[string]any{}
		for key, value := range row {
			if isTextColumnName(key, textCols) {
				next[key] = "[text analyzed — not shown]"
				continue
			}
			next[key] = value
		}
		out = append(out, next)
	}
	return out
}

func isTextColumnName(name string, textCols []string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, col := range textCols {
		if strings.ToLower(strings.TrimSpace(col)) == lower {
			return true
		}
	}
	return false
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func TextAnalysisGuidance(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case DomainHealthcare:
		return "Healthcare patient-submitted comments/feedback/survey text. Focus on operational themes (wait time, billing, staff, portal UX). Do not surface clinical details or identify individuals."
	default:
		return "User-submitted text fields. Summarize sentiment and recurring themes without quoting long passages."
	}
}

func textAnalysisPlannerGuidance(profile Profile, wantsText bool) string {
	if !wantsText && profile.Domain != DomainHealthcare {
		return ""
	}
	lines := []string{
		"For comment/feedback/sentiment questions:",
		"1) locate patient-submitted text fields from text_fields catalog hints",
		"2) run COUNT/GROUP BY when rating or sentiment columns already exist",
		"3) otherwise fetch a bounded text sample (LIMIT <= 24, one text column) — raw text is analyzed internally and redacted from staff results",
		"4) never SELECT identifiers with free text in the same query",
	}
	if profile.Domain == DomainHealthcare {
		lines = append(lines, "Prefer patient feedback, portal messages, and survey tables over clinical progress notes.")
	}
	return strings.Join(lines, "\n")
}
