package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func parseQueuedTurnInput(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	trimmedTabs := strings.TrimLeft(raw, "\t")
	if len(trimmedTabs) == len(raw) {
		return "", false
	}
	message := strings.TrimSpace(trimmedTabs)
	if message == "" {
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
	if input == nil {
		return false, nil
	}
	if pendingInputs == nil {
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

		if queuedMessage, queueOK := parseQueuedTurnInput(event.line); queueOK {
			*pendingInputs = append(*pendingInputs, queuedMessage)
			emitSystem(ui, fmt.Sprintf("TAB queue: queued next turn (#%d): %s", len(*pendingInputs), truncateForWatch(queuedMessage, 140)))
			continue
		}

		command, _ := parseSlashCommand(strings.TrimSpace(event.line))
		if command == "exit" || command == "quit" {
			return true, nil
		}
		if strings.HasPrefix(strings.TrimSpace(event.line), "/") {
			if handled, quit := handleActiveTurnSlashCommand(c, jobID, strings.TrimSpace(event.line), ui); handled {
				return quit, nil
			}
		}

		if strings.TrimSpace(event.line) != "" {
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
		job, err := c.Cancel(context.Background(), *jobID, body)
		if err != nil {
			emitAssistantError(ui, "error canceling job: "+err.Error())
			return true, false
		}
		emitSystem(ui, fmt.Sprintf("canceled job %d status=%s", job.ID, job.Status))
		return true, false
	case "interrupt":
		if strings.TrimSpace(body) == "" {
			emitSystem(ui, "usage: /interrupt <context>")
			return true, false
		}
		previousID := *jobID
		job, err := c.Interrupt(context.Background(), previousID, body)
		if err != nil {
			emitAssistantError(ui, "error interrupting job: "+err.Error())
			return true, false
		}
		*jobID = job.ID
		emitAuthorityControlResult(ui, "interrupt", previousID, job)
		return true, false
	case "replan":
		if strings.TrimSpace(body) == "" {
			emitSystem(ui, "usage: /replan <context>")
			return true, false
		}
		previousID := *jobID
		job, err := c.Replan(context.Background(), previousID, body)
		if err != nil {
			emitAssistantError(ui, "error replanning job: "+err.Error())
			return true, false
		}
		*jobID = job.ID
		emitAuthorityControlResult(ui, "replan", previousID, job)
		return true, false
	default:
		return false, false
	}
}

func emitAuthorityControlResult(ui *chatUI, action string, previousID int64, job model.Job) {
	if job.ID != previousID {
		emitSystem(ui, fmt.Sprintf("%s revised authority: job %d replaced job %d and restarted intent validation", action, job.ID, previousID))
		return
	}
	emitSystem(ui, fmt.Sprintf("%s submitted for job %d status=%s", action, job.ID, job.Status))
}

func awaitInteractiveTurn(
	c *client.Client,
	input *chatInputReader,
	jobID int64,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	pendingInputs *[]string,
	ui *chatUI,
) (model.JobDetails, bool, error) {
	lastStatus := ""
	lastStepStatus := map[int64]string{}
	lastStepDetails := map[int64]string{}
	lastExternalOutputOffsets := map[int64]int{}
	seenContextIDs := map[int64]struct{}{}
	observedJobID := int64(0)

	for {
		if observedJobID != jobID {
			lastStatus = ""
			lastStepStatus = map[int64]string{}
			lastStepDetails = map[int64]string{}
			lastExternalOutputOffsets = map[int64]int{}
			seenContextIDs = map[int64]struct{}{}
			observedJobID = jobID
		}
		details, err := c.Show(context.Background(), jobID)
		if err != nil {
			return model.JobDetails{}, false, err
		}

		status := details.Job.Status
		if status != lastStatus {
			emitSystem(ui, fmt.Sprintf("job %d status=%s", jobID, status))
			lastStatus = status
		}

		printed := false
		if progress || verbose {
			printed = printStepStatusUpdatesWithUI(details.Steps, lastStepStatus, ui) || printed
		}
		if progress && !verbose {
			printed = printExternalAgentStreamUpdatesWithUI(details.Steps, lastExternalOutputOffsets, ui, maxChars) || printed
		}
		if verbose {
			printed = printStepDetailUpdates(details.Steps, lastStepDetails, maxChars) || printed
		}
		if progress || verbose {
			printed = printContextUpdatesWithUI(details.Contexts, seenContextIDs, progress, verbose, maxChars, ui) || printed
		}
		if printed {
			emitRule(ui)
		}

		if status != model.JobStatusWaiting {
			quit, err := captureQueuedTurnInput(c, &jobID, input, pendingInputs, ui)
			if err != nil {
				return model.JobDetails{}, false, err
			}
			if quit {
				return details, true, nil
			}
		}

		if status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusCanceled {
			return details, false, nil
		}

		if status == model.JobStatusWaiting {
			question := latestContextValue(details.Contexts, "input_question")
			if strings.TrimSpace(question) != "" {
				emitNeedsInput(ui, question)
			} else {
				emitNeedsInput(ui, "assistant needs input to continue")
			}
			emitSystem(ui, "reply normally to submit feedback, or use /interrupt, /replan, /cancel, /exit")

			for {
				fmt.Print(feedbackPrompt(ui))
				rawInput, eof, err := input.readBlocking()
				if err != nil {
					return model.JobDetails{}, false, err
				}
				if eof {
					return details, true, nil
				}

				feedbackInput := strings.TrimSpace(rawInput)
				if feedbackInput == "" {
					continue
				}

				if strings.HasPrefix(feedbackInput, "/") {
					command, body := parseSlashCommand(feedbackInput)
					switch command {
					case "exit", "quit":
						return details, true, nil
					case "help":
						printInteractiveInputHelp()
						continue
					case "cancel":
						job, err := c.Cancel(context.Background(), jobID, body)
						if err != nil {
							fmt.Fprintf(os.Stderr, "error canceling job: %v\n", err)
							continue
						}
						emitSystem(ui, fmt.Sprintf("canceled job %d status=%s", job.ID, job.Status))
					case "interrupt":
						if strings.TrimSpace(body) == "" {
							emitSystem(ui, "usage: /interrupt <context>")
							continue
						}
						previousID := jobID
						job, err := c.Interrupt(context.Background(), previousID, body)
						if err != nil {
							fmt.Fprintf(os.Stderr, "error interrupting job: %v\n", err)
							continue
						}
						jobID = job.ID
						emitAuthorityControlResult(ui, "interrupt", previousID, job)
					case "replan":
						if strings.TrimSpace(body) == "" {
							emitSystem(ui, "usage: /replan <context>")
							continue
						}
						previousID := jobID
						job, err := c.Replan(context.Background(), previousID, body)
						if err != nil {
							fmt.Fprintf(os.Stderr, "error replanning job: %v\n", err)
							continue
						}
						jobID = job.ID
						emitAuthorityControlResult(ui, "replan", previousID, job)
					default:
						emitSystem(ui, "unknown command in feedback mode. use /help")
						continue
					}
					break
				}

				previousID := jobID
				job, err := c.SubmitFeedback(context.Background(), previousID, feedbackInput)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error submitting feedback: %v\n", err)
					continue
				}
				jobID = job.ID
				emitAuthorityControlResult(ui, "feedback", previousID, job)
				break
			}
		}

		time.Sleep(interval)
	}
}
