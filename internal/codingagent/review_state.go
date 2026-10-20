package codingagent

func (state *taskState) projectReviewEntry(entry Entry) {
	switch entry.Phase {
	case "task_executing":
		state.pendingReview = nil
		state.reviewSession = nil
		state.reviewQuestion, state.reviewReply = "", ""
	case "call_completed":
		if entry.Role == Review && entry.Result != nil {
			state.pendingReview = entry.Result
		}
	case "review_response":
		if len(entry.Notes) != 1 {
			return
		}
		if entry.ResponseKind == ExecutionError {
			state.reviewQuestion = entry.Notes[0]
		}
		if entry.ResponseKind == ExecutionQuestion || entry.ResponseKind == ExecutionPermission {
			state.reviewQuestion = entry.Notes[0]
			state.inputRequired = &ExecutionInputRequired{TaskID: entry.TaskID, Role: Review, Kind: entry.ResponseKind, Text: entry.Notes[0]}
		}
	case "execution_reply":
		if entry.Role != Review || entry.NativeInput != nil || len(entry.Notes) != 2 {
			return
		}
		state.reviewReply = entry.Notes[1]
		state.pendingReview = nil
		state.reviewSession = nil
		state.inputRequired = nil
	}
}
