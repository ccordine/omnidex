package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *parserSieve) outcome() ExecutionResponseKind {
	present := map[string]bool{}
	for _, parts := range s.parts {
		for _, part := range parts {
			present[part.Kind] = true
		}
	}
	// A progress/completion claim cannot hide any unresolved owner interaction
	// or unfinished assignment. Review check failures that expose defects are
	// classified at the review semantic boundary, never remapped here because
	// findings happen to exist. Preserve the entire original final response.
	for _, kind := range []ExecutionResponseKind{ExecutionPermission, ExecutionQuestion, ExecutionError, ExecutionDone} {
		if present[string(kind)] {
			return kind
		}
	}
	return ExecutionError // Silence and progress alone do not establish completion.
}

type parserReviewFindings struct {
	Findings []struct {
		Quote string `json:"quote"`
		File  string `json:"file"`
	} `json:"findings"`
}

// Interpret only the reviewer's final prose against its frozen task. It receives no
// authority beyond the frozen task instructions, or performs another implementation review.
func (s *parserSieve) review(ctx context.Context) (json.RawMessage, error) {
	if s.call.Task == nil {
		return nil, fmt.Errorf("review response requires its code-selected task")
	}
	out := ReviewResponseResult{Kind: s.outcome(), ReviewResult: ReviewResult{Findings: []Finding{}, Verification: s.call.Response}}
	seen := map[string]bool{}
	for index, block := range s.blocks {
		if strings.TrimSpace(block.Text) == "" {
			continue
		}
		step := s.step(index, "review_findings")
		var parsed parserReviewFindings
		if err := s.leaf(ctx, step, &parsed); err != nil {
			return nil, err
		}
		if err := validateParserReviewFindings(step.Source, parsed); err != nil {
			return nil, err
		}
		for _, finding := range parsed.Findings {
			key := finding.File + "\x00" + finding.Quote
			if seen[key] {
				continue
			}
			seen[key] = true
			out.Findings = append(out.Findings, Finding{TaskID: s.call.Task.ID, File: finding.File, Observation: finding.Quote})
		}
	}
	return json.Marshal(out)
}
func validateParserReviewFindings(source string, result parserReviewFindings) error {
	if result.Findings == nil {
		return fmt.Errorf("review parser requires a findings array")
	}
	for _, finding := range result.Findings {
		if strings.TrimSpace(finding.Quote) == "" || !strings.Contains(source, finding.Quote) {
			return fmt.Errorf("review finding requires an exact source quote")
		}
		if finding.File != "" && !strings.Contains(source, finding.File) {
			return fmt.Errorf("review parser invented a filename")
		}
	}
	return nil
}
