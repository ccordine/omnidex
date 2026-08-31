package main

import (
	"errors"
)

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
				"job %d interrupted; enter a redirection to resume",
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
		return false, err
	}
	return false, session.renderer.system(
		"job %d interrupted; enter a redirection to resume",
		jobID,
	)
}
