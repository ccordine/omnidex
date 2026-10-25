package queue

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/jackc/pgx/v5"
)

// Every continuation is derived by code from a validated before/after pair.
// All writes, including the receipt, use the caller's existing transaction.
func (r *Repository) applyUserIntentTx(ctx context.Context, tx pgx.Tx, saved storedUserIntent, current sessionintent.Context) (sessionintent.Receipt, error) {
	after, err := sessionintent.Apply(userIntentDisplayedContext(current, saved.request), saved.result.Patch)
	receipt := sessionintent.Receipt{OperationID: saved.request.OperationID, Patch: saved.result.Patch, JobID: current.JobID, Generation: current.Generation, Status: current.JobStatus, Revision: current.Revision}
	if err != nil {
		return receipt, err
	}
	saved.current = current
	if after.Approval != nil {
		outcome, err := r.applyUserConfirmationTx(ctx, tx, saved, *after.Approval)
		if err != nil {
			return receipt, err
		}
		receipt.JobID, receipt.Generation, receipt.Status, receipt.Revision = outcome.JobID, outcome.Generation, outcome.Status, outcome.Revision
		receipt.Summary, receipt.ExitClient = outcome.Summary, outcome.ExitClient
		receipt.MessageID, receipt.TurnOperationID, receipt.ControlOperationID = outcome.MessageID, outcome.TurnOperationID, outcome.ControlOperationID
		applyUserPresentation(after, &receipt)
		return receipt, nil
	}
	if current.PlanState == "proposed" && after.PlanState == "frozen" && len(current.Items) == 0 {
		// An empty proposal is a planning conversation, not executable work.
		// Keep its question visible, including for already persisted intents.
		receipt.Patch = nil
		receipt.Summary = "There are no tasks to start yet. Continue planning with what you want done."
		var response string
		err := tx.QueryRow(ctx, `SELECT payload_json->'notes'->>2 FROM evidence
		 WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'generation'=$2::bigint::text
		 AND payload_json->>'phase'='planning_draft' AND jsonb_array_length(payload_json->'notes')=3
		 ORDER BY id DESC LIMIT 1`, current.JobID, current.Generation).Scan(&response)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return receipt, err
		}
		if response != "" {
			receipt.Summary = "There are no tasks to start yet.\n\n" + response
		}
		applyUserPresentation(after, &receipt)
		return receipt, nil
	}
	if err := applyUserLedgerDeltaTx(ctx, tx, saved, after, &receipt); err != nil {
		return receipt, err
	}
	if after.JobStatus != current.JobStatus && after.JobStatus != receipt.Status {
		summary, err := r.applyUserJobStateTx(ctx, tx, saved, after.JobStatus, &receipt)
		if err != nil {
			return receipt, err
		}
		receipt.Summary = summary
	}
	text := ""
	if after.Message != nil {
		text = after.Message.Text
	}
	if after.NativeReply != nil {
		text = *after.NativeReply
	}
	if text != "" {
		summary, err := r.persistUserContentTx(ctx, tx, saved, text, &receipt)
		if err != nil {
			return receipt, err
		}
		receipt.Summary = summary
	}
	applyUserPresentation(after, &receipt)
	return receipt, nil
}

func applyUserPresentation(after sessionintent.Snapshot, receipt *sessionintent.Receipt) {
	if after.StatusVisible {
		receipt.Summary = fmt.Sprintf("Job %d: %s.", receipt.JobID, receipt.Status)
		if receipt.JobID == 0 {
			receipt.Summary = "No active job."
		}
	}
	if !after.Open {
		receipt.ExitClient = true
		receipt.Summary = "Leaving this client conversation."
	}
	if receipt.Summary == "" {
		receipt.Summary = "No state changes requested."
	}
}

func userDeltaOperation(id model.LifecycleOperationID, field string) model.LifecycleOperationID {
	digest := sha256.Sum256([]byte(string(id) + "\x00" + field))
	return model.LifecycleOperationID(fmt.Sprintf("lifecycle_operation_%x", digest))
}

func (r *Repository) applyUserJobStateTx(ctx context.Context, tx pgx.Tx, saved storedUserIntent, status string, receipt *sessionintent.Receipt) (string, error) {
	request, current := saved.request, saved.current
	request.OperationID = userDeltaOperation(request.OperationID, "job_status")
	switch status {
	case model.JobStatusRunning:
		job, err := lockedJobTx(ctx, tx, current.JobID)
		if err != nil {
			return "", err
		}
		authority, err := lockChannelTurnAuthorityTx(ctx, tx, current.State.ChannelID)
		if err != nil {
			return "", err
		}
		if err := validateChannelSessionJobAuthority(current.State.ChannelID, authority, current.State.WorkspaceIdentity, job); err != nil {
			return "", err
		}
		_, planning, err := jobRecoveryStepTx(ctx, tx, job)
		if err != nil {
			return "", err
		}
		var result LifecycleJobResult
		if planning {
			// The intent receipt owns replay. Reuse the existing planning
			// transition to retain the interrupted generation as history.
			command := ReplanJobCommand{OperationID: request.OperationID, JobID: job.ID, ObservedLedgerRevision: current.Revision, Feedback: request.Text, WorkspaceRoot: current.State.WorkspaceRoot, WorkspaceIdentity: current.State.WorkspaceIdentity}
			descriptor, describeErr := describeLifecycleOperation(command.OperationID, LifecycleReplanJob, command)
			if describeErr != nil {
				return "", describeErr
			}
			result, err = replanJobTx(ctx, tx, command, descriptor)
			if err == nil {
				receipt.ControlOperationID = command.OperationID
				// Replanning appends the owner message in assistant channels.
				// Bind that newly persisted row to the receipt for echo suppression.
				messageErr := tx.QueryRow(ctx, `SELECT id FROM ai_channel_messages
				 WHERE channel_id=$1 AND role='user' AND id>COALESCE($2::bigint,0)
				 ORDER BY id DESC LIMIT 1`, current.State.ChannelID, current.State.LatestMessageID).Scan(&receipt.MessageID)
				if messageErr != nil && !errors.Is(messageErr, pgx.ErrNoRows) {
					return "", messageErr
				}
				if messageErr == nil {
					if _, err := channelSessionUserMessageTx(ctx, tx, current.State.ChannelID, receipt.MessageID, request.Text); err != nil {
						return "", err
					}
				}
			}
		} else {
			result, err = resumeJobTx(ctx, tx, ResumeJobCommand{OperationID: request.OperationID, JobID: job.ID, ExpectedGeneration: current.Generation, ObservedLedgerRevision: current.Revision, Feedback: request.Text, WorkspaceRoot: current.State.WorkspaceRoot, WorkspaceIdentity: current.State.WorkspaceIdentity})
		}
		job = result.Job
		if err != nil {
			return "", err
		}
		receipt.Status, receipt.Generation = job.Status, job.CurrentGeneration
		return "Resumed the same job from retained progress.", nil

	case model.JobStatusCanceled:
		result, err := cancelJobTx(ctx, tx, CancelJobCommand{OperationID: request.OperationID, JobID: current.JobID, Reason: request.Text, WorkspaceRoot: current.State.WorkspaceRoot, WorkspaceIdentity: current.State.WorkspaceIdentity})
		if err != nil {
			return "", err
		}
		if result.Applied {
			if err := terminalizeScrumJobCardTx(ctx, tx, result.Job, model.JobStatusCanceled); err != nil {
				return "", err
			}
		}
		receipt.Status = result.Job.Status
		receipt.ControlOperationID = request.OperationID
		return "Job canceled.", nil
	case model.JobStatusWaiting:
		command, err := normalizeInterruptJobCommand(ReplanJobCommand{OperationID: request.OperationID, JobID: current.JobID, Feedback: request.Text, WorkspaceRoot: current.State.WorkspaceRoot, WorkspaceIdentity: current.State.WorkspaceIdentity})
		if err != nil {
			return "", err
		}
		descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleInterruptJob, command)
		if err != nil {
			return "", err
		}
		result, err := interruptJobTx(ctx, tx, command, descriptor)
		if err != nil {
			return "", err
		}
		receipt.Status, receipt.Generation = result.Job.Status, result.Job.CurrentGeneration
		receipt.ControlOperationID = request.OperationID
		return "Job interrupted; waiting for your direction.", nil
	default:
		return "", fmt.Errorf("unsupported job state")
	}
}

func (r *Repository) persistUserContentTx(ctx context.Context, tx pgx.Tx, saved storedUserIntent, text string, receipt *sessionintent.Receipt) (string, error) {
	request, current := saved.request, saved.current
	if current.JobStatus == model.JobStatusFailed && current.Recoverable {
		// A reply addressed to the retained workflow continues that workflow.
		// The same recovery transition retains its context and exact owner text.
		saved.request.Text = text
		return r.applyUserJobStateTx(ctx, tx, saved, model.JobStatusRunning, receipt)
	}
	request.OperationID = userDeltaOperation(request.OperationID, "message")
	if err := lockLifecycleOperationIdentityTx(ctx, tx, request.OperationID); err != nil {
		return "", err
	}
	command := ChannelSessionTurnCommand{OperationID: request.OperationID, ChannelID: current.State.ChannelID, WorkspaceRoot: current.State.WorkspaceRoot, WorkspaceIdentity: current.State.WorkspaceIdentity, Text: text, CodingMode: codingagent.Agents, ExpectedInputID: current.InputID}
	if command.Text == "" {
		return "", fmt.Errorf("forwarded user message is absent")
	}
	var result ChannelSessionTurnResult
	var err error
	if current.JobStatus == model.JobStatusPending || current.JobStatus == model.JobStatusRunning {
		command.ExpectedJobID, command.ExpectedGeneration = &current.JobID, &current.Generation
		result, err = submitChannelSessionMessageTx(ctx, tx, command)
	} else {
		authority, readErr := lockChannelTurnAuthorityTx(ctx, tx, command.ChannelID)
		if readErr != nil {
			return "", readErr
		}
		if err := validateAssistantSessionAuthority(command, authority); err != nil {
			return "", err
		}
		var job model.Job
		if current.JobID > 0 {
			job, err = lockedJobTx(ctx, tx, current.JobID)
			if err != nil {
				return "", err
			}
		}
		descriptor, describeErr := describeLifecycleOperation(command.OperationID, LifecycleChannelSession, command)
		if describeErr != nil {
			return "", describeErr
		}
		if _, err := reserveLifecycleOperationIdentityTx(ctx, tx, descriptor.ID, descriptor.Kind, descriptor.Payload); err != nil {
			return "", err
		}
		var stepID *int64
		result, stepID, err = r.applyChannelSessionTurnTx(ctx, tx, command, authority, job, current.JobID > 0 && !terminalJobStatus(job.Status))
		if err == nil {
			err = insertChannelSessionTurnOperationTx(ctx, tx, descriptor, result, stepID)
		}
	}
	if err != nil {
		return "", err
	}
	receipt.JobID, receipt.Generation, receipt.Status = result.Job.ID, result.Job.CurrentGeneration, result.Job.Status
	receipt.TurnOperationID = result.OperationID
	if result.UserMessage != nil {
		receipt.MessageID = result.UserMessage.ID
	}
	return "Message accepted by the current workflow.", nil

}

func (r *Repository) applyUserConfirmationTx(ctx context.Context, tx pgx.Tx, saved storedUserIntent, approved bool) (sessionintent.Receipt, error) {
	current := saved.current
	if current.Pending == nil {
		return sessionintent.Receipt{}, fmt.Errorf("no pending proposal")
	}
	pending, err := readUserIntentTx(ctx, tx, current.Pending.ID, current.State.ChannelID, current.State.WorkspaceIdentity)
	if err != nil {
		return sessionintent.Receipt{}, err
	}
	if pending.receipt != nil || !pending.result.RequiresApproval {
		return sessionintent.Receipt{}, fmt.Errorf("proposal is no longer awaiting approval")
	}
	outcome := sessionintent.Receipt{OperationID: pending.request.OperationID, Patch: pending.result.Patch, JobID: current.JobID, Generation: current.Generation, Status: current.JobStatus, Revision: current.Revision, Summary: "Proposed changes declined."}
	if approved {
		bound, err := lockUserIntentContextTx(ctx, tx, pending.current)
		if err != nil {
			return outcome, err
		}
		// The reply authorizes this exact stored delta once. It does not become a
		// second proposal, and may not confirm a confirmation.
		delta, err := sessionintent.Apply(userIntentDisplayedContext(bound, pending.request), pending.result.Patch)
		if err != nil {
			return outcome, err
		}
		if delta.Approval != nil {
			return outcome, fmt.Errorf("confirmation cannot recursively confirm")
		}
		pending.request.Text = saved.request.Text
		outcome, err = r.applyUserIntentTx(ctx, tx, pending, bound)
		if err != nil {
			return outcome, err
		}
	}
	if err := saveUserIntentReceiptTx(ctx, tx, pending.request.OperationID, outcome); err != nil {
		return outcome, err
	}
	return outcome, nil
}
