package codingagent

import "context"

// Invalid proposals are corrected within their role, without expanding grants.
func (controller *Controller) review(ctx context.Context, scope Assignment, tasks map[string]Task, round int64) (ReviewResult, Result, error) {
	for ctx.Err() == nil {
		result, err := controller.invoke(ctx, scope, round)
		if err != nil {
			return ReviewResult{}, result, err
		}
		var review ReviewResult
		err = decodeContract(result.Content, &review)
		if err == nil {
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
	return ReviewResult{}, Result{}, ctx.Err()
}
