package codingagent

import (
	"encoding/json"
)

func MapParserResponse(call Assignment, content json.RawMessage) (json.RawMessage, error) {
	if call.Role == PlanningInitial || call.Role == PlanningMutation {
		return mapPlanningResponse(call, content)
	}
	if call.Role != ReviewResponse {
		return content, nil
	}
	review, err := DecodeReviewResponse(call, content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(review)
}
func DecodeReviewResponse(call Assignment, content json.RawMessage) (ReviewResponseResult, error) {
	taskID := ""
	if call.Task != nil {
		taskID = call.Task.ID
	}
	var parsed struct {
		Kind     ExecutionResponseKind `json:"kind"`
		Findings []struct {
			File        string `json:"file"`
			Observation string `json:"observation"`
		} `json:"findings"`
		Verification string `json:"verification"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return ReviewResponseResult{}, err
	}
	out := ReviewResponseResult{Kind: parsed.Kind, ReviewResult: ReviewResult{Findings: []Finding{}, Verification: call.Response}}
	for _, finding := range parsed.Findings {
		out.Findings = append(out.Findings, Finding{TaskID: taskID, File: finding.File, Observation: finding.Observation})
	}
	return out, nil
}
