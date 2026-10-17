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
		case ExecutionDone:
			normalizeReviewFindingFiles(tasks, &review.ReviewResult)
			err = ValidateReview(tasks, review.ReviewResult)
		case ExecutionQuestion, ExecutionPermission, ExecutionError:
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
	case ExecutionError:
		return ReviewResult{}, result, &IncompleteTaskError{Reason: "Review is incomplete: " + response, ThreadID: result.ThreadID, TurnID: result.TurnID}
	default:
		return review.ReviewResult, result, nil
	}
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
