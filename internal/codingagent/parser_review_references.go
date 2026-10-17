package codingagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type ReviewCriterionReference struct {
	Reference   string `json:"reference"`
	Requirement string `json:"requirement"`
}

type ReviewTaskReference struct {
	Reference string                     `json:"reference"`
	TaskID    string                     `json:"task_id"`
	Criteria  []ReviewCriterionReference `json:"criteria"`
}

func ReviewReferences(call Assignment) []ReviewTaskReference {
	tasks := []Task{}
	if call.Role == ReviewResponse && call.Task != nil {
		tasks = append(tasks, *call.Task)
	}
	references := make([]ReviewTaskReference, 0, len(tasks))
	for index, task := range tasks {
		reference := ReviewTaskReference{Reference: fmt.Sprintf("task_%d", index+1), TaskID: task.ID,
			Criteria: []ReviewCriterionReference{}}
		seen := map[string]bool{}
		appendCriterion := func(id, requirement string) {
			if !seen[requirement] {
				reference.Criteria = append(reference.Criteria, ReviewCriterionReference{Reference: id, Requirement: requirement})
				seen[requirement] = true
			}
		}
		for criterion, requirement := range task.Acceptance {
			appendCriterion(fmt.Sprintf("criterion_%d", criterion+1), requirement)
		}
		references = append(references, reference)
	}
	return references
}

type nativeAgentReviewFinding struct {
	CriterionReference string  `json:"requirement"`
	File               *string `json:"file"`
	Observation        *string `json:"observation"`
}

func MapParserResponse(call Assignment, content json.RawMessage) (json.RawMessage, error) {
	if call.Role != ReviewResponse {
		return content, nil
	}
	review, err := DecodeReviewResponse(call, content)
	if err != nil {
		return nil, fmt.Errorf("decode task-local review response parser: %w", err)
	}
	return json.Marshal(review)
}

func DecodeReviewResponse(call Assignment, content json.RawMessage) (ReviewResponseResult, error) {
	var native struct {
		Kind         ExecutionResponseKind      `json:"kind"`
		Findings     []nativeAgentReviewFinding `json:"findings"`
		Verification string                     `json:"verification,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&native); err != nil {
		return ReviewResponseResult{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ReviewResponseResult{}, fmt.Errorf("review parser contains trailing data")
	}
	if native.Findings == nil {
		return ReviewResponseResult{}, fmt.Errorf("review parser requires a findings array")
	}
	references := ReviewReferences(call)
	if len(references) != 1 {
		return ReviewResponseResult{}, fmt.Errorf("review parser requires one code-selected task")
	}
	task := references[0]
	review := ReviewResponseResult{Kind: native.Kind, ReviewResult: ReviewResult{Findings: make([]Finding, 0, len(native.Findings)), Verification: native.Verification}}
	for _, finding := range native.Findings {
		if finding.File == nil || finding.Observation == nil {
			return ReviewResponseResult{}, fmt.Errorf("review parser finding lacks its exact result fields")
		}
		criteria := map[string]string{}
		for _, criterion := range task.Criteria {
			criteria[criterion.Reference] = criterion.Requirement
		}
		requirement, known := criteria[finding.CriterionReference]
		if !known {
			return ReviewResponseResult{}, fmt.Errorf("review parser finding has an unknown criterion reference for its task")
		}
		review.Findings = append(review.Findings, Finding{
			TaskID: task.TaskID, Requirement: requirement, File: *finding.File, Observation: *finding.Observation,
		})
	}
	return review, nil
}
