package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/client"
)

func runInterrupt(c *client.Client, args []string) {
	if len(args) < 2 {
		die("interrupt requires <job-id> and context text")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	feedback := strings.TrimSpace(strings.Join(args[1:], " "))
	if feedback == "" {
		die("context text is required")
	}

	job, err := c.Interrupt(context.Background(), id, feedback)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("submitted interrupt for job %d, status=%s\n", job.ID, job.Status)
}

func runCancel(c *client.Client, args []string) {
	if len(args) < 1 {
		die("cancel requires <job-id> [reason]")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	reason := ""
	if len(args) > 1 {
		reason = strings.TrimSpace(strings.Join(args[1:], " "))
	}

	job, err := c.Cancel(context.Background(), id, reason)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("canceled job %d, status=%s\n", job.ID, job.Status)
}

func runReplan(c *client.Client, args []string) {
	if len(args) < 2 {
		die("replan requires <job-id> and replanning context text")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	feedback := strings.TrimSpace(strings.Join(args[1:], " "))
	if feedback == "" {
		die("replanning context text is required")
	}

	job, err := c.Replan(context.Background(), id, feedback)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("replanned job %d, status=%s\n", job.ID, job.Status)
}

func runContinueJob(c *client.Client, args []string) {
	if len(args) < 2 {
		die("continue requires <job-id> and follow-up instruction text")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	instruction := strings.TrimSpace(strings.Join(args[1:], " "))
	if instruction == "" {
		die("follow-up instruction text is required")
	}

	details, err := c.Show(context.Background(), id)
	if err != nil {
		die(err.Error())
	}

	metadata := map[string]any{}
	if len(details.Job.Metadata) > 0 {
		_ = json.Unmarshal(details.Job.Metadata, &metadata)
	}
	sessionID := strings.TrimSpace(fmt.Sprintf("%v", metadata["session_id"]))
	if sessionID == "" || sessionID == "<nil>" {
		sessionID = fmt.Sprintf("job-%d", details.Job.ID)
	}
	metadata["session_id"] = sessionID
	metadata["parent_job_id"] = details.Job.ID
	delete(metadata, "replan_feedback")

	job, err := c.Enqueue(context.Background(), instruction, details.Job.Pipeline, metadata)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("continued job %d -> new job %d (%s) status=%s session=%s\n", details.Job.ID, job.ID, job.Pipeline, job.Status, sessionID)
}
