package main

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type pendingAgentAnswer struct {
	jobID int64
	pause model.CodingAgentPause
}

type pendingChatJob struct{ jobID, generation int64 }

func observedAgentPause(details *model.JobDetails) (*pendingAgentAnswer, error) {
	if details == nil || details.AgentPause == nil {
		return nil, nil
	}
	pause := details.AgentPause
	if details.Job.ID < 1 || details.Job.Status != model.JobStatusWaiting ||
		pause.Schema != "omnidex-coding-agent-pause-v1" ||
		pause.Generation != details.Job.CurrentGeneration || pause.StepID < 1 ||
		pause.WaitingAttempt < 1 || len(pause.Questions) == 0 {
		return nil, fmt.Errorf("displayed question differs from the active waiting job")
	}
	copy := *pause
	copy.Questions = append([]string(nil), pause.Questions...)
	return &pendingAgentAnswer{jobID: details.Job.ID, pause: copy}, nil
}

func (session *chatSession) submitPendingTurn(ctx context.Context, pending *pendingSessionTurn) (client.SessionTurnReceipt, error) {
	if pending == nil {
		return client.SessionTurnReceipt{}, fmt.Errorf("session turn lacks pending operation authority")
	}
	if pending.answer == nil {
		if job := pending.activeJob; job != nil {
			return session.client.SubmitObservedSessionTurn(ctx, session.channel, session.workspaceIdentity,
				pending.operationID, pending.exactText, pending.codingMode, job.jobID, job.generation)
		}
		return session.client.SubmitSessionTurn(ctx, session.channel, session.workspaceIdentity,
			pending.operationID, pending.exactText, pending.codingMode)
	}
	answer := pending.answer
	job, err := session.client.AnswerAgentQuestion(ctx, session.channel, session.workspaceIdentity,
		answer.jobID, pending.operationID, pending.exactText, &answer.pause)
	if err != nil {
		return client.SessionTurnReceipt{}, err
	}
	return client.SessionTurnReceipt{
		OperationID: pending.operationID, Disposition: model.ChannelSessionTurnFeedback,
		ChannelID: session.channel.ID, WorkspaceRoot: session.channel.WorkspaceRoot,
		WorkspaceIdentity: session.workspaceIdentity, JobID: job.ID,
		Generation: answer.pause.Generation, Status: job.Status,
	}, nil
}

func validatePendingSessionTurn(turn model.ChannelSessionTurn, pending *pendingSessionTurn) error {
	if turn.Text != pending.exactText {
		return fmt.Errorf("persisted session turn differs from pending operation authority")
	}
	if answer := pending.answer; answer != nil &&
		(turn.Disposition != model.ChannelSessionTurnFeedback || turn.JobID != answer.jobID ||
			turn.Generation != answer.pause.Generation) {
		return fmt.Errorf("persisted answer differs from the displayed question authority")
	}
	if job := pending.activeJob; job != nil && (turn.Disposition != model.ChannelSessionTurnQueued || turn.JobID != job.jobID || turn.Generation != job.generation) {
		return fmt.Errorf("persisted message differs from the observed job authority")
	}
	return nil
}

func (session *chatSession) renderPendingTurnAccepted(receipt client.SessionTurnReceipt, pending *pendingSessionTurn) error {
	if pending.answer == nil || session.renderer.verbosity == chatDebug {
		return session.renderTurnAccepted(receipt)
	}
	return session.renderer.console.WriteOutput("omni> Answer received.\n")
}
