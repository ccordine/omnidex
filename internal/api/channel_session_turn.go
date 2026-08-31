package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const maxChannelSessionTurnBodyBytes int64 = 64 * 1024

type channelSessionTurnRequest struct {
	WorkspaceRoot     string                     `json:"workspace_root"`
	WorkspaceIdentity string                     `json:"workspace_identity"`
	Text              string                     `json:"text"`
	OperationID       queue.LifecycleOperationID `json:"operation_id"`
}

type channelSessionTurnResponse struct {
	OperationID       queue.LifecycleOperationID          `json:"operation_id"`
	Disposition       queue.ChannelSessionTurnDisposition `json:"disposition"`
	ChannelID         model.ChannelID                     `json:"channel_id"`
	WorkspaceRoot     string                              `json:"workspace_root"`
	WorkspaceIdentity string                              `json:"workspace_identity"`
	JobID             int64                               `json:"job_id"`
	Status            string                              `json:"status"`
	Generation        int64                               `json:"generation"`
	UserMessage       *model.ChannelMessage               `json:"user_message,omitempty"`
}

func (s *Server) postChannelSessionTurn(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
) {
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var request channelSessionTurnRequest
	if err := decodeExactChannelJSON(
		w,
		r,
		"channel session turn request",
		maxChannelSessionTurnBodyBytes,
		&request,
	); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	operationID, err := queue.ParseLifecycleOperationID(string(request.OperationID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.OperationID = operationID
	if err := model.ValidateChannelWorkspaceRoot(request.WorkspaceRoot); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireServerWorkspaceIdentity(
		request.WorkspaceRoot,
		request.WorkspaceIdentity,
	); err != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("session workspace authority: %v", err))
		return
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, request.Text); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.repo.SubmitChannelSessionTurn(
		r.Context(),
		queue.ChannelSessionTurnCommand{
			OperationID:       operationID,
			ChannelID:         channelID,
			WorkspaceRoot:     request.WorkspaceRoot,
			WorkspaceIdentity: request.WorkspaceIdentity,
			Text:              request.Text,
		},
	)
	if err != nil {
		writeError(w, channelSessionTurnStatus(err), err.Error())
		return
	}
	response, err := channelSessionTurnReceipt(channelID, request, result)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	phase, summary := realtimeJobChanged, "Interactive turn accepted"
	if result.Disposition == queue.ChannelSessionTurnEnqueued {
		phase, summary = realtimeJobQueued, "Job queued"
	}
	if result.Applied {
		s.publishChannelJobProgress(result.ChannelID, result.Job.ID, phase, summary)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, response)
}

func channelSessionTurnStatus(err error) int {
	switch {
	case errors.Is(err, queue.ErrChannelSessionNotFound):
		return http.StatusNotFound
	case errors.Is(err, queue.ErrChannelSessionTurnInvalid):
		return http.StatusBadRequest
	case errors.Is(err, queue.ErrChannelSessionAuthority),
		errors.Is(err, queue.ErrChannelSessionWorkspace),
		errors.Is(err, queue.ErrObjectiveSessionContextCapacity),
		errors.Is(err, queue.ErrLifecycleOperationConflict),
		errors.Is(err, queue.ErrStepNotWritable),
		errors.Is(err, queue.ErrStaleJobGeneration):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func channelSessionTurnReceipt(
	expectedChannelID model.ChannelID,
	request channelSessionTurnRequest,
	result queue.ChannelSessionTurnResult,
) (channelSessionTurnResponse, error) {
	if result.OperationID != request.OperationID || result.ChannelID != expectedChannelID ||
		result.WorkspaceRoot != request.WorkspaceRoot ||
		result.WorkspaceIdentity != request.WorkspaceIdentity || result.Job.ID < 1 ||
		result.Job.CurrentGeneration < 1 {
		return channelSessionTurnResponse{}, fmt.Errorf(
			"channel session turn result differs from exact request authority",
		)
	}
	switch result.Disposition {
	case queue.ChannelSessionTurnEnqueued,
		queue.ChannelSessionTurnReplanned,
		queue.ChannelSessionTurnFeedback:
	default:
		return channelSessionTurnResponse{}, fmt.Errorf(
			"channel session turn returned unregistered disposition %q",
			result.Disposition,
		)
	}
	switch result.Job.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
	default:
		return channelSessionTurnResponse{}, fmt.Errorf(
			"channel session turn returned unregistered job status %q",
			result.Job.Status,
		)
	}
	if result.Disposition == queue.ChannelSessionTurnEnqueued {
		if result.UserMessage == nil || result.UserMessage.ChannelID != expectedChannelID ||
			result.UserMessage.Role != model.ChannelMessageRoleUser ||
			result.UserMessage.Content != request.Text {
			return channelSessionTurnResponse{}, fmt.Errorf(
				"channel session enqueue result has no exact user message",
			)
		}
	} else if result.UserMessage != nil {
		return channelSessionTurnResponse{}, fmt.Errorf(
			"channel session control unexpectedly returned a user message",
		)
	}
	return channelSessionTurnResponse{
		OperationID:       result.OperationID,
		Disposition:       result.Disposition,
		ChannelID:         result.ChannelID,
		WorkspaceRoot:     result.WorkspaceRoot,
		WorkspaceIdentity: result.WorkspaceIdentity,
		JobID:             result.Job.ID,
		Status:            result.Job.Status,
		Generation:        result.Job.CurrentGeneration,
		UserMessage:       result.UserMessage,
	}, nil
}
