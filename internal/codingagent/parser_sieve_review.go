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
	// or required failure. Preserve the entire original final response for input.
	for _, kind := range []ExecutionResponseKind{ExecutionPermission, ExecutionQuestion, ExecutionError, ExecutionDone} {
		if present[string(kind)] {
			return kind
		}
	}
	return ExecutionError // Silence and progress alone do not establish completion.
}

type parserCriterionResult struct {
	Status   string `json:"status"`
	Quote    string `json:"quote"`
	Findings []struct {
		Quote string `json:"quote"`
		File  string `json:"file"`
	} `json:"findings"`
}

func (s *parserSieve) review(ctx context.Context) (json.RawMessage, error) {
	if s.call.Task == nil || len(s.call.Task.Acceptance) == 0 {
		return nil, fmt.Errorf("review sieve requires the exact task criteria")
	}
	result := ReviewResponseResult{Kind: s.outcome(), ReviewResult: ReviewResult{Findings: []Finding{}}}
	unverified := false
	for _, criterion := range s.call.Task.Acceptance {
		covered, blocked := false, false
		seen := map[string]bool{}
		for index := range s.blocks {
			if strings.TrimSpace(s.blocks[index].Text) == "" {
				continue
			}
			step := s.step(index, "review_criterion")
			step.Criterion = criterion
			var parsed parserCriterionResult
			if err := s.leaf(ctx, step, &parsed); err != nil {
				return nil, err
			}
			if err := validateParserCriterion(step.Source, parsed); err != nil {
				return nil, err
			}
			if parsed.Status == "absent" {
				continue
			}
			if err := s.receipt("criterion_evidence", map[string]any{"criterion": criterion, "block_id": step.BlockID, "result": parsed}); err != nil {
				return nil, err
			}
			covered = covered || parsed.Status == "pass" || parsed.Status == "defect"
			blocked = blocked || parsed.Status == "blocked"
			for _, finding := range parsed.Findings {
				key := finding.File + "\x00" + finding.Quote
				if seen[key] {
					continue
				}
				seen[key] = true
				result.Findings = append(result.Findings, Finding{TaskID: s.call.Task.ID, Requirement: criterion, File: finding.File, Observation: finding.Quote})
			}
		}
		unverified = unverified || !covered || blocked
	}
	if result.Kind == ExecutionDone && unverified {
		result.Kind = ExecutionError
	}
	// The verification projection retains original evidence, including optional
	// diagnostics. Criterion coverage above, not the existence of prose, gates done.
	result.Verification = s.call.Response
	return json.Marshal(result)
}

func validateParserCriterion(source string, result parserCriterionResult) error {
	if result.Findings == nil || !contains([]string{"absent", "blocked", "pass", "defect"}, result.Status) {
		return fmt.Errorf("invalid criterion disposition or missing findings")
	}
	if result.Status == "absent" {
		if result.Quote != "" || len(result.Findings) != 0 {
			return fmt.Errorf("absent criterion invented evidence")
		}
		return nil
	}
	if strings.TrimSpace(result.Quote) == "" || !strings.Contains(source, result.Quote) {
		return fmt.Errorf("criterion evidence is not an exact original source quote")
	}
	if (result.Status == "defect") != (len(result.Findings) > 0) {
		return fmt.Errorf("criterion status disagrees with its concrete findings")
	}
	for _, finding := range result.Findings {
		if strings.TrimSpace(finding.Quote) == "" || !strings.Contains(source, finding.Quote) {
			return fmt.Errorf("review finding has no exact source support")
		}
		if finding.File != "" && !strings.Contains(source, finding.File) {
			return fmt.Errorf("review parser invented a filename")
		}
	}
	return nil
}
