package omni

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) runBench(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: omni bench <list|report>")
	}
	switch args[0] {
	case "list":
		return a.runBenchList(args[1:])
	case "report":
		return a.runBenchReport(args[1:])
	case "run":
		return a.runBenchRun(args[1:])
	case "suite":
		return a.runBenchSuite(args[1:])
	default:
		return fmt.Errorf("usage: omni bench <list|report|run|suite>")
	}
}

func (a *App) runBenchList(args []string) error {
	fs := flag.NewFlagSet("omni bench list", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	rootFlag := fs.String("root", envOrDefault("OMNI_BENCHMARK_ROOT", "benchmarks"), "benchmark manifest root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected bench list argument(s): %s", strings.Join(fs.Args(), " "))
	}
	manifests, err := LoadBenchmarkManifests(resolveOmniResourceRoot(*rootFlag, "benchmarks"))
	if err != nil {
		return err
	}
	for _, manifest := range manifests {
		recipe := manifest.Recipe
		if strings.TrimSpace(recipe) == "" {
			recipe = "none"
		}
		fmt.Fprintf(a.out, "%s\trecipe=%s\t%s\n", manifest.ID, recipe, manifest.Description)
	}
	return nil
}

func (a *App) runBenchReport(args []string) error {
	fs := flag.NewFlagSet("omni bench report", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	workspaceFlag := fs.String("workspace", "", "workspace whose session metrics should be reported; defaults to current directory")
	sessionRootFlag := fs.String("session-root", "", "override session root directory")
	jsonFlag := fs.Bool("json", false, "print JSON report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected bench report argument(s): %s", strings.Join(fs.Args(), " "))
	}
	session, err := loadSessionForWorkspace(*workspaceFlag, *sessionRootFlag)
	if err != nil {
		return err
	}
	report := BenchmarkReportFromSession(session)
	if *jsonFlag {
		blob, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode benchmark report: %w", err)
		}
		_, err = fmt.Fprintln(a.out, string(blob))
		return err
	}
	fmt.Fprintln(a.out, formatBenchmarkReportText(report))
	return nil
}

func (a *App) runBenchRun(args []string) error {
	fs := flag.NewFlagSet("omni bench run", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	rootFlag := fs.String("root", envOrDefault("OMNI_BENCHMARK_ROOT", "benchmarks"), "benchmark manifest root")
	workspaceFlag := fs.String("workspace", "", "benchmark workspace; defaults to an isolated temp directory")
	sessionRootFlag := fs.String("session-root", "", "override session root directory")
	runRootFlag := fs.String("run-root", filepath.Join(os.TempDir(), "omni-bench"), "root for isolated benchmark workspaces")
	jsonFlag := fs.Bool("json", false, "print JSON result")
	dryRunFlag := fs.Bool("dry-run", false, "prepare and report the benchmark without model execution")
	manifestID := ""
	parseArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		manifestID = args[0]
		parseArgs = args[1:]
	}
	if err := fs.Parse(parseArgs); err != nil {
		return err
	}
	if manifestID == "" && fs.NArg() == 1 {
		manifestID = fs.Arg(0)
	} else if fs.NArg() > 0 {
		return fmt.Errorf("usage: omni bench run <id> [--root PATH] [--workspace PATH] [--session-root PATH] [--run-root PATH] [--json] [--dry-run]")
	}
	if manifestID == "" {
		return fmt.Errorf("usage: omni bench run <id> [--root PATH] [--workspace PATH] [--session-root PATH] [--run-root PATH] [--json] [--dry-run]")
	}
	manifests, err := LoadBenchmarkManifests(resolveOmniResourceRoot(*rootFlag, "benchmarks"))
	if err != nil {
		return err
	}
	manifest, ok := findBenchmarkManifest(manifests, manifestID)
	if !ok {
		return fmt.Errorf("benchmark %q not found", manifestID)
	}
	client := a.structuredPlannerClient()
	if client == nil && !*dryRunFlag {
		return fmt.Errorf("llm client is required")
	}
	result, runErr := RunBenchmarkManifest(
		context.Background(),
		manifest,
		client,
		io.Discard,
		io.Discard,
		BenchmarkRunOptions{
			Root:        *runRootFlag,
			Workspace:   *workspaceFlag,
			SessionRoot: *sessionRootFlag,
			DryRun:      *dryRunFlag,
		},
	)
	if *jsonFlag {
		blob, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("encode benchmark run result: %w", err)
		}
		fmt.Fprintln(a.out, string(blob))
	} else {
		fmt.Fprintln(a.out, formatBenchmarkRunResultText(result))
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (a *App) runBenchSuite(args []string) error {
	fs := flag.NewFlagSet("omni bench suite", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	rootFlag := fs.String("root", envOrDefault("OMNI_BENCHMARK_ROOT", "benchmarks"), "benchmark manifest root")
	sessionRootFlag := fs.String("session-root", "", "override session root directory")
	runRootFlag := fs.String("run-root", filepath.Join(os.TempDir(), "omni-bench"), "root for isolated benchmark workspaces")
	jsonFlag := fs.Bool("json", false, "print JSON result")
	dryRunFlag := fs.Bool("dry-run", false, "prepare and report the benchmark suite without model execution")
	suiteID := ""
	parseArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		suiteID = args[0]
		parseArgs = args[1:]
	}
	if err := fs.Parse(parseArgs); err != nil {
		return err
	}
	if suiteID == "" && fs.NArg() == 1 {
		suiteID = fs.Arg(0)
	} else if fs.NArg() > 0 {
		return fmt.Errorf("usage: omni bench suite <app-gauntlet> [--root PATH] [--session-root PATH] [--run-root PATH] [--json] [--dry-run]")
	}
	if suiteID == "" {
		return fmt.Errorf("usage: omni bench suite <app-gauntlet> [--root PATH] [--session-root PATH] [--run-root PATH] [--json] [--dry-run]")
	}
	manifests, err := LoadBenchmarkManifests(resolveOmniResourceRoot(*rootFlag, "benchmarks"))
	if err != nil {
		return err
	}
	ids, err := BenchmarkSuiteManifestIDs(suiteID, manifests)
	if err != nil {
		return err
	}
	client := a.structuredPlannerClient()
	if client == nil && !*dryRunFlag {
		return fmt.Errorf("llm client is required")
	}
	start := time.Now()
	suite := BenchmarkSuiteRunResult{
		ID:        suiteID,
		StartedAt: start.UTC().Format(time.RFC3339),
		Success:   true,
	}
	var suiteErr error
	for _, id := range ids {
		manifest, _ := findBenchmarkManifest(manifests, id)
		result, runErr := RunBenchmarkManifest(
			context.Background(),
			manifest,
			client,
			io.Discard,
			io.Discard,
			BenchmarkRunOptions{
				Root:        filepath.Join(*runRootFlag, sanitizeBenchmarkID(suiteID)),
				SessionRoot: *sessionRootFlag,
				DryRun:      *dryRunFlag,
			},
		)
		suite.Results = append(suite.Results, result)
		if runErr != nil && suiteErr == nil {
			suiteErr = runErr
		}
		if !result.Success {
			suite.Success = false
		}
	}
	suite.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	suite.Duration = time.Since(start).Round(time.Millisecond).String()
	if suiteErr != nil {
		suite.Error = suiteErr.Error()
	}
	if *jsonFlag {
		blob, err := json.MarshalIndent(suite, "", "  ")
		if err != nil {
			return fmt.Errorf("encode benchmark suite result: %w", err)
		}
		fmt.Fprintln(a.out, string(blob))
	} else {
		fmt.Fprintln(a.out, formatBenchmarkSuiteRunResultText(suite))
	}
	if suiteErr != nil {
		return suiteErr
	}
	if !suite.Success {
		return fmt.Errorf("benchmark suite %q failed", suiteID)
	}
	return nil
}

func findBenchmarkManifest(manifests []BenchmarkManifest, id string) (BenchmarkManifest, bool) {
	id = strings.TrimSpace(id)
	for _, manifest := range manifests {
		if manifest.ID == id {
			return manifest, true
		}
	}
	return BenchmarkManifest{}, false
}

func formatBenchmarkRunResultText(result BenchmarkRunResult) string {
	lines := []string{
		fmt.Sprintf("benchmark=%s", result.ID),
		fmt.Sprintf("success=%t duration=%s", result.Success, result.Duration),
		fmt.Sprintf("workspace=%s", result.Workspace),
		fmt.Sprintf("model_calls=%d commands=%d rejected_commands=%d loop_exhaustions=%d", result.Report.ModelCalls, result.Report.Commands, result.Report.RejectedCommands, result.Report.LoopExhaustions),
	}
	if strings.TrimSpace(result.Error) != "" {
		lines = append(lines, "error="+result.Error)
	}
	return strings.Join(lines, "\n")
}

func formatBenchmarkSuiteRunResultText(result BenchmarkSuiteRunResult) string {
	lines := []string{
		fmt.Sprintf("suite=%s", result.ID),
		fmt.Sprintf("success=%t duration=%s benchmarks=%d", result.Success, result.Duration, len(result.Results)),
	}
	for _, bench := range result.Results {
		lines = append(lines, fmt.Sprintf("- %s success=%t workspace=%s", bench.ID, bench.Success, bench.Workspace))
	}
	if strings.TrimSpace(result.Error) != "" {
		lines = append(lines, "error="+result.Error)
	}
	return strings.Join(lines, "\n")
}

func loadOptionalRecipes(root string) []Recipe {
	recipes, err := LoadRecipes(resolveOmniResourceRoot(root, "recipes"))
	if err == nil {
		return recipes
	}
	return nil
}

func (a *App) handleTurn(session *Session, input string, activity *activityIndicator, opts turnRouteOptions) (Turn, string, error) {
	objective := strings.TrimSpace(opts.Objective)
	if objective == "" {
		objective = input
	}
	execSeed := execPromptForTurnRoute(objective, opts)

	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	events := []Event{}
	liveTimeline := isLiveTimelineWriter(a.out)
	timelineStarted := false
	emitEvent := func(eventType, summary string, details map[string]string) {
		evt := a.newEvent(eventType, summary, details)
		events = append(events, evt)
		if !liveTimeline {
			return
		}
		activity.Pause()
		if !timelineStarted {
			fmt.Fprintln(a.out, "\ntimeline")
			fmt.Fprintln(a.out, "--------")
			timelineStarted = true
		}
		a.printTimelineEvent(evt)
		activity.Resume()
	}

	_ = a.runLogger.Log("structured_command", "turn_started", map[string]interface{}{
		"user_input":       input,
		"permission_mode":  session.Permission,
		"workspace":        session.WorkspacePath,
		"active_directory": session.ActiveDirectoryPath,
		"execution_policy": "thinking_pilot_entry_then_execution",
	})

	if activity == nil {
		activity = &activityIndicator{}
	}
	execCtx, cancel := context.WithTimeout(context.Background(), defaultCommandDecisionTimeout)
	signalCtx, stopSignal := signal.NotifyContext(execCtx, os.Interrupt)
	defer stopSignal()
	stopEsc := a.startEscapeInterrupt(signalCtx, cancel, activity, emitEvent)
	defer func() { stopEsc() }()
	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	activeDirectory := a.resolveActiveDirectoryForTurn(session, objective, emitEvent)
	prepCtx := a.prepareInteractiveTurnContext(signalCtx, objective, activeDirectory, emitEvent)
	memoryCtx := prepCtx.Memory
	sessionMemories := append([]SessionMemory(nil), session.Memories...)
	sessionMemories = append(sessionMemories, prepCtx.SessionMemories...)
	thoughtStore, _ := NewThoughtChannelStore("", session.WorkspaceHash, turnID)
	thinkingService := a.thinkingService
	if thinkingService != nil && thoughtStore != nil {
		if svc, ok := thinkingService.(OllamaThinkingService); ok {
			svc.Store = thoughtStore
			svc.Deps = a.buildThinkingToolDeps()
			thinkingService = svc
		}
	}

	emitEvent("thinking_pilot_started", "Thinking pilot is the turn entry point", map[string]string{
		"turn_id": turnID,
		"route":   firstNonEmpty(opts.ReasonCode, "default"),
	})
	execPrompt := execSeed
	pilotToolTask := ""
	pilotTrigger := firstNonEmpty(strings.TrimSpace(opts.PilotTrigger), "turn_entry")
	if thinkingService != nil {
		pilotOutcome, pilotErr := thinkingService.OrchestrateTurn(signalCtx, ThinkingInput{
			TurnID:          turnID,
			Step:            0,
			Trigger:         pilotTrigger,
			UserPrompt:      objective,
			WorkingDir:      activeDirectory,
			SessionMemories: sessionMemories,
			PrepContext:     prepCtx.Bundle,
			ActivePrompt:    NewActivePromptContext(objective, "", explicitReactAppAcceptanceCriteria(objective, "")),
		}, func(evt StructuredCommandEvent) {
			emitEvent(evt.Type, evt.Summary, evt.Details)
		})
		if pilotErr != nil {
			emitEvent("thinking_pilot_failed", "Thinking pilot failed; falling back to execution layer", map[string]string{
				"error": truncateOutput(pilotErr.Error()),
			})
		} else {
			emitEvent("thinking_pilot_decision", "Thinking pilot decided next action", map[string]string{
				"action":              string(pilotOutcome.Action),
				"channel_id":          pilotOutcome.ChannelID,
				"execution_prompt":    truncateOutput(pilotOutcome.ExecutionPrompt),
				"execution_tool_task": truncateOutput(pilotOutcome.ExecutionToolTask),
			})
			if opts.ThinkOnly {
				assistantResponse := strings.TrimSpace(firstNonEmpty(pilotOutcome.DirectAnswer, pilotOutcome.Conclusion))
				if assistantResponse == "" && pilotOutcome.Action == ThinkingActionInvokeExecution {
					assistantResponse = "Thinking concluded that execution would help, but /think is reasoning-only. Refine the question or use /build for implementation."
					if task := strings.TrimSpace(pilotOutcome.ExecutionToolTask); task != "" {
						assistantResponse += "\n\nSuggested next work: " + task
					}
				}
				if assistantResponse == "" {
					assistantResponse = "I could not produce a direct answer from the thinking channel."
				}
				assistantResponse = a.reviewFinalResponse(context.Background(), input, assistantResponse, []string{
					"pilot_action=think_only",
					"channel_id=" + pilotOutcome.ChannelID,
				}, emitEvent)
				a.persistInteractiveTurnMemory(context.Background(), turnID, input, assistantResponse, memoryCtx.Tags, CommandDecisionResult{Answer: assistantResponse}, emitEvent)
				emitEvent("thinking_pilot_direct_answer", "Thinking pilot answered via /think", map[string]string{
					"channel_id": pilotOutcome.ChannelID,
				})
				reason := firstNonEmpty(opts.ReasonCode, "slash_think")
				turn := Turn{
					ID:                   turnID,
					UserInput:            input,
					IntentClassification: IntentExecution,
					Confidence:           1.0,
					ReasonCodes:          []string{reason},
					Response:             assistantResponse,
					Events:               events,
					CreatedAt:            nowUTC(),
				}
				return turn, assistantResponse, nil
			}
			if pilotOutcome.Action == ThinkingActionDirectAnswer && !opts.ForceExecution {
				assistantResponse := strings.TrimSpace(pilotOutcome.DirectAnswer)
				assistantResponse = a.reviewFinalResponse(context.Background(), input, assistantResponse, []string{
					"pilot_action=direct_answer",
					"channel_id=" + pilotOutcome.ChannelID,
				}, emitEvent)
				a.persistInteractiveTurnMemory(context.Background(), turnID, input, assistantResponse, memoryCtx.Tags, CommandDecisionResult{Answer: assistantResponse}, emitEvent)
				emitEvent("thinking_pilot_direct_answer", "Thinking pilot answered user directly", map[string]string{
					"channel_id": pilotOutcome.ChannelID,
				})
				turn := Turn{
					ID:                   turnID,
					UserInput:            input,
					IntentClassification: IntentExecution,
					Confidence:           1.0,
					ReasonCodes:          []string{"thinking_pilot_direct_answer"},
					Response:             assistantResponse,
					Events:               events,
					CreatedAt:            nowUTC(),
				}
				return turn, assistantResponse, nil
			}
			if strings.TrimSpace(pilotOutcome.ExecutionPrompt) != "" {
				execPrompt = strings.TrimSpace(pilotOutcome.ExecutionPrompt)
			} else if opts.ForceExecution {
				execPrompt = execSeed
			}
			pilotToolTask = strings.TrimSpace(pilotOutcome.ExecutionToolTask)
		}
	} else if opts.ThinkOnly {
		assistantResponse := a.reviewFinalResponse(context.Background(), input, "Thinking is disabled (OMNI_DISABLE_THINKING). Re-enable thinking or ask without /think.", nil, emitEvent)
		turn := Turn{
			ID:                   turnID,
			UserInput:            input,
			IntentClassification: IntentExecution,
			Confidence:           1.0,
			ReasonCodes:          []string{firstNonEmpty(opts.ReasonCode, "slash_think")},
			Response:             assistantResponse,
			Events:               events,
			CreatedAt:            nowUTC(),
		}
		return turn, assistantResponse, nil
	}
	if pilotToolTask != "" {
		execPrompt = strings.TrimSpace(execPrompt + "\n\nPilot execution scope: " + pilotToolTask)
	}
	emitEvent("structured_command_started", "Execution layer invoked by thinking pilot", map[string]string{
		"permission_mode": string(session.Permission),
		"exec_prompt":     truncateOutput(execPrompt),
	})

	result, execErr := runStructuredCommandDecisionWithConfig(
		signalCtx,
		execPrompt,
		session.Messages,
		a.structuredPlannerClient(),
		&stdoutBuf,
		&stderrBuf,
		func(evt StructuredCommandEvent) {
			emitEvent(evt.Type, evt.Summary, evt.Details)
		},
		func(ctx context.Context, question string) (string, error) {
			stopEsc()
			activity.Pause()
			fmt.Fprintf(a.out, "\nassistant?> %s\nuser> ", question)
			answer, err := readLineFromReader(ctx, a.in)
			activity.Resume()
			if ctx.Err() == nil {
				stopEsc = a.startEscapeInterrupt(signalCtx, cancel, activity, emitEvent)
			}
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(answer), nil
		},
		structuredCommandDecisionRunConfig{
			SessionMemories:         sessionMemories,
			PrepContext:             prepCtx.Bundle,
			CurrentWorkingDirectory: activeDirectory,
			Recipes:                 a.recipes,
			PromptInterpreter:       a.promptInterpreter,
			ContextSummarizer:       a.contextSummarizer,
			CompletionChecker:       a.completionChecker,
			Evaluator:               a.evaluator,
			EvaluatorThreshold:      a.evaluatorThreshold,
			ShellSpecialist:         a.shellSpecialist,
			CodeContentSpecialist:   a.codeSpecialist,
			CursorArchitectAgent:    a.cursorArchitect,
			CodexArchitectAgent:     a.codexArchitect,
			EnableCommandCache:      a.enableCommandCache,
			CommandCacheRoot:        a.commandCacheRoot,
			ThinkingService:         thinkingService,
			ThoughtTurnID:           turnID,
			TaskMode:                opts.TaskMode,
		},
	)
	cancel()

	responseStdout, responseStderr := structuredCommandResponseStreams(result, stdoutBuf.String(), stderrBuf.String(), execErr)
	assistantResponse := formatStructuredCommandChatResponse(result, responseStdout, responseStderr, "")
	if execErr != nil {
		assistantResponse = formatStructuredCommandChatResponse(result, responseStdout, responseStderr, execErr.Error())
		eventType := "structured_command_failed"
		eventSummary := "Structured command execution failed"
		if result.PartialProgress {
			eventType = "structured_planner_failed_after_progress"
			eventSummary = "Planner failed after successful command progress"
		}
		details := map[string]string{
			"error":     execErr.Error(),
			"command":   result.Command,
			"exit_code": fmt.Sprintf("%d", result.ExitCode),
			"stdout":    truncateOutput(stdoutBuf.String()),
			"stderr":    truncateOutput(stderrBuf.String()),
		}
		if result.PartialProgress {
			details["pending_objectives"] = pendingStructuredObjectiveIDs(result.ObjectiveLedger)
		}
		if isTransientStructuredLLMError(execErr) {
			details["diagnosis"] = classifyStructuredLLMFailure(execErr)
			if result.PartialProgress {
				details["mitigation"] = "Ollama timed out while planning the next step after successful command progress; rerun the request to continue from the updated workspace state."
			} else {
				details["mitigation"] = "Ollama backend failed before command completion; inspect journalctl -u ollama and consider CPU library mode."
			}
		}
		emitEvent(eventType, eventSummary, details)
	} else {
		emitEvent("structured_command_completed", "Structured command executed", map[string]string{
			"command":   result.Command,
			"exit_code": fmt.Sprintf("%d", result.ExitCode),
			"stdout":    truncateOutput(responseStdout),
			"stderr":    truncateOutput(responseStderr),
		})
	}
	for _, memory := range rememberCapabilityMemoriesFromObservations(session, result.Observations) {
		emitEvent("capability_memory_stored", "Stored structured self-correction capability memory", map[string]string{
			"kind":    memory.Kind,
			"content": truncateOutput(memory.Content),
		})
	}
	if memory, ok := rememberValidatedPlaybookFromResult(session, input, result, "structured_planner"); ok {
		emitEvent("validated_playbook_stored", "Stored validated playbook from accepted command evidence", map[string]string{
			"kind":         memory.Kind,
			"tags":         strings.Join(memory.Tags, ","),
			"content":      truncateOutput(validatedPlaybookMemorySummary(memory)),
			"scope_policy": "advisory_only_validators_still_decide",
		})
	}
	assistantResponse = a.reviewFinalResponse(context.Background(), input, assistantResponse, structuredResponseReviewEvidence(result, responseStdout, responseStderr, execErr), emitEvent)
	a.persistInteractiveTurnMemory(context.Background(), turnID, input, assistantResponse, memoryCtx.Tags, result, emitEvent)

	reasonCode := firstNonEmpty(opts.ReasonCode, "structured_llm_command")
	turn := Turn{
		ID:                   turnID,
		UserInput:            input,
		IntentClassification: IntentExecution,
		Confidence:           1.0,
		ReasonCodes:          []string{reasonCode},
		Response:             assistantResponse,
		Events:               events,
		CreatedAt:            nowUTC(),
	}

	return turn, assistantResponse, nil
}

func (a *App) handleMicroQueueTurn(session *Session, objective string) (Turn, string, error) {
	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	events := []Event{a.newEvent("micro_queue_started", "Manager-manager micro job queue started", map[string]string{
		"permission_mode": string(session.Permission),
	})}

	_ = a.runLogger.Log("micro_queue", "turn_started", map[string]interface{}{
		"objective":       objective,
		"permission_mode": session.Permission,
		"workspace":       session.WorkspacePath,
	})

	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	execCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	result, execErr := ExecuteMicroJobQueue(execCtx, objective, session.WorkspacePath, a.ollama, &stdoutBuf, &stderrBuf, DefaultMicroJobQueueConfig())
	cancel()

	if result.ProjectProfile.Summary != "" {
		events = append(events, a.newEvent("micro_queue_project_profiled", "Project run profile created", map[string]string{
			"summary":        result.ProjectProfile.Summary,
			"run_commands":   strings.Join(result.ProjectProfile.RunCommands, " | "),
			"test_commands":  strings.Join(result.ProjectProfile.TestCommands, " | "),
			"build_commands": strings.Join(result.ProjectProfile.BuildCommands, " | "),
		}))
	}
	if len(result.Jobs) > 0 {
		events = append(events, a.newEvent("micro_queue_plan_created", "Micro job queue plan created", map[string]string{
			"job_count": fmt.Sprintf("%d", len(result.Jobs)),
		}))
	}
	for _, item := range result.Results {
		events = append(events, a.newEvent("micro_job_completed", "Micro job completed", map[string]string{
			"job_id":    item.Job.ID,
			"done":      fmt.Sprintf("%t", item.Done),
			"exit_code": fmt.Sprintf("%d", item.ExitCode),
			"command":   truncateOutput(item.Command),
			"error":     item.Error,
		}))
	}

	response := formatMicroQueueResponse(result, stdoutBuf.String(), stderrBuf.String(), "")
	if execErr != nil {
		response = formatMicroQueueResponse(result, stdoutBuf.String(), stderrBuf.String(), execErr.Error())
		events = append(events, a.newEvent("micro_queue_failed", "Micro job queue failed", map[string]string{"error": execErr.Error()}))
	} else {
		events = append(events, a.newEvent("micro_queue_completed", "Micro job queue completed", map[string]string{
			"done":      fmt.Sprintf("%t", result.Done),
			"jobs":      fmt.Sprintf("%d", len(result.Jobs)),
			"completed": fmt.Sprintf("%d", len(result.Results)),
		}))
	}
	response = a.reviewFinalResponse(context.Background(), "/micro "+objective, response, []string{
		result.Summary,
		stdoutBuf.String(),
		stderrBuf.String(),
	}, func(eventType, summary string, details map[string]string) {
		events = append(events, a.newEvent(eventType, summary, details))
	})

	turn := Turn{
		ID:                   turnID,
		UserInput:            "/micro " + objective,
		IntentClassification: IntentExecution,
		Confidence:           1.0,
		ReasonCodes:          []string{"micro_job_queue"},
		Response:             response,
		Events:               events,
		CreatedAt:            nowUTC(),
	}
	return turn, response, nil
}
