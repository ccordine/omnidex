package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/client"
)

func runFeedback(c *client.Client, args []string) {
	operationID, args, err := parseLifecycleOperationArgs(args)
	if err != nil {
		die(err.Error())
	}
	if len(args) < 2 {
		die("feedback requires [--operation-id id] <job-id> and feedback text")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	feedback := strings.TrimSpace(strings.Join(args[1:], " "))
	if feedback == "" {
		die("feedback text is required")
	}

	announceLifecycleOperationID(operationID)
	job, err := c.SubmitFeedback(context.Background(), id, operationID, feedback)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("submitted feedback for job %d, new status=%s\n", job.ID, job.Status)
}
