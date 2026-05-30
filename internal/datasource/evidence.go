package datasource

import (
	"fmt"
	"strings"
)

// StepEvidence is one read-only SQL step backing the final answer.
type StepEvidence struct {
	Step     int      `json:"step"`
	Purpose  string   `json:"purpose,omitempty"`
	SQL      string   `json:"sql"`
	RowCount int      `json:"row_count"`
	Facts    []string `json:"facts,omitempty"`
}

// QueryEvidence links a human question to verified SQL steps and hard facts.
type QueryEvidence struct {
	Question   string         `json:"question"`
	Summary    string         `json:"summary"`
	StepCount  int            `json:"step_count"`
	RowCount   int            `json:"row_count"`
	Confidence string         `json:"confidence"`
	Steps      []StepEvidence `json:"steps"`
	Citations  []string       `json:"citations,omitempty"`
}

func BuildQueryEvidence(result QueryResult) QueryEvidence {
	steps := make([]StepEvidence, 0, len(result.QuerySteps))
	citations := make([]string, 0, len(result.HardFacts))
	for _, step := range result.QuerySteps {
		steps = append(steps, StepEvidence{
			Step:     step.Step,
			Purpose:  step.Purpose,
			SQL:      step.SQL,
			RowCount: step.RowCount,
			Facts:    append([]string{}, step.HardFacts...),
		})
	}
	for _, fact := range result.HardFacts {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		citations = append(citations, fact)
	}
	for _, insight := range result.TextInsights {
		citations = append(citations, fmt.Sprintf("text:%s samples=%d %s", insight.Field, insight.Samples, cavemanText(insight.Summary, 120)))
	}
	confidence := evidenceConfidence(result)
	summary := strings.TrimSpace(result.Answer)
	if summary == "" && result.Count > 0 {
		summary = fmt.Sprintf("%d row(s) returned from the database.", result.Count)
	}
	return QueryEvidence{
		Question:   strings.TrimSpace(result.Question),
		Summary:    summary,
		StepCount:  len(result.QuerySteps),
		RowCount:   result.Count,
		Confidence: confidence,
		Steps:      steps,
		Citations:  citations,
	}
}

func evidenceConfidence(result QueryResult) string {
	if len(result.QuerySteps) == 0 {
		if result.Count > 0 {
			return "medium"
		}
		return "low"
	}
	if len(result.HardFacts) >= len(result.QuerySteps)*2 && result.Count > 0 {
		return "high"
	}
	if len(result.QuerySteps) >= 2 || result.Count > 0 {
		return "medium"
	}
	return "low"
}

func pickChartResult(result QueryResult) QueryResult {
	best := QueryResult{
		Question: result.Question,
		Columns:  result.Columns,
		Rows:     result.Rows,
		Count:    result.Count,
		SQL:      result.SQL,
	}
	bestScore := chartResultScore(best)
	for i := len(result.QuerySteps) - 1; i >= 0; i-- {
		step := result.QuerySteps[i]
		if len(step.Columns) == 0 || len(step.Rows) == 0 {
			continue
		}
		candidate := QueryResult{
			Question: result.Question,
			SQL:      step.SQL,
			Columns:  step.Columns,
			Rows:     step.Rows,
			Count:    len(step.Rows),
		}
		score := chartResultScore(candidate)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func chartResultScore(result QueryResult) int {
	spec := BuildChartSpec(result)
	if spec == nil || len(spec.Series) == 0 {
		return 0
	}
	score := len(spec.Series) * 10
	if spec.Type == "line" {
		score += 5
	}
	return score
}

func formatEvidenceAnswer(result QueryResult) string {
	answer := strings.TrimSpace(result.Answer)
	if answer == "" {
		return answer
	}
	if strings.Contains(strings.ToLower(answer), "missing:") {
		return answer
	}
	if len(result.QuerySteps) == 0 {
		return answer
	}
	queryWord := "queries"
	if len(result.QuerySteps) == 1 {
		queryWord = "query"
	}
	prefix := fmt.Sprintf("Evidence: %d read-only %s verified this answer.", len(result.QuerySteps), queryWord)
	if strings.HasPrefix(strings.ToLower(answer), "investigation ran") {
		return answer
	}
	return prefix + "\n\n" + answer
}

// FinalizeQueryResult attaches evidence metadata and picks the best chart/table snapshot.
func FinalizeQueryResult(result QueryResult) QueryResult {
	if strings.TrimSpace(result.Evidence.Question) != "" {
		return result
	}
	chartSource := pickChartResult(result)
	result.Columns = chartSource.Columns
	result.Rows = chartSource.Rows
	result.Count = len(chartSource.Rows)
	if strings.TrimSpace(result.SQL) == "" {
		result.SQL = chartSource.SQL
	}
	result.Answer = formatEvidenceAnswer(result)
	result.Evidence = BuildQueryEvidence(result)
	return result
}
