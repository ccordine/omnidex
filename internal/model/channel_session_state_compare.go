package model

// Equal compares the persisted values used by lightweight session polling.
// A revision hash cannot add information to these already available values.
func (state ChannelSessionState) Equal(other ChannelSessionState) bool {
	if state.ChannelID != other.ChannelID || state.WorkspaceRoot != other.WorkspaceRoot ||
		state.WorkspaceIdentity != other.WorkspaceIdentity ||
		!state.ChannelUpdatedAt.Equal(other.ChannelUpdatedAt) ||
		!equalSessionValue(state.LatestMessageID, other.LatestMessageID) ||
		!equalSessionValue(state.LatestTurnOperationID, other.LatestTurnOperationID) ||
		!equalSessionValue(state.LatestControlOperationID, other.LatestControlOperationID) {
		return false
	}
	if state.LatestJob == nil || other.LatestJob == nil {
		return state.LatestJob == nil && other.LatestJob == nil
	}
	left, right := state.LatestJob, other.LatestJob
	return left.ID == right.ID && left.Generation == right.Generation &&
		left.Status == right.Status && left.UpdatedAt.Equal(right.UpdatedAt)
}

func equalSessionValue[T comparable](left, right *T) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
