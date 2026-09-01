package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func parseQueuedTurnInput(raw string) (string, bool) {
	if raw == "" || raw[0] != '\t' {
		return "", false
	}
	message := raw[1:]
	if strings.TrimSpace(message) == "" {
		return "", false
	}
	return message, true
}

func captureQueuedTurnInput(
	c *client.Client,
	jobID *int64,
	input *chatInputReader,
	pendingInputs *[]string,
	ui *chatUI,
) (bool, error) {
	if input == nil || pendingInputs == nil {
		return false, nil
	}
	if jobID == nil || *jobID <= 0 {
		return false, fmt.Errorf("active job id is required")
	}
	for {
		event, ok := input.readNonBlocking()
		if !ok {
			return false, nil
		}
		if event.err != nil {
			return false, event.err
		}
		if event.eof {
			return true, nil
		}
		if queuedMessage, ok := parseQueuedTurnInput(event.line); ok {
			*pendingInputs = append(*pendingInputs, queuedMessage)
			emitSystem(ui, fmt.Sprintf(
				"TAB queue: queued next turn (#%d): %s",
				len(*pendingInputs), truncateForWatch(queuedMessage, 140),
			))
			continue
		}
		line := strings.TrimSpace(event.line)
		command, _ := parseSlashCommand(line)
		if command == "exit" || command == "quit" {
			return true, nil
		}
		if strings.HasPrefix(line, "/") {
			if handled, quit := handleActiveTurnSlashCommand(c, jobID, line, ui); handled {
				return quit, nil
			}
		}
		if line != "" {
			emitSystem(ui, "turn in progress: TAB + message queues a follow-up; /interrupt, /replan, and /cancel steer the active job")
		}
	}
}

func handleActiveTurnSlashCommand(c *client.Client, jobID *int64, line string, ui *chatUI) (bool, bool) {
	if jobID == nil || *jobID <= 0 {
		emitAssistantError(ui, "active job id is unavailable")
		return true, false
	}
	command, body := parseSlashCommand(line)
	switch command {
	case "help":
		printInteractiveInputHelp()
		return true, false
	case "cancel":
		if strings.TrimSpace(body) == "" {
			emitSystem(ui, "usage: /cancel <reason>")
			return true, false
		}
		job, err := c.Cancel(context.Background(), queue.CancelJobCommand{
			OperationID: newLifecycleOperationID(), JobID: *jobID, Reason: body,
		})
		if err != nil {
			emitAssistantError(ui, "error canceling job: "+err.Error())
			return true, false
		}
		emitSystem(ui, fmt.Sprintf("canceled job %d status=%s", job.ID, job.Status))
		return true, false
	case "interrupt", "replan":
		return handleSameJobSteering(c, jobID, command, body, ui)
	default:
		return false, false
	}
}

func handleSameJobSteering(
	c *client.Client,
	jobID *int64,
	action, body string,
	ui *chatUI,
) (bool, bool) {
	if strings.TrimSpace(body) == "" {
		emitSystem(ui, fmt.Sprintf("usage: /%s <context>", action))
		return true, false
	}
	previousID := *jobID
	var job model.Job
	var err error
	if action == "interrupt" {
		job, err = c.Interrupt(context.Background(), previousID, newLifecycleOperationID(), body)
	} else {
		job, err = c.Replan(context.Background(), previousID, newLifecycleOperationID(), body)
	}
	if err != nil {
		verb := "interrupting"
		if action == "replan" {
			verb = "replanning"
		}
		emitAssistantError(ui, fmt.Sprintf("error %s job: %v", verb, err))
		return true, false
	}
	if err := validateSameJobControl(action, previousID, job); err != nil {
		emitAssistantError(ui, err.Error())
		return true, false
	}
	*jobID = job.ID
	emitAuthorityControlResult(ui, action, previousID, job)
	return true, false
}

func emitAuthorityControlResult(ui *chatUI, action string, previousID int64, job model.Job) {
	emitSystem(ui, fmt.Sprintf("%s submitted for job %d status=%s", action, job.ID, job.Status))
}

func validateSameJobControl(action string, previousID int64, job model.Job) error {
	if previousID < 1 || job.ID != previousID {
		return fmt.Errorf(
			"%s violated same-job authority: received job %d for active job %d",
			action, job.ID, previousID,
		)
	}
	return nil
}
