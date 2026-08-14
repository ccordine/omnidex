package api

import (
	"log"
	"net/http"
)

type scrumAutoWorkCommitState string

const (
	scrumAutoWorkCommitted         scrumAutoWorkCommitState = "committed"
	scrumAutoWorkCommittedDegraded scrumAutoWorkCommitState = "committed_degraded"
)

type scrumAutoWorkMutationResponse struct {
	CommitState    scrumAutoWorkCommitState `json:"commit_state"`
	AutoWork       ScrumAutoWorkConfig      `json:"auto_work"`
	OperationError string                   `json:"operation_error,omitempty"`
}

func writeScrumAutoWorkMutationResponse(
	w http.ResponseWriter,
	projectID int64,
	config ScrumAutoWorkConfig,
	postCommitErr error,
) {
	response := scrumAutoWorkMutationResponse{
		CommitState: scrumAutoWorkCommitted,
		AutoWork:    config,
	}
	status := http.StatusOK
	if postCommitErr != nil {
		status = http.StatusMultiStatus
		response.CommitState = scrumAutoWorkCommittedDegraded
		response.OperationError = postCommitErr.Error()
		log.Printf(
			"Scrum auto-work settings committed with degraded reconciliation: project_id=%d enabled=%t error=%v",
			projectID,
			config.Enabled,
			postCommitErr,
		)
	}
	writeJSON(w, status, response)
}
