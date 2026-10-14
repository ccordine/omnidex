package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

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
	document := string(content)
	if len(content) > 0 && content[0] == '"' {
		if err := json.Unmarshal(content, &document); err != nil {
			return nil, fmt.Errorf("decode native review text: %w", err)
		}
	}
	raw, comments, err := splitNativeReviewDocument(document)
	if err != nil {
		return nil, fmt.Errorf("decode native agent review: %w", err)
	}
	review, directErr := decodeAgentReviewReceipt(call, raw)
	if directErr != nil {
		var envelope struct {
			Findings []struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			} `json:"findings"`
			OverallExplanation string `json:"overall_explanation"`
			OverallCorrectness string `json:"overall_correctness"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("decode native agent review: %w", directErr)
		}
		receipt, details, err := splitNativeReviewDocument(envelope.OverallExplanation)
		if err == nil {
			review, err = decodeAgentReviewReceipt(call, receipt)
		}
		if err != nil {
			return nil, fmt.Errorf("native agent review omitted its exact task and criterion receipt: %w", err)
		}
		if len(envelope.Findings) > len(review.Findings) {
			return nil, fmt.Errorf("native agent review omitted referenced findings from its structured receipt")
		}
		for index, finding := range envelope.Findings {
			detail := strings.TrimSpace(strings.TrimSpace(finding.Title) + ": " + strings.TrimSpace(finding.Body))
			if detail != ":" && detail != "" {
				review.Findings[index].Observation = strings.TrimSpace(review.Findings[index].Observation + "\n\nNative review detail: " + detail)
			}
		}
		if details != "" {
			comments = strings.TrimSpace(details + "\n\n" + comments)
		}
		if envelope.OverallCorrectness != "" && envelope.OverallCorrectness != "patch is correct" && len(review.Findings) == 0 {
			return nil, fmt.Errorf("native agent review reported %q without a referenced finding", envelope.OverallCorrectness)
		}
	}
	if comments != "" {
		if len(review.Findings) == 0 {
			return nil, fmt.Errorf("native agent review returned unreferenced full review comments")
		}
		review.Findings[0].Observation = strings.TrimSpace(review.Findings[0].Observation + "\n\nFull review comments:\n" + comments)
	}
	return json.Marshal(review)
}

func splitNativeReviewDocument(text string) (json.RawMessage, string, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, "", err
	}
	tail := strings.TrimSpace(text[decoder.InputOffset():])
	if tail == "" {
		return raw, "", nil
	}
	for _, heading := range []string{"Full review comments:", "Review comments:", "Review comment:"} {
		if strings.HasPrefix(tail, heading) {
			return raw, strings.TrimSpace(strings.TrimPrefix(tail, heading)), nil
		}
	}
	return nil, "", errors.New("unexpected text after native review result")
}

func decodeAgentReviewReceipt(call codingagent.Assignment, content json.RawMessage) (codingagent.ReviewResult, error) {
	var native struct {
		Findings     []nativeAgentReviewFinding `json:"findings"`
		Questions    []string                   `json:"questions"`
		Verification string                     `json:"verification,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&native); err != nil {
		return codingagent.ReviewResult{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return codingagent.ReviewResult{}, fmt.Errorf("native agent review contains trailing data")
	}
	if native.Findings == nil || native.Questions == nil {
		return codingagent.ReviewResult{}, fmt.Errorf("native agent review requires findings and questions arrays")
	}
	tasks := map[string]agentReviewTaskReference{}
	for _, task := range agentReviewReferences(call) {
		tasks[task.Reference] = task
	}
	review := codingagent.ReviewResult{Findings: make([]codingagent.Finding, 0, len(native.Findings)), Questions: native.Questions, Verification: native.Verification}
	for _, finding := range native.Findings {
		task, known := tasks[finding.TaskReference]
		if !known || finding.File == nil || finding.Observation == nil {
			return codingagent.ReviewResult{}, fmt.Errorf("native agent review finding lacks its exact task reference or result fields")
		}
		criteria := map[string]string{}
		for _, criterion := range task.Criteria {
			criteria[criterion.Reference] = criterion.Requirement
		}
		requirement, known := criteria[finding.CriterionReference]
		if !known {
			return codingagent.ReviewResult{}, fmt.Errorf("native agent review finding has an unknown criterion reference for its task")
		}
		review.Findings = append(review.Findings, codingagent.Finding{
			TaskID: task.TaskID, Requirement: requirement, File: *finding.File, Observation: *finding.Observation,
		})
	}
	return review, nil
}
