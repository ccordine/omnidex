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

func runEnqueue(c *client.Client, args []string) {
	fs := flag.NewFlagSet("enqueue", flag.ExitOnError)
	pipeline := fs.String("pipeline", model.PipelineAssistant, "pipeline type: assistant|chat|story")
	searchQuery := fs.String("search-query", "", "override web search query for this job")
	sessionID := fs.String("session", "", "optional session/thread identifier for continuity")
	modelAnalyze := fs.String("model-analyze", "", "override analyze model for this job")
	modelResponse := fs.String("model-response", "", "override response model for this job")
	modelSearch := fs.String("model-search", "", "override search-query model for this job")
	modelTagger := fs.String("model-tagger", "", "override tagging model for this job")
	modelPlan := fs.String("model-plan", "", "override planner model for this job")
	modelVerify := fs.String("model-verify", "", "override verification evaluator model for this job")
	modelMemory := fs.String("model-memory", "", "override memory-inference model for this job")
	agentFlagPointers := registerCLIAgentRuntimeFlags(fs)
	_ = fs.Parse(args)
	agentOverrides, err := cliAgentRuntimeConfigFromFlags(agentFlagPointers.Values())
	if err != nil {
		die(err.Error())
	}

	instruction := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if instruction == "" {
		die("instruction is required")
	}

	metadata := map[string]any{}
	cwd := ""
	if dir, err := os.Getwd(); err == nil && strings.TrimSpace(dir) != "" {
		cwd = strings.TrimSpace(dir)
	}
	if cwd != "" {
		metadata["client_cwd"] = cwd
	}
	hostSnapshot := discoverHostEnvironmentSnapshot(cwd)
	applyHostEnvironmentMetadata(metadata, hostSnapshot)
	applyHostTemporalMetadata(metadata, time.Now())
	if cwd != "" {
		metadata["client_cwd"] = cwd
	}
	if strings.TrimSpace(*searchQuery) != "" {
		metadata["search_query"] = strings.TrimSpace(*searchQuery)
	}
	if strings.TrimSpace(*sessionID) != "" {
		metadata["session_id"] = strings.TrimSpace(*sessionID)
	}
	if strings.TrimSpace(*modelAnalyze) != "" {
		metadata["model_analyze"] = strings.TrimSpace(*modelAnalyze)
	}
	if strings.TrimSpace(*modelResponse) != "" {
		metadata["model_response"] = strings.TrimSpace(*modelResponse)
	}
	if strings.TrimSpace(*modelSearch) != "" {
		metadata["model_search"] = strings.TrimSpace(*modelSearch)
	}
	if strings.TrimSpace(*modelTagger) != "" {
		metadata["model_tagger"] = strings.TrimSpace(*modelTagger)
	}
	if strings.TrimSpace(*modelPlan) != "" {
		metadata["model_plan"] = strings.TrimSpace(*modelPlan)
	}
	if strings.TrimSpace(*modelVerify) != "" {
		metadata["model_verify"] = strings.TrimSpace(*modelVerify)
	}
	if strings.TrimSpace(*modelMemory) != "" {
		metadata["model_memory"] = strings.TrimSpace(*modelMemory)
	}
	agentOverrides.ApplyToMetadata(metadata)
	if err := persistHostCapabilityMemory(c, hostSnapshot); err != nil {
		fmt.Fprintf(os.Stderr, "warn: capability memory sync failed: %v\n", err)
	}

	job, err := c.Enqueue(context.Background(), instruction, *pipeline, metadata)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("enqueued job %d (%s) status=%s\n", job.ID, job.Pipeline, job.Status)
}
