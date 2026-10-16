package codingagent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Invalid proposals are corrected within their role, without expanding grants.
func (controller *Controller) review(ctx context.Context, scope Assignment, tasks map[string]Task, round int64) (ReviewResult, Result, error) {
	for attempt := 0; attempt < 3 && ctx.Err() == nil; attempt++ {
		result, err := controller.invoke(ctx, scope, round)
		if err != nil {
			return ReviewResult{}, result, err
		}
		var review ReviewResult
		err = decodeContract(result.Content, &review)
		if err == nil {
			normalizeReviewFindingFiles(tasks, &review)
			err = ValidateReview(tasks, review, scope.Requirements...)
		}
		if err == nil {
			return review, result, nil
		}
		scope.ValidationNotes = []string{err.Error()}
		taskID := ""
		if scope.Task != nil {
			taskID = scope.Task.ID
		}
		if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "review_rejected", TaskID: taskID, CallID: result.CallID, Role: scope.Role, Notes: scope.ValidationNotes}); err != nil {
			return ReviewResult{}, result, err
		}
	}
	if err := ctx.Err(); err != nil {
		return ReviewResult{}, Result{}, err
	}
	return ReviewResult{}, Result{}, fmt.Errorf("review did not return a valid task-local judgment after three attempts")
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
