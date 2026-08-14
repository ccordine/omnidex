package api

type scrumCardDeleteResponse struct {
	CommitState       scrumCardMutationCommitState `json:"commit_state"`
	ProjectID         int64                        `json:"project_id"`
	CardID            string                       `json:"card_id"`
	ExpectedUpdatedAt string                       `json:"expected_updated_at"`
	Deleted           bool                         `json:"deleted"`
}
