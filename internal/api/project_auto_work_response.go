package api

import (
	"log"
	"net/http"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

type projectAutoWorkCommitState string

const (
	projectAutoWorkCommitted         projectAutoWorkCommitState = "committed"
	projectAutoWorkCommittedDegraded projectAutoWorkCommitState = "committed_degraded"
)

type projectAutoWorkAuthority struct {
	AutoWork            ScrumAutoWorkConfig `json:"auto_work"`
	ActiveCardID        string              `json:"active_card_id"`
	ActiveCardUpdatedAt string              `json:"active_card_updated_at"`
	JobID               int64               `json:"job_id"`
	PausedCards         int                 `json:"paused_cards"`
}

type projectAutoWorkOutcome struct {
	Authority projectAutoWorkAuthority
	Board     *ScrumBoard
	Card      *ScrumCard
	PlayQueue map[string]any
	Message   string
}

type projectAutoWorkResponse struct {
	CommitState projectAutoWorkCommitState `json:"commit_state"`
	ProjectID   int64                      `json:"project_id"`
	projectAutoWorkAuthority
	Board          *ScrumBoard    `json:"board,omitempty"`
	Card           *ScrumCard     `json:"card,omitempty"`
	PlayQueue      map[string]any `json:"play_queue,omitempty"`
	Message        string         `json:"message"`
	OperationError string         `json:"operation_error,omitempty"`
}

func projectAutoWorkAuthorityFromResult(result queue.ProjectAutoWorkResult) projectAutoWorkAuthority {
	authority := projectAutoWorkAuthority{
		AutoWork: ScrumAutoWorkConfig{
			Enabled: result.Config.Enabled, SourceColumns: result.Config.SourceColumns,
		},
		JobID: result.JobID, PausedCards: result.PausedCards,
	}
	if result.ActiveCard != nil {
		authority.ActiveCardID = result.ActiveCard.ID
		authority.ActiveCardUpdatedAt = result.ActiveCard.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return authority
}

func writeProjectAutoWorkResponse(
	w http.ResponseWriter,
	projectID int64,
	outcome projectAutoWorkOutcome,
	postCommitErr error,
) {
	response := projectAutoWorkResponse{
		CommitState: projectAutoWorkCommitted, ProjectID: projectID,
		projectAutoWorkAuthority: outcome.Authority,
		Board:                    outcome.Board, Card: outcome.Card, PlayQueue: outcome.PlayQueue,
		Message: outcome.Message,
	}
	status := http.StatusOK
	if postCommitErr != nil {
		status = http.StatusMultiStatus
		response.CommitState = projectAutoWorkCommittedDegraded
		response.OperationError = postCommitErr.Error()
		log.Printf("project auto-work committed with degraded reconciliation project_id=%d error=%v", projectID, postCommitErr)
	}
	writeJSON(w, status, response)
}
