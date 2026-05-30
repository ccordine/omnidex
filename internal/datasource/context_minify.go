package datasource

import (
	"fmt"
	"sort"
	"strings"
)

const (
	MaxInvestigationQueries  = 6
	InvestigationFactBudget  = 3200
	PerStepFactBudget        = 520
	MaxRowsPerStepSample     = 12
	CavemanPerCellChars      = 48
	CavemanPurposeChars      = 120
)

// QueryStep records one read-only query in a multi-step investigation.
type QueryStep struct {
	Step      int              `json:"step"`
	Purpose   string           `json:"purpose,omitempty"`
	SQL       string           `json:"sql"`
	RowCount  int              `json:"row_count"`
	Columns   []string         `json:"columns,omitempty"`
	Rows      []map[string]any `json:"rows,omitempty"`
	HardFacts []string         `json:"hard_facts,omitempty"`
}

func cavemanText(raw string, maxChars int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if text == "" {
		return ""
	}
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	marker := " … "
	if maxChars <= len(marker)+12 {
		return text[:maxChars]
	}
	head := (maxChars - len(marker)) * 2 / 3
	tail := maxChars - len(marker) - head
	if tail < 0 {
		tail = 0
	}
	if head <= 0 {
		return text[:maxChars]
	}
	return text[:head] + marker + text[len(text)-tail:]
}

func appendCavemanLinesWithinBudget(lines []string, budget int) []string {
	if budget <= 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	used := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cost := len(line) + 1
		if used+cost > budget {
			remaining := budget - used
			if remaining > 24 {
				out = append(out, cavemanText(line, remaining)+" …[truncated]")
			}
			break
		}
		out = append(out, line)
		used += cost
	}
	return out
}

func buildInvestigationCavemanBlock(question string, steps []QueryStep, budget int) []string {
	if budget <= 0 {
		budget = InvestigationFactBudget
	}
	lines := []string{
		"CAVE MAN SUMMARY. SOURCE IS READ-ONLY SQL STEPS. USE ONLY THESE HARD FACTS.",
		"question: " + cavemanText(question, 360),
	}
	used := len(lines[0]) + len(lines[1]) + 2
	for _, step := range steps {
		for _, fact := range step.HardFacts {
			line := fmt.Sprintf("q%d: %s", step.Step, fact)
			if used+len(line)+1 > budget {
				lines = append(lines, "... [older facts omitted to stay in budget]")
				return lines
			}
			lines = append(lines, line)
			used += len(line) + 1
		}
	}
	return lines
}

func minifyQueryStepFacts(step int, purpose, sql string, columns []string, rows []map[string]any, strict bool) []string {
	purpose = strings.TrimSpace(purpose)
	lines := []string{}
	if purpose != "" {
		lines = append(lines, "purpose="+cavemanText(purpose, CavemanPurposeChars))
	}
	lines = append(lines, fmt.Sprintf("rows=%d", len(rows)))
	if len(columns) > 0 {
		safeCols := filterDisplayColumns(columns, strict)
		lines = append(lines, "cols="+strings.Join(safeCols, ","))
	}
	if len(rows) == 0 {
		return appendCavemanLinesWithinBudget(lines, PerStepFactBudget)
	}

	safeCols := filterDisplayColumns(columns, strict)
	limit := MaxRowsPerStepSample
	if len(rows) < limit {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		lines = append(lines, fmt.Sprintf("r%d=%s", i+1, formatRowCaveman(safeCols, rows[i])))
	}
	if len(rows) > limit {
		lines = append(lines, fmt.Sprintf("+%d more rows omitted", len(rows)-limit))
	}
	_ = sql // sql stored on QueryStep, not duplicated in caveman facts
	return appendCavemanLinesWithinBudget(lines, PerStepFactBudget)
}

func filterDisplayColumns(columns []string, strict bool) []string {
	out := make([]string, 0, len(columns))
	for _, col := range columns {
		if strict && (IsSensitiveColumn(col) || IsTextContentColumn(col, "")) {
			continue
		}
		out = append(out, col)
	}
	if len(out) == 0 && len(columns) > 0 {
		return []string{"[redacted]"}
	}
	return out
}

func formatRowCaveman(columns []string, row map[string]any) string {
	if len(columns) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(columns))
	for _, col := range columns {
		value := stringifyCell(row[col])
		text := cavemanText(fmt.Sprint(value), CavemanPerCellChars)
		if text == "" || text == "<nil>" {
			continue
		}
		parts = append(parts, col+"="+text)
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return "{}"
	}
	return strings.Join(parts, " ")
}

func mergeHardFacts(steps []QueryStep) []string {
	out := make([]string, 0, len(steps)*4)
	for _, step := range steps {
		for _, fact := range step.HardFacts {
			fact = strings.TrimSpace(fact)
			if fact == "" {
				continue
			}
			out = append(out, fmt.Sprintf("q%d %s", step.Step, fact))
		}
	}
	return out
}
