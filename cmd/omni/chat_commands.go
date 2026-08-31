package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const ctrlCInterruptReason = "User interrupted the active objective from omni chat."

func parseChatCommand(line string) (name, text string, command bool) {
	if !strings.HasPrefix(line, "/") {
		return "", line, false
	}
	body := strings.TrimPrefix(line, "/")
	delimiter := strings.IndexByte(body, ' ')
	if delimiter < 0 {
		return body, "", true
	}
	return body[:delimiter], body[delimiter+1:], true
}

func newOperationID() (queue.LifecycleOperationID, error) {
	return queue.NewRandomLifecycleOperationID()
}

type pendingControl struct {
	jobID         int64
	action        string
	exactText     string
	locallyEchoed bool
	operationID   queue.LifecycleOperationID
}

func (session *chatSession) controlOperationID(
	jobID int64,
	action string,
	exactText string,
	locallyEchoed bool,
) (queue.LifecycleOperationID, error) {
	if session.pendingTurn != nil {
		return "", fmt.Errorf("session turn %q remains unresolved", session.pendingTurn.operationID)
	}
	if session.pendingControl != nil && session.pendingControl.jobID == jobID &&
		session.pendingControl.action == action && session.pendingControl.exactText == exactText {
		return session.pendingControl.operationID, nil
	}
	if session.pendingControl != nil {
		return "", fmt.Errorf(
			"/%s operation %q remains unresolved",
			session.pendingControl.action,
			session.pendingControl.operationID,
		)
	}
	operationID, err := newOperationID()
	if err != nil {
		return "", err
	}
	session.pendingControl = &pendingControl{
		jobID: jobID, action: action, exactText: exactText,
		locallyEchoed: locallyEchoed, operationID: operationID,
	}
	return operationID, nil
}

func (session *chatSession) control(
	action,
	text string,
	locallyEchoed bool,
	expectedJobID *int64,
) (int64, error) {
	validatedText, err := validatedChatControlText(action, text)
	if err != nil {
		return 0, err
	}
	text = validatedText
	unresolvedControl := session.pendingControl
	if err := session.reloadSnapshot(); err != nil {
		return 0, err
	}
	if pending := unresolvedControl; pending != nil &&
		pending.action == action && pending.exactText == text {
		if persisted, exists := session.controls[pending.operationID]; exists {
			if persisted.JobID != pending.jobID || persisted.Kind != sessionControlKind(pending.action) ||
				persisted.Text != pending.exactText {
				return 0, fmt.Errorf("persisted /%s operation differs from pending authority", action)
			}
			session.pendingControl = nil
			return pending.jobID, session.renderer.system("job %d · %s accepted", pending.jobID, action)
		}
		job, err := session.replayPendingControl(pending)
		if err != nil {
			return 0, err
		}
		if err := session.reloadSnapshotAuthoritatively(); err != nil {
			return 0, err
		}
		if _, exists := session.controls[pending.operationID]; !exists {
			return 0, fmt.Errorf("resolved /%s operation is absent from persisted session authority", action)
		}
		session.pendingControl = nil
		return job.ID, session.renderer.system("job %d · %s accepted", job.ID, action)
	}
	if session.active == nil {
		if expectedJobID != nil && *expectedJobID > 0 {
			return 0, fmt.Errorf(
				"/%s target job %d is no longer active",
				action,
				*expectedJobID,
			)
		}
		return 0, fmt.Errorf("/%s requires an active server job", action)
	}
	jobID := session.active.Job.ID
	if expectedJobID != nil && jobID != *expectedJobID {
		return 0, fmt.Errorf(
			"/%s target job changed from %d to %d before submission",
			action,
			*expectedJobID,
			jobID,
		)
	}
	operationID, err := session.controlOperationID(jobID, action, text, locallyEchoed)
	if err != nil {
		return 0, err
	}
	var receipt model.Job
	switch action {
	case "interrupt":
		receipt, err = awaitChatRequest(
			session.ctx,
			session.signals,
			func(requestContext context.Context) (model.Job, error) {
				return session.client.Interrupt(
					requestContext,
					session.channel,
					session.workspaceIdentity,
					jobID,
					operationID,
					text,
				)
			},
		)
	case "redirect":
		receipt, err = awaitChatRequest(
			session.ctx,
			session.signals,
			func(requestContext context.Context) (model.Job, error) {
				return session.client.Replan(
					requestContext,
					session.channel,
					session.workspaceIdentity,
					jobID,
					operationID,
					text,
				)
			},
		)
	case "cancel":
		receipt, err = awaitChatRequest(
			session.ctx,
			session.signals,
			func(requestContext context.Context) (model.Job, error) {
				return session.client.Cancel(
					requestContext,
					session.channel,
					session.workspaceIdentity,
					jobID,
					operationID,
					text,
				)
			},
		)
	}
	if err != nil {
		if definitiveChatRequestFailure(err) {
			session.pendingControl = nil
		}
		return 0, err
	}
	if receipt.ID != jobID {
		return 0, fmt.Errorf("/%s changed job identity from %d to %d", action, jobID, receipt.ID)
	}
	if err := session.reloadSnapshot(); err != nil {
		return 0, err
	}
	if _, persisted := session.controls[operationID]; !persisted {
		return 0, fmt.Errorf(
			"/%s operation %q was accepted but is absent from persisted session state",
			action,
			operationID,
		)
	}
	session.pendingControl = nil
	return jobID, session.renderer.system("job %d · %s accepted", jobID, action)
}

func validatedChatControlText(action, exactText string) (string, error) {
	switch action {
	case "interrupt":
		if strings.TrimSpace(exactText) == "" {
			exactText = ctrlCInterruptReason
		}
		if err := client.ValidateInterruptFeedback(exactText); err != nil {
			return "", err
		}
	case "redirect":
		if strings.TrimSpace(exactText) == "" {
			return "", fmt.Errorf("/redirect requires exact redirection text")
		}
		if err := client.ValidateReplanFeedback(exactText); err != nil {
			return "", err
		}
	case "cancel":
		if strings.TrimSpace(exactText) == "" {
			return "", fmt.Errorf("/cancel requires a reason")
		}
		if err := client.ValidateCancelReason(exactText); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported active-job control %q", action)
	}
	return exactText, nil
}

func printChatHelp(renderer chatRenderer) error {
	return renderer.console.WriteOutput(
		"commands:\n" +
			"  /status                 reload authoritative job state\n" +
			"  /interrupt [reason]     pause the active job on the same job identity\n" +
			"  /redirect <text>        explicitly redirect the active job\n" +
			"  /cancel <reason>        terminally cancel the active job\n" +
			"  /exit                   close this client; server work continues\n" +
			"Ordinary text is submitted to the server-owned session turn boundary.\n",
	)
}
