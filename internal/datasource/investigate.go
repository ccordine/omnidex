package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/omni"
)

type investigationStepPlan struct {
	Status         string `json:"status"`
	Purpose        string `json:"purpose"`
	SQL            string `json:"sql"`
	Answer         string `json:"answer"`
	ExplorationLog string `json:"exploration_log"`
}

// runInvestigation executes multiple read-only queries, minifying each step into hard facts.
func runInvestigation(ctx context.Context, input AnalyticalAskInput, catalog SchemaCatalog, runner omni.MemorySQLRunner, llm omni.DBManagerLLMClient) (QueryResult, error) {
	strict := input.Profile.PrivacyMode == PrivacyStrict
	question := strings.TrimSpace(input.Question)
	steps := make([]QueryStep, 0, MaxInvestigationQueries)
	var finalAnswer string
	var finalSQL string
	var finalColumns []string
	var finalRows []map[string]any
	var allTextInsights []TextInsight
	wantsText := WantsTextAnalysis(question)

	for stepNum := 1; stepNum <= MaxInvestigationQueries; stepNum++ {
		cavemanFacts := buildInvestigationCavemanBlock(question, steps, InvestigationFactBudget)
		plan, err := planInvestigationStep(ctx, llm, input.Profile, catalog, question, stepNum, cavemanFacts, wantsText)
		if err != nil {
			return QueryResult{}, err
		}

		status := strings.ToLower(strings.TrimSpace(plan.Status))
		if status == "complete" || status == "done" {
			finalAnswer = strings.TrimSpace(plan.Answer)
			if finalAnswer == "" && plan.ExplorationLog != "" {
				finalAnswer = strings.TrimSpace(plan.ExplorationLog)
			}
			break
		}

		sql := strings.TrimSpace(plan.SQL)
		if sql == "" {
			if strings.TrimSpace(plan.Answer) != "" {
				finalAnswer = strings.TrimSpace(plan.Answer)
				break
			}
			return QueryResult{}, fmt.Errorf("investigation step %d returned no sql", stepNum)
		}
		if err := omni.ValidateReadOnlyPostgresQuery(sql); err != nil {
			return QueryResult{}, fmt.Errorf("step %d: %w", stepNum, err)
		}
		if err := ValidatePrivacySafeSelect(sql, strict); err != nil {
			return QueryResult{}, fmt.Errorf("step %d: %w", stepNum, err)
		}
		sql = enforceQueryLimit(sql, MaxQueryRows)

		rows, err := runner.Query(ctx, sql)
		if err != nil {
			return QueryResult{}, fmt.Errorf("step %d query failed: %w", stepNum, err)
		}
		columns, publicRows := rowsToColumns(rows)
		tableHint := inferTableFromSQL(sql, catalog)
		textCols := textContentColumns(columns, catalog, tableHint)
		facts := minifyQueryStepFacts(stepNum, plan.Purpose, sql, columns, publicRows, strict)
		if len(textCols) > 0 {
			insights, insightFacts, redacted := analyzeTextResults(ctx, llm, input.Profile, plan.Purpose, tableHint, textCols, publicRows)
			if len(insightFacts) > 0 {
				facts = append(facts, insightFacts...)
			}
			publicRows = redacted
			allTextInsights = append(allTextInsights, insights...)
		}
		steps = append(steps, QueryStep{
			Step:      stepNum,
			Purpose:   strings.TrimSpace(plan.Purpose),
			SQL:       sql,
			RowCount:  len(publicRows),
			Columns:   columns,
			Rows:      publicRows,
			HardFacts: facts,
		})
		finalSQL = sql
		finalColumns = columns
		finalRows = publicRows

		if status == "complete_after_query" {
			finalAnswer = strings.TrimSpace(plan.Answer)
			break
		}
	}

	if finalAnswer == "" {
		if len(steps) == 0 {
			return QueryResult{}, fmt.Errorf("investigation produced no queries or answer")
		}
		synthesized, err := synthesizeInvestigationAnswer(ctx, llm, input.Profile, question, steps)
		if err != nil {
			finalAnswer = deterministicInvestigationAnswer(question, steps)
		} else {
			finalAnswer = synthesized
		}
	}

	hardFacts := mergeHardFacts(steps)
	answer := finalAnswer
	if len(steps) > 1 {
		prefix := fmt.Sprintf("Investigation ran %d read queries.", len(steps))
		if strings.TrimSpace(answer) == "" {
			answer = prefix
		} else {
			answer = prefix + " " + answer
		}
	}

	return QueryResult{
		Question:     question,
		SQL:          finalSQL,
		Answer:       answer,
		Columns:      finalColumns,
		Rows:         finalRows,
		Count:        len(finalRows),
		HardFacts:    hardFacts,
		QuerySteps:   steps,
		TextInsights: allTextInsights,
	}, nil
}

func planInvestigationStep(ctx context.Context, llm omni.DBManagerLLMClient, profile Profile, catalog SchemaCatalog, question string, stepNum int, cavemanFacts []string, wantsText bool) (investigationStepPlan, error) {
	relevant := selectRelevantTables(catalog, question, 16)
	textFields := CatalogTextFields(catalog)
	if !wantsText && len(textFields) > 12 {
		textFields = textFields[:12]
	}
	payload, _ := json.Marshal(map[string]any{
		"question":             question,
		"step":                 stepNum,
		"max_steps":            MaxInvestigationQueries,
		"domain":               profile.Domain,
		"context_prompt":       profile.ContextPrompt,
		"catalog_summary":      catalog.Summary,
		"relevant_tables":      relevant,
		"text_fields":          textFields,
		"wants_text_analysis":  wantsText,
		"privacy_mode":         profile.PrivacyMode,
		"hard_facts":           cavemanFacts,
	})
	extraGuidance := textAnalysisPlannerGuidance(profile, wantsText)
	resp, err := llm.ChatRaw(ctx, omni.OllamaChatRequest{
		Messages: []omni.OllamaMessage{
			{
				Role: "system",
				Content: strings.Join(filterNonEmpty([]string{
					"Return JSON only.",
					`Schema: {"status":"continue|complete","purpose":"why this query","sql":"read-only PostgreSQL or empty when complete","answer":"evidence-backed answer when status=complete; cite step facts","exploration_log":"brief reasoning"}`,
					"You are an evidence-driven analytical database investigator.",
					"Staff ask in plain language; you answer with accurate SQL-backed evidence.",
					"Use hard_facts from prior steps as the only evidence about prior query results.",
					"If CAVE MAN SUMMARY is present, treat it as the full bounded evidence set; do not invent omitted rows.",
					"Run multiple small read queries when needed: explore counts, distinct values, joins, then answer.",
					"When status=continue, sql is required. When status=complete, answer is required and sql may be empty.",
					"Never invent tables or columns. Only SELECT or WITH queries.",
					DomainGuidance(profile.Domain),
					"In strict privacy mode, prefer COUNT/GROUP BY aggregates; avoid selecting sensitive identifier columns.",
					"Add LIMIT on exploratory row pulls.",
					extraGuidance,
				}), "\n"),
			},
			{Role: "user", Content: string(payload)},
		},
		Format: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":          map[string]any{"type": "string"},
				"purpose":         map[string]any{"type": "string"},
				"sql":             map[string]any{"type": "string"},
				"answer":          map[string]any{"type": "string"},
				"exploration_log": map[string]any{"type": "string"},
			},
			"required": []string{"status", "exploration_log"},
		},
		Options: map[string]any{"temperature": 0},
	})
	if err != nil {
		return investigationStepPlan{}, err
	}
	var plan investigationStepPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &plan); err != nil {
		return investigationStepPlan{}, err
	}
	return plan, nil
}

func filterNonEmpty(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func synthesizeInvestigationAnswer(ctx context.Context, llm omni.DBManagerLLMClient, profile Profile, question string, steps []QueryStep) (string, error) {
	if llm == nil {
		return "", fmt.Errorf("llm client is required")
	}
	cavemanFacts := buildInvestigationCavemanBlock(question, steps, InvestigationFactBudget)
	payload, _ := json.Marshal(map[string]any{
		"question":   question,
		"domain":     profile.Domain,
		"hard_facts": cavemanFacts,
	})
	resp, err := llm.ChatRaw(ctx, omni.OllamaChatRequest{
		Messages: []omni.OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"Return JSON only.",
					`Schema: {"answer":"concise user-facing answer grounded in hard_facts"}`,
					"Role: evidence-driven reducer. Use only hard_facts from read-only SQL steps.",
					"Every claim must be supportable by cited step facts (q1, q2, ...). Missing evidence: say MISSING: <item>.",
					"Prefer accurate counts and comparisons over vague language.",
					"If input says CAVE MAN SUMMARY, treat it as the full bounded evidence set and do not invent omitted details.",
					"For text/sentiment hard facts, summarize themes and counts — never quote patient comments verbatim.",
					DomainGuidance(profile.Domain),
				}, "\n"),
			},
			{Role: "user", Content: string(payload)},
		},
		Format: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]any{"type": "string"},
			},
			"required": []string{"answer"},
		},
		Options: map[string]any{"temperature": 0, "num_predict": 400},
	})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &parsed); err != nil {
		return "", err
	}
	answer := strings.TrimSpace(parsed.Answer)
	if answer == "" {
		return "", fmt.Errorf("synthesized answer is empty")
	}
	return answer, nil
}

func deterministicInvestigationAnswer(question string, steps []QueryStep) string {
	var b strings.Builder
	b.WriteString("Answer from investigation (")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("):\n")
	for _, step := range steps {
		b.WriteString(fmt.Sprintf("- Step %d (%d rows): ", step.Step, step.RowCount))
		if step.Purpose != "" {
			b.WriteString(step.Purpose)
			b.WriteString(" — ")
		}
		if len(step.HardFacts) > 0 {
			b.WriteString(strings.Join(step.HardFacts, "; "))
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
