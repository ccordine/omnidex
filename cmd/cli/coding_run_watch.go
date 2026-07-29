package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func awaitCodingRun(c *client.Client, jobID int64, interval time.Duration, progress, verbose bool, maxChars int) (model.JobDetails, error) {
	if c == nil {
		return model.JobDetails{}, fmt.Errorf("core client is required")
	}
	if jobID <= 0 {
		return model.JobDetails{}, fmt.Errorf("coding job id is required")
	}
	if interval <= 0 {
		return model.JobDetails{}, fmt.Errorf("poll interval must be positive")
	}
	lastStatus := ""
	lastStepStatus := map[int64]string{}
	lastStepDetails := map[int64]string{}
	lastExternalOutputOffsets := map[int64]int{}
	seenContextIDs := map[int64]struct{}{}
	for {
		details, err := c.Show(context.Background(), jobID)
		if err != nil {
			return model.JobDetails{}, fmt.Errorf("read coding job %d: %w", jobID, err)
		}
		if details.Job.Status != lastStatus {
			fmt.Printf("job %d status=%s\n", jobID, details.Job.Status)
			lastStatus = details.Job.Status
		}
		printed := false
		if progress || verbose {
			printed = printStepStatusUpdates(details.Steps, lastStepStatus) || printed
		}
		if progress && !verbose {
			printed = printExternalAgentStreamUpdatesWithUI(details.Steps, lastExternalOutputOffsets, nil, maxChars) || printed
		}
		if verbose {
			printed = printStepDetailUpdates(details.Steps, lastStepDetails, maxChars) || printed
		}
		if progress || verbose {
			printed = printContextUpdates(details.Contexts, seenContextIDs, progress, verbose, maxChars) || printed
		}
		if printed {
			fmt.Println("---")
		}
		switch details.Job.Status {
		case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
			if result := strings.TrimSpace(details.Job.Result); result != "" {
				fmt.Printf("result:\n%s\n", result)
			}
			return details, nil
		case model.JobStatusWaiting:
			question := strings.TrimSpace(latestContextValue(details.Contexts, "input_question"))
			if question == "" {
				question = "coding job requires direct feedback"
			}
			return details, fmt.Errorf("%s; continue with `omni feedback %d \"...\"`", question, jobID)
		}
		time.Sleep(interval)
	}
}
