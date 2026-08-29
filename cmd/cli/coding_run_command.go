package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type codingRunRequest struct {
	Instruction string
	Pipeline    string
	Workspace   string
	SessionID   string
}

func buildCodingRunRequest(instruction, cwd, session string) (codingRunRequest, error) {
	if strings.TrimSpace(instruction) == "" {
		return codingRunRequest{}, fmt.Errorf("coding instruction is required")
	}
	if cwd == "" || cwd != strings.TrimSpace(cwd) {
		return codingRunRequest{}, fmt.Errorf("coding workspace is required")
	}
	if session != strings.TrimSpace(session) {
		return codingRunRequest{}, fmt.Errorf("coding session identifier must be canonical")
	}
	return codingRunRequest{
		Instruction: instruction,
		Pipeline:    model.PipelineCoding,
		Workspace:   cwd,
		SessionID:   session,
	}, nil
}

func runCoding(c *client.Client, args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	session := fs.String("session", "", "optional session identifier for follow-up continuity")
	interval := fs.Duration("interval", 2*time.Second, "poll interval while the coding job runs")
	progress := fs.Bool("progress", true, "print concise live coding events")
	verbose := fs.Bool("verbose", false, "print full prompts and context diagnostics")
	maxChars := fs.Int("max-chars", 1200, "max characters shown per event (0 disables truncation)")
	_ = fs.Parse(args)

	cwd, err := os.Getwd()
	if err != nil {
		die("resolve coding workspace: " + err.Error())
	}
	request, err := buildCodingRunRequest(strings.Join(fs.Args(), " "), cwd, *session)
	if err != nil {
		die(err.Error())
	}
	job, err := c.EnqueueCoding(context.Background(), request.Instruction, request.Workspace, request.SessionID)
	if err != nil {
		die(err.Error())
	}
	fmt.Printf("coding job %d queued workspace=%s\n", job.ID, cwd)
	details, err := awaitCodingRun(c, job.ID, *interval, *progress, *verbose, *maxChars)
	if err != nil {
		die(err.Error())
	}
	if details.Job.Status != model.JobStatusCompleted {
		detail := strings.TrimSpace(details.Job.Error)
		if detail == "" {
			detail = "job ended with status " + details.Job.Status
		}
		die(detail)
	}
}
