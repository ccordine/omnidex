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
	profile := fs.String("profile", "default", "execution profile: default|architect")
	pipeline := fs.String("pipeline", model.PipelineAssistant, "pipeline type: assistant|chat|story")
	webMode := fs.String("web", "auto", "web search mode: auto|on|off")
	workspaceMode := fs.String("workspace", "auto", "workspace scan mode: auto|on|off")
	allowMissingTools := fs.Bool("allow-missing-tools", false, "continue even if planner-required tools are missing")
	searchQuery := fs.String("search-query", "", "override web search query for this job")
	reasoningLevel := fs.String("reasoning", "auto", "thinking level: auto|fast|deep")
	autonomyMode := fs.String("autonomy", "auto", "autonomy mode: auto|on|off")
	approvalMode := fs.String("approval", "auto", "risk approval mode: auto|on|off")
	verificationMode := fs.String("verify", "auto", "verification mode: auto|on|off")
	verificationIterations := fs.Int("verify-iterations", 2, "verification refinement passes (1-4)")
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
	architectMode, err := applyExecutionProfile(
		args,
		*profile,
		webMode,
		workspaceMode,
		allowMissingTools,
		reasoningLevel,
		autonomyMode,
		approvalMode,
		verificationMode,
		verificationIterations,
		nil,
		nil,
		nil,
	)
	if err != nil {
		die(err.Error())
	}

	instruction := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if instruction == "" {
		die("instruction is required")
	}

	normalizedWebMode := strings.ToLower(strings.TrimSpace(*webMode))
	metadata := map[string]any{}
	metadata["persistent_execution"] = "on"
	metadata["planning_passes"] = 3
	metadata["review_always"] = "on"
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
	if architectMode {
		metadata["architect_mode"] = "on"
	}
	switch normalizedWebMode {
	case "", "auto":
		metadata["web_search"] = "auto"
	case "on", "force":
		metadata["web_search"] = "force"
	case "off":
		metadata["web_search"] = "off"
	default:
		die("invalid --web value (use auto|on|off)")
	}
	switch strings.ToLower(strings.TrimSpace(*workspaceMode)) {
	case "", "auto":
		metadata["workspace_scan"] = "auto"
	case "on", "force":
		metadata["workspace_scan"] = "on"
	case "off":
		metadata["workspace_scan"] = "off"
	default:
		die("invalid --workspace value (use auto|on|off)")
	}
	metadata["allow_missing_tools"] = *allowMissingTools
	if strings.TrimSpace(*searchQuery) != "" {
		metadata["search_query"] = strings.TrimSpace(*searchQuery)
	}
	switch strings.ToLower(strings.TrimSpace(*reasoningLevel)) {
	case "", "auto":
		metadata["reasoning_level"] = "auto"
	case "fast":
		metadata["reasoning_level"] = "fast"
	case "deep":
		metadata["reasoning_level"] = "deep"
	default:
		die("invalid --reasoning value (use auto|fast|deep)")
	}
	switch strings.ToLower(strings.TrimSpace(*autonomyMode)) {
	case "", "auto":
		metadata["autonomy_mode"] = "auto"
	case "on", "true", "enabled":
		metadata["autonomy_mode"] = "on"
	case "off", "false", "disabled", "strict":
		metadata["autonomy_mode"] = "off"
	default:
		die("invalid --autonomy value (use auto|on|off)")
	}
	switch strings.ToLower(strings.TrimSpace(*approvalMode)) {
	case "", "auto":
		metadata["approval_mode"] = "auto"
	case "on", "force":
		metadata["approval_mode"] = "force"
	case "off":
		metadata["approval_mode"] = "off"
	default:
		die("invalid --approval value (use auto|on|off)")
	}
	switch strings.ToLower(strings.TrimSpace(*verificationMode)) {
	case "", "auto":
		metadata["verification_mode"] = "auto"
	case "on", "force":
		metadata["verification_mode"] = "force"
	case "off":
		metadata["verification_mode"] = "off"
	default:
		die("invalid --verify value (use auto|on|off)")
	}
	if *verificationIterations < 1 || *verificationIterations > 4 {
		die("invalid --verify-iterations value (use 1-4)")
	}
	metadata["verification_iterations"] = *verificationIterations
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
