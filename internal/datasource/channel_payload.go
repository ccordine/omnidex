package datasource

import "strings"

type ChannelMessagePayload struct {
	Query    QueryResult   `json:"query"`
	Chart    *ChartSpec    `json:"chart,omitempty"`
	Evidence QueryEvidence `json:"evidence,omitempty"`
	JobID    int64         `json:"job_id,omitempty"`
}

func BuildChannelMessagePayload(result QueryResult, jobID int64) ChannelMessagePayload {
	result = FinalizeQueryResult(result)
	chart := BuildChartSpec(result)
	if chart != nil && strings.TrimSpace(result.Question) != "" {
		chart.Title = chartTitle(result)
	}
	return ChannelMessagePayload{
		Query:    result,
		Chart:    chart,
		Evidence: result.Evidence,
		JobID:    jobID,
	}
}

func chartTitle(result QueryResult) string {
	question := strings.TrimSpace(result.Question)
	if question != "" {
		return cavemanText(question, 120)
	}
	if strings.TrimSpace(result.Answer) != "" {
		return cavemanText(result.Answer, 120)
	}
	return "Query results"
}
