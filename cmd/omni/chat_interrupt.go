package main

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func (session *chatSession) currentJobInterrupted() bool {
	if session.active == nil || session.active.Job.Status != model.JobStatusWaiting {
		return false
	}
	for _, control := range session.controls {
		if control.Kind == model.ChannelSessionControlInterrupt && control.JobID == session.active.Job.ID &&
			control.Generation == session.active.Job.CurrentGeneration {
			return true
		}
	}
	return false
}

func (session *chatSession) resolveOperationError(err error) (quit bool, resultErr error) {
	if err == nil {
		return false, nil
	}
	if errors.Is(err, errChatTerminated) {
		session.cancel()
		return true, nil
	}
	if !errors.Is(err, errChatRequestInterrupted) {
		return false, session.renderer.system("%v", err)
	}
	if session.pendingIntent != nil {
		err := session.resolvePendingUserIntent()
		return session.exitRequested, err
	}
	if err := session.renderer.system("request interrupted; reconciling server state"); err != nil {
		return false, err
	}
	resolution, err := session.resolvePendingOperation()
	if err != nil {
		if errors.Is(err, errChatTerminated) {
			session.cancel()
			return true, nil
		}
		if !definitiveChatRequestFailure(err) {
			return false, err
		}
		// A definitive response proves the ambiguous operation did not commit
		// under this exact identity. Report that failure, release the local
		// idempotency guard, and continue resolving the user's Ctrl-C against
		// current persisted state.
		session.pendingTurn = nil
		session.pendingControl = nil
		session.pendingPlan = nil
		resolution = nil
		if renderErr := session.renderer.system("pending operation was rejected: %v", err); renderErr != nil {
			return false, renderErr
		}
	}
	if err := session.reloadSnapshotAuthoritatively(); err != nil {
		if errors.Is(err, errChatTerminated) {
			session.cancel()
			return true, nil
		}
		return false, err
	}
	if err := session.acknowledgePendingResolution(resolution); err != nil {
		return false, err
	}
	if resolution != nil {
		switch resolution.action {
		case "interrupt":
			return false, session.renderer.system(
				"job %d paused; ask to continue this job or redirect it",
				resolution.jobID,
			)
		case "cancel":
			return false, session.renderer.system("job %d is canceled", resolution.jobID)
		}
	}
	if session.active == nil {
		if resolution != nil {
			return false, session.renderer.system(
				"job %d reached terminal state before interruption",
				resolution.jobID,
			)
		}
		return false, session.renderer.system("no active server job remains to interrupt")
	}
	if session.active.Job.Status == model.JobStatusWaiting {
		return false, session.renderer.system("job %d is already waiting for you; no work is running", session.active.Job.ID)
	}
	targetJobID := session.active.Job.ID
	if resolution != nil {
		targetJobID = resolution.jobID
		if session.active.Job.ID != targetJobID {
			return false, session.renderer.system(
				"resolved job %d is no longer active; current job %d was not interrupted",
				targetJobID,
				session.active.Job.ID,
			)
		}
	}
	reason := ctrlCInterruptReason
	if session.pendingControl != nil && session.pendingControl.action == "interrupt" {
		reason = session.pendingControl.exactText
	}
	jobID, err := session.control("interrupt", reason, false, &targetJobID)
	if err != nil {
		if errors.Is(err, errChatTerminated) {
			session.cancel()
			return true, nil
		}
		if errors.Is(err, errChatRequestInterrupted) {
			return false, session.renderer.system(
				"interrupt request was itself interrupted; press Ctrl-C to retry",
			)
		}
		// Planning can finish between the authoritative read and the interrupt.
		// Only a definitive state rejection permits dropping this operation;
		// transport uncertainty and authorization failures retain their errors.
		if client.IsHTTPStatus(err, http.StatusBadRequest) || client.IsHTTPStatus(err, http.StatusConflict) {
			if reloadErr := session.reloadSnapshotAuthoritatively(); reloadErr == nil && session.active != nil &&
				session.active.Job.ID == targetJobID && session.active.Job.Status == model.JobStatusWaiting {
				session.pendingControl = nil
				return false, session.renderer.system("job %d is already waiting for you; no work is running", targetJobID)
			}
		}
		return false, err
	}
	return false, session.renderer.system(
		"job %d paused; ask to continue this job or redirect it",
		jobID,
	)
}
