package codingagent

import (
	"encoding/json"
	"fmt"
)

func MapParserResponse(call Assignment, content json.RawMessage) (json.RawMessage, error) {
	if call.ParserStep != nil {
		if err := ValidateParserLeafResponse(call, content); err != nil {
			return nil, err
		}
		return content, nil
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
	if call.Task == nil {
		return ReviewResponseResult{}, fmt.Errorf("review parser requires one code-selected task")
	}
	var parsed struct {
		Kind     ExecutionResponseKind `json:"kind"`
		Findings []struct {
			File        *string `json:"file"`
			Observation *string `json:"observation"`
		} `json:"findings"`
		Verification string `json:"verification"`
	}
	if err := decodeContract(content, &parsed); err != nil {
		return ReviewResponseResult{}, err
	}
	if parsed.Findings == nil {
		return ReviewResponseResult{}, fmt.Errorf("review parser requires a findings array")
	}
	out := ReviewResponseResult{Kind: parsed.Kind, ReviewResult: ReviewResult{Findings: []Finding{}, Verification: parsed.Verification}}
	for _, finding := range parsed.Findings {
		if finding.File == nil || finding.Observation == nil {
			return ReviewResponseResult{}, fmt.Errorf("review finding requires file and observation fields")
		}
		out.Findings = append(out.Findings, Finding{TaskID: call.Task.ID, File: *finding.File, Observation: *finding.Observation})
	}
	return out, nil
}
