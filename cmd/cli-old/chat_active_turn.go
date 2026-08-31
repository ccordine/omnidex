package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

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
	seenContextIDs := map[int64]struct{}{}
	observedJobID := int64(0)

	for {
		if observedJobID != jobID {
			lastStatus = ""
			lastStepStatus = map[int64]string{}
			lastStepDetails = map[int64]string{}
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
						if strings.TrimSpace(body) == "" {
							emitSystem(ui, "usage: /cancel <reason>")
							continue
						}
						job, err := c.Cancel(context.Background(), queue.CancelJobCommand{
							OperationID: newLifecycleOperationID(), JobID: jobID, Reason: body,
						})
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
						job, err := c.Interrupt(context.Background(), previousID, newLifecycleOperationID(), body)
						if err != nil {
							fmt.Fprintf(os.Stderr, "error interrupting job: %v\n", err)
							continue
						}
						if err := validateSameJobControl("interrupt", previousID, job); err != nil {
							fmt.Fprintln(os.Stderr, err)
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
						job, err := c.Replan(context.Background(), previousID, newLifecycleOperationID(), body)
						if err != nil {
							fmt.Fprintf(os.Stderr, "error replanning job: %v\n", err)
							continue
						}
						if err := validateSameJobControl("replan", previousID, job); err != nil {
							fmt.Fprintln(os.Stderr, err)
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
				job, err := c.SubmitFeedback(context.Background(), previousID, newLifecycleOperationID(), feedbackInput)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error submitting feedback: %v\n", err)
					continue
				}
				if err := validateSameJobControl("feedback", previousID, job); err != nil {
					fmt.Fprintln(os.Stderr, err)
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
