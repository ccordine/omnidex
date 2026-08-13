package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type codingRunRequest struct {
	Instruction string
	Pipeline    string
	Metadata    map[string]any
}

func buildCodingRunRequest(instruction, cwd, session string, agent *cliAgentRuntimeConfig) (codingRunRequest, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return codingRunRequest{}, fmt.Errorf("coding instruction is required")
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return codingRunRequest{}, fmt.Errorf("coding workspace is required")
	}
	metadata := map[string]any{
		"client_cwd":   cwd,
		"host_env_cwd": cwd,
	}
	if session = strings.TrimSpace(session); session != "" {
		metadata["session_id"] = session
	}
	agentValues := map[string]string{}
	if agent != nil {
		agentValues = agent.ToMap()
	}
	if strings.TrimSpace(agentValues["agent_system"]) == "" {
		agentValues["agent_system"] = agentconfig.SystemOmnidex
	}
	metadata["instance_agent_config"] = agentValues
	return codingRunRequest{
		Instruction: instruction,
		Pipeline:    model.PipelineCoding,
		Metadata:    metadata,
	}, nil
}

func runCoding(c *client.Client, args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	session := fs.String("session", "", "optional session identifier for follow-up continuity")
	interval := fs.Duration("interval", 2*time.Second, "poll interval while the coding job runs")
	progress := fs.Bool("progress", true, "print concise live coding events")
	verbose := fs.Bool("verbose", false, "print full prompts and context diagnostics")
	maxChars := fs.Int("max-chars", 1200, "max characters shown per event (0 disables truncation)")
	agentFlags := registerCLIAgentRuntimeFlags(fs)
	_ = fs.Parse(args)

	agent, err := cliAgentRuntimeConfigFromFlags(agentFlags.Values())
	if err != nil {
		die(err.Error())
	}
	cwd, err := os.Getwd()
	if err != nil {
		die("resolve coding workspace: " + err.Error())
	}
	request, err := buildCodingRunRequest(strings.Join(fs.Args(), " "), cwd, *session, agent)
	if err != nil {
		die(err.Error())
	}
	job, err := c.EnqueueCoding(context.Background(), request.Instruction, request.Metadata)
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
