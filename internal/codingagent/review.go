package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Parsing failures never repeat the native review or its environment operations.
func (controller *Controller) review(ctx context.Context, scope Assignment, tasks map[string]Task, round int64, pending *Result) (ReviewResult, Result, error) {
	var result Result
	if pending != nil {
		result = *pending
	} else {
		var err error
		result, err = controller.invoke(ctx, scope, round)
		if err != nil {
			return ReviewResult{}, result, err
		}
	}
	var response string
	if err := json.Unmarshal(result.Content, &response); err != nil || strings.TrimSpace(response) == "" {
		return ReviewResult{}, result, fmt.Errorf("review requires an ordinary nonblank Codex response")
	}
	parser := controller.assignment(ReviewResponse, nil, controller.Configuration.LedgerParser)
	parser.Task, parser.Response = scope.Task, response
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return ReviewResult{}, result, err
	}
	for _, entry := range entries {
		if entry.Phase == "review_response" && entry.CallID == result.CallID &&
			(entry.ResponseKind == ExecutionDone || entry.ResponseKind == ExecutionError) &&
			entry.ReviewResponse != nil && len(entry.Notes) == 1 && entry.Notes[0] == response {
			review := reviewTaskResult(scope.Task.ID, *entry.ReviewResponse, response)
			if err := ValidateReview(tasks, review); err != nil {
				return ReviewResult{}, result, err
			}
			return review, result, nil
		}
		if entry.Phase == "review_parse_rejected" && entry.CallID == result.CallID {
			parser.ValidationNotes = entry.Notes
		}
	}
	parsed, err := controller.invoke(ctx, parser, round)
	if err != nil {
		return ReviewResult{}, result, err
	}
	var review ReviewResponseResult
	err = decodeContract(parsed.Content, &review)
	if err == nil {
		switch review.Kind {
		case ExecutionDone, ExecutionError:
			review.ReviewResult = reviewTaskResult(scope.Task.ID, review, response)
			normalizeReviewFindingFiles(tasks, &review.ReviewResult)
			err = ValidateReview(tasks, review.ReviewResult)
		case ExecutionQuestion, ExecutionPermission:
		default:
			err = fmt.Errorf("review response parser returned unknown kind %q", review.Kind)
		}
	}
	if err != nil {
		if _, saveErr := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "review_parse_rejected", TaskID: scope.Task.ID,
			CallID: result.CallID, ParserCallID: parsed.CallID, Round: round, Role: ReviewResponse, Notes: []string{err.Error()}}); saveErr != nil {
			return ReviewResult{}, result, saveErr
		}
		return ReviewResult{}, result, &ParserFailure{Cause: err}
	}
	_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "review_response", TaskID: scope.Task.ID,
		CallID: result.CallID, ParserCallID: parsed.CallID, Round: round, Role: Review, ResponseKind: review.Kind,
		ReviewResponse: &review, Notes: []string{response}})
	if err != nil {
		return ReviewResult{}, result, err
	}
	switch review.Kind {
	case ExecutionQuestion, ExecutionPermission:
		return ReviewResult{}, result, &ExecutionInputRequired{TaskID: scope.Task.ID, Role: Review, Kind: review.Kind, Text: response}
	default:
		return review.ReviewResult, result, nil
	}
}

func reviewTaskResult(taskID string, review ReviewResponseResult, response string) ReviewResult {
	if review.Kind == ExecutionError && len(review.Findings) == 0 {
		// A completed reviewer response describing unfinished verification is
		// retained task feedback. When no findings were extracted, give the
		// executor the exact report through the existing correction loop.
		// Only a later independent review can accept the task.
		return ReviewResult{Findings: []Finding{{TaskID: taskID, Observation: response}}, Verification: response}
	}
	return review.ReviewResult
}

// Reviewers commonly attach source locations to a file name. Preserve the
// exact task grant as the contract value, but only when the prefix is itself
// an authorized file and the suffix is a strictly numeric source location.
func normalizeReviewFindingFiles(tasks map[string]Task, result *ReviewResult) {
	for index := range result.Findings {
		finding := &result.Findings[index]
		task, known := tasks[finding.TaskID]
		if !known || finding.File == "" {
			continue
		}
		allowed := append(append(append([]string{}, task.WriteFiles...), task.ReadFiles...), task.DeleteFiles...)
		if contains(allowed, finding.File) {
			continue
		}
		for _, file := range allowed {
			location, found := strings.CutPrefix(finding.File, file+":")
			if found && validSourceLocation(location) {
				finding.File = file
				break
			}
		}
	}
}

func validSourceLocation(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) == 0 || len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		coordinates := strings.Split(part, ":")
		if len(coordinates) == 0 || len(coordinates) > 2 {
			return false
		}
		for _, coordinate := range coordinates {
			number, err := strconv.ParseUint(coordinate, 10, 64)
			if err != nil || number == 0 {
				return false
			}
		}
	}
	return true
}
