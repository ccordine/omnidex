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
	if call.Role == codingagent.Review && call.Task != nil {
		tasks = append(tasks, *call.Task)
	}
	if call.Role == codingagent.OverallReview && call.Workload != nil {
		tasks = call.Workload.Tasks
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
		if call.Role == codingagent.OverallReview {
			bound := map[string]bool{}
			for _, id := range task.RequirementIDs {
				bound[id] = true
			}
			for parent, requirement := range call.Requirements {
				if bound[requirement.ID] {
					appendCriterion(fmt.Sprintf("parent_%d", parent+1), requirement.Outcome)
				}
			}
		}
		references = append(references, reference)
	}
	return references
}

type nativeAgentReviewFinding struct {
	TaskReference      string  `json:"task_id"`
	CriterionReference string  `json:"requirement"`
	File               *string `json:"file"`
	Observation        *string `json:"observation"`
}

func mapAgentReviewResult(call codingagent.Assignment, content json.RawMessage) (json.RawMessage, error) {
	if call.Role != codingagent.Review && call.Role != codingagent.OverallReview {
		return content, nil
	}
	var native struct {
		Findings  []nativeAgentReviewFinding `json:"findings"`
		Questions []string                   `json:"questions"`
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&native); err != nil {
		return nil, fmt.Errorf("decode native agent review: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("native agent review contains trailing data")
	}
	if native.Findings == nil || native.Questions == nil {
		return nil, fmt.Errorf("native agent review requires findings and questions arrays")
	}
	tasks := map[string]agentReviewTaskReference{}
	for _, task := range agentReviewReferences(call) {
		tasks[task.Reference] = task
	}
	review := codingagent.ReviewResult{Findings: make([]codingagent.Finding, 0, len(native.Findings)), Questions: native.Questions}
	for _, finding := range native.Findings {
		task, known := tasks[finding.TaskReference]
		if !known || finding.File == nil || finding.Observation == nil {
			return nil, fmt.Errorf("native agent review finding lacks its exact task reference or result fields")
		}
		criteria := map[string]string{}
		for _, criterion := range task.Criteria {
			criteria[criterion.Reference] = criterion.Requirement
		}
		requirement, known := criteria[finding.CriterionReference]
		if !known {
			return nil, fmt.Errorf("native agent review finding has an unknown criterion reference for its task")
		}
		review.Findings = append(review.Findings, codingagent.Finding{
			TaskID: task.TaskID, Requirement: requirement, File: *finding.File, Observation: *finding.Observation,
		})
	}
	return json.Marshal(review)
}
