package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const ctrlCInterruptReason = "User interrupted the active objective from omni chat."

func parseChatCommand(line string) (name, text string, command bool) {
	if !strings.HasPrefix(line, "/") {
		return "", line, false
	}
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(line), "/"), " ", 2)
	name = strings.ToLower(strings.TrimSpace(parts[0]))
	if len(parts) == 2 {
		text = strings.TrimSpace(parts[1])
	}
	return name, text, true
}

func newOperationID() (queue.LifecycleOperationID, error) {
	return queue.NewRandomLifecycleOperationID()
}

func interruptedBoundary(steps []model.Step) bool {
	for _, step := range steps {
		if step.Status != model.StepStatusWaiting {
			continue
		}
		return step.Action == "objective_resolve" || step.Action == "v3_coding"
	}
	return false
}

func (session *chatSession) control(action, text string) error {
	if session.active == nil {
		return fmt.Errorf("/%s requires an active job", action)
	}
	jobID := session.active.Job.ID
	operationID, err := newOperationID()
	if err != nil {
		return err
	}
	var receipt model.Job
	switch action {
	case "interrupt":
		if text == "" {
			text = ctrlCInterruptReason
		}
		receipt, err = session.client.Interrupt(session.ctx, jobID, operationID, text)
	case "redirect", "replan":
		if text == "" {
			return fmt.Errorf("/%s requires exact redirection text", action)
		}
		receipt, err = session.client.Replan(session.ctx, jobID, operationID, text)
	case "feedback":
		if text == "" {
			return fmt.Errorf("/feedback requires exact feedback text")
		}
		receipt, err = session.client.SubmitFeedback(session.ctx, jobID, operationID, text)
	case "cancel":
		if text == "" {
			return fmt.Errorf("/cancel requires a reason")
		}
		receipt, err = session.client.Cancel(session.ctx, jobID, operationID, text)
	default:
		return fmt.Errorf("unsupported active-job control %q", action)
	}
	if err != nil {
		return err
	}
	if receipt.ID != jobID {
		return fmt.Errorf("/%s changed job identity from %d to %d", action, jobID, receipt.ID)
	}
	_, err = session.refreshJob(true)
	return err
}

func printChatHelp(renderer chatRenderer) {
	fmt.Fprintln(renderer.out, "commands:")
	fmt.Fprintln(renderer.out, "  /status                 reload authoritative job state")
	fmt.Fprintln(renderer.out, "  /interrupt [reason]     pause the active job on the same job identity")
	fmt.Fprintln(renderer.out, "  /redirect <text>        replace active work on the same job with this direction")
	fmt.Fprintln(renderer.out, "  /cancel <reason>        terminally cancel the active job")
	fmt.Fprintln(renderer.out, "  /exit                   close this client; server work continues")
	fmt.Fprintln(renderer.out, "While work is active, ordinary text is an exact same-job redirection.")
}
