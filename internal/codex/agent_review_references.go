package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/codingagent"
)

type agentReviewCriterionReference struct {
	Reference   string `json:"reference"`
	Requirement string `json:"requirement"`
}

type agentReviewTaskReference struct {
	Reference string                          `json:"reference"`
	TaskID    string                          `json:"task_id"`
	Criteria  []agentReviewCriterionReference `json:"criteria"`
}

func agentReviewReferences(call codingagent.Assignment) []agentReviewTaskReference {
	tasks := []codingagent.Task{}
	if call.Role == codingagent.ReviewResponse && call.Task != nil {
		tasks = append(tasks, *call.Task)
	}
	references := make([]agentReviewTaskReference, 0, len(tasks))
	for index, task := range tasks {
		reference := agentReviewTaskReference{Reference: fmt.Sprintf("task_%d", index+1), TaskID: task.ID,
			Criteria: []agentReviewCriterionReference{}}
		seen := map[string]bool{}
		appendCriterion := func(id, requirement string) {
			if !seen[requirement] {
				reference.Criteria = append(reference.Criteria, agentReviewCriterionReference{Reference: id, Requirement: requirement})
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

func mapAgentReviewResponse(call codingagent.Assignment, content json.RawMessage) (json.RawMessage, error) {
	if call.Role != codingagent.ReviewResponse {
		return content, nil
	}
	review, err := decodeAgentReviewResponse(call, content)
	if err != nil {
		return nil, fmt.Errorf("decode task-local review response parser: %w", err)
	}
	return json.Marshal(review)
}

func decodeAgentReviewResponse(call codingagent.Assignment, content json.RawMessage) (codingagent.ReviewResponseResult, error) {
	var native struct {
		Kind         codingagent.ExecutionResponseKind `json:"kind"`
		Findings     []nativeAgentReviewFinding        `json:"findings"`
		Verification string                            `json:"verification,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&native); err != nil {
		return codingagent.ReviewResponseResult{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return codingagent.ReviewResponseResult{}, fmt.Errorf("review parser contains trailing data")
	}
	if native.Findings == nil {
		return codingagent.ReviewResponseResult{}, fmt.Errorf("review parser requires a findings array")
	}
	references := agentReviewReferences(call)
	if len(references) != 1 {
		return codingagent.ReviewResponseResult{}, fmt.Errorf("review parser requires one code-selected task")
	}
	task := references[0]
	review := codingagent.ReviewResponseResult{Kind: native.Kind, ReviewResult: codingagent.ReviewResult{Findings: make([]codingagent.Finding, 0, len(native.Findings)), Verification: native.Verification}}
	for _, finding := range native.Findings {
		if finding.File == nil || finding.Observation == nil {
			return codingagent.ReviewResponseResult{}, fmt.Errorf("review parser finding lacks its exact result fields")
		}
		criteria := map[string]string{}
		for _, criterion := range task.Criteria {
			criteria[criterion.Reference] = criterion.Requirement
		}
		requirement, known := criteria[finding.CriterionReference]
		if !known {
			return codingagent.ReviewResponseResult{}, fmt.Errorf("review parser finding has an unknown criterion reference for its task")
		}
		review.Findings = append(review.Findings, codingagent.Finding{
			TaskID: task.TaskID, Requirement: requirement, File: *finding.File, Observation: *finding.Observation,
		})
	}
	return review, nil
}
