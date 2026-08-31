package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/queue"
)

func runInterrupt(c *client.Client, args []string) {
	operationID, args, err := parseLifecycleOperationArgs(args)
	if err != nil {
		die(err.Error())
	}
	if len(args) < 2 {
		die("interrupt requires [--operation-id id] <job-id> and context text")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	feedback := strings.TrimSpace(strings.Join(args[1:], " "))
	if feedback == "" {
		die("context text is required")
	}

	announceLifecycleOperationID(operationID)
	job, err := c.Interrupt(context.Background(), id, operationID, feedback)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("submitted interrupt for job %d, status=%s\n", job.ID, job.Status)
}

func runCancel(c *client.Client, args []string) {
	operationID, args, err := parseLifecycleOperationArgs(args)
	if err != nil {
		die(err.Error())
	}
	if len(args) < 2 {
		die("cancel requires [--operation-id id] <job-id> <reason>")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	reason := strings.TrimSpace(strings.Join(args[1:], " "))
	if reason == "" {
		die("cancel reason is required")
	}

	announceLifecycleOperationID(operationID)
	job, err := c.Cancel(context.Background(), queue.CancelJobCommand{
		OperationID: operationID, JobID: id, Reason: reason,
	})
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("canceled job %d, status=%s\n", job.ID, job.Status)
}

func runReplan(c *client.Client, args []string) {
	operationID, args, err := parseLifecycleOperationArgs(args)
	if err != nil {
		die(err.Error())
	}
	if len(args) < 2 {
		die("replan requires [--operation-id id] <job-id> and replanning context text")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	feedback := strings.TrimSpace(strings.Join(args[1:], " "))
	if feedback == "" {
		die("replanning context text is required")
	}

	announceLifecycleOperationID(operationID)
	job, err := c.Replan(context.Background(), id, operationID, feedback)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("replanned job %d, status=%s\n", job.ID, job.Status)
}
