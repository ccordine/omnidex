package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Parsing failures never repeat the native review or its environment operations.
func (controller *Controller) review(ctx context.Context, scope Assignment, round int64, pending *Result) (ReviewResult, Result, error) {
	taskID := ""
	if scope.Task != nil {
		taskID = scope.Task.ID
	}
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
	parser.Task, parser.Objective, parser.Response = scope.Task, scope.Objective, response
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return ReviewResult{}, result, err
	}
	for _, entry := range entries {
		if entry.Phase == "review_response" && entry.CallID == result.CallID &&
			(entry.ResponseKind == ExecutionDone || entry.ResponseKind == ExecutionError) &&
			entry.ReviewResponse != nil && len(entry.Notes) == 1 && entry.Notes[0] == response {
			return reviewTaskResult(taskID, *entry.ReviewResponse, response), result, nil
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
			review.ReviewResult = reviewTaskResult(taskID, review, response)
		case ExecutionQuestion, ExecutionPermission:
		default:
			err = fmt.Errorf("review response parser returned unknown kind %q", review.Kind)
		}
	}
	if err != nil {
		if _, saveErr := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "review_parse_rejected", TaskID: taskID,
			CallID: result.CallID, ParserCallID: parsed.CallID, Round: round, Role: ReviewResponse, Notes: []string{err.Error()}}); saveErr != nil {
			return ReviewResult{}, result, saveErr
		}
		return ReviewResult{}, result, &ParserFailure{Cause: err}
	}
	_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "review_response", TaskID: taskID,
		CallID: result.CallID, ParserCallID: parsed.CallID, Round: round, Role: Review, ResponseKind: review.Kind,
		ReviewResponse: &review, Notes: []string{response}})
	if err != nil {
		return ReviewResult{}, result, err
	}
	switch review.Kind {
	case ExecutionQuestion, ExecutionPermission:
		return ReviewResult{}, result, &ExecutionInputRequired{TaskID: taskID, Role: Review, Kind: review.Kind, Text: response}
	default:
		return review.ReviewResult, result, nil
	}
}

// ReviewGoal runs once the database has no incomplete ledger items left.
func (controller *Controller) ReviewGoal(ctx context.Context, objective Objective) error {
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return err
	}
	state := &taskState{}
	for _, entry := range entries {
		if entry.TaskID == "" {
			state.projectReviewEntry(entry)
		}
	}
	if state.inputRequired != nil {
		return state.inputRequired
	}
	call := controller.assignment(Review, nil, controller.Configuration.Reviewer)
	call.Objective = &objective
	call.Session = state.reviewSession
	call.Response, call.Reply = state.reviewQuestion, state.reviewReply
	review, result, err := controller.review(ctx, call, 0, state.pendingReview)
	if err != nil {
		return err
	}
	if len(review.Findings) != 0 {
		return &IncompleteTaskError{Reason: review.Verification, ThreadID: result.ThreadID, TurnID: result.TurnID}
	}
	return nil
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
