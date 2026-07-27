package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

func runChat(c *client.Client, args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	profile := fs.String("profile", "default", "execution profile for chat: default|architect")
	sessionID := fs.String("session", "", "session/thread identifier for this interactive chat")
	webMode := fs.String("web", "off", "web search mode for chat turns: auto|on|off")
	workspaceMode := fs.String("workspace", "auto", "workspace scan mode for chat turns: auto|on|off")
	localMedia := fs.Bool("local-media", true, "enable local host media automation via capability matching (no exact phrase required)")
	localBrowser := fs.Bool("local-browser", true, "enable local browser scan automation via capability matching")
	localScreen := fs.Bool("local-screen", true, "enable local screen reading automation via capability matching")
	localShell := fs.Bool("local-shell", true, "enable local shell automation via capability matching in current directory")
	localAudio := fs.Bool("local-audio", true, "enable local audio-notes automation via capability matching")
	allowMissingTools := fs.Bool("allow-missing-tools", true, "continue even if planner-required tools are missing")
	reasoningLevel := fs.String("reasoning", "fast", "thinking level: auto|fast|deep")
	autonomyMode := fs.String("autonomy", "on", "autonomy mode for chat turns: auto|on|off")
	approvalMode := fs.String("approval", "off", "risk approval mode: auto|on|off")
	verificationMode := fs.String("verify", "off", "verification mode: auto|on|off")
	verificationIterations := fs.Int("verify-iterations", 1, "verification refinement passes (1-4)")
	confirmActions := fs.Bool("confirm-actions", true, "require explicit confirmation before executing local automation actions")
	interval := fs.Duration("interval", 2*time.Second, "poll interval while waiting for each turn")
	progress := fs.Bool("progress", true, "print live stage/event updates while waiting for each turn")
	verbose := fs.Bool("verbose", false, "print full debug trace (including LLM prompts and full context dumps) while waiting")
	maxChars := fs.Int("max-chars", 1200, "max characters shown per streamed LLM/context entry (0 disables truncation)")
	modelAnalyze := fs.String("model-analyze", "", "override analyze model for this chat session")
	modelResponse := fs.String("model-response", "", "override response model for this chat session")
	modelSearch := fs.String("model-search", "", "override search-query model for this chat session")
	modelTagger := fs.String("model-tagger", "", "override tagging model for this chat session")
	modelPlan := fs.String("model-plan", "", "override planner model for this chat session")
	modelVerify := fs.String("model-verify", "", "override verification evaluator model for this chat session")
	modelMemory := fs.String("model-memory", "", "override memory-inference model for this chat session")
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
		verbose,
		maxChars,
		localShell,
	)
	if err != nil {
		die(err.Error())
	}

	baseMetadata := map[string]any{}
	baseMetadata["persistent_execution"] = "on"
	baseMetadata["planning_passes"] = 3
	baseMetadata["review_always"] = "on"
	chatCWD := ""
	if dir, err := os.Getwd(); err == nil && strings.TrimSpace(dir) != "" {
		chatCWD = strings.TrimSpace(dir)
	}
	session := strings.TrimSpace(*sessionID)
	if session == "" {
		session = defaultProjectScopedSessionID(chatCWD)
	}
	hostSnapshot := discoverHostEnvironmentSnapshot(chatCWD)
	applyHostEnvironmentMetadata(baseMetadata, hostSnapshot)
	applyHostTemporalMetadata(baseMetadata, time.Now())
	if architectMode {
		baseMetadata["architect_mode"] = "on"
	}
	switch strings.ToLower(strings.TrimSpace(*webMode)) {
	case "", "auto":
		baseMetadata["web_search"] = "auto"
	case "on", "force":
		baseMetadata["web_search"] = "force"
	case "off":
		baseMetadata["web_search"] = "off"
	default:
		die("invalid --web value (use auto|on|off)")
	}
	switch strings.ToLower(strings.TrimSpace(*workspaceMode)) {
	case "", "auto":
		baseMetadata["workspace_scan"] = "auto"
	case "on", "force":
		baseMetadata["workspace_scan"] = "on"
	case "off":
		baseMetadata["workspace_scan"] = "off"
	default:
		die("invalid --workspace value (use auto|on|off)")
	}
	baseMetadata["allow_missing_tools"] = *allowMissingTools
	switch strings.ToLower(strings.TrimSpace(*reasoningLevel)) {
	case "", "auto":
		baseMetadata["reasoning_level"] = "auto"
	case "fast":
		baseMetadata["reasoning_level"] = "fast"
	case "deep":
		baseMetadata["reasoning_level"] = "deep"
	default:
		die("invalid --reasoning value (use auto|fast|deep)")
	}
	switch strings.ToLower(strings.TrimSpace(*autonomyMode)) {
	case "", "auto":
		baseMetadata["autonomy_mode"] = "auto"
	case "on", "true", "enabled":
		baseMetadata["autonomy_mode"] = "on"
	case "off", "false", "disabled", "strict":
		baseMetadata["autonomy_mode"] = "off"
	default:
		die("invalid --autonomy value (use auto|on|off)")
	}
	switch strings.ToLower(strings.TrimSpace(*approvalMode)) {
	case "", "auto":
		baseMetadata["approval_mode"] = "auto"
	case "on", "force":
		baseMetadata["approval_mode"] = "force"
	case "off":
		baseMetadata["approval_mode"] = "off"
	default:
		die("invalid --approval value (use auto|on|off)")
	}
	switch strings.ToLower(strings.TrimSpace(*verificationMode)) {
	case "", "auto":
		baseMetadata["verification_mode"] = "auto"
	case "on", "force":
		baseMetadata["verification_mode"] = "force"
	case "off":
		baseMetadata["verification_mode"] = "off"
	default:
		die("invalid --verify value (use auto|on|off)")
	}
	if *verificationIterations < 1 || *verificationIterations > 4 {
		die("invalid --verify-iterations value (use 1-4)")
	}
	baseMetadata["verification_iterations"] = *verificationIterations
	if strings.TrimSpace(*modelAnalyze) != "" {
		baseMetadata["model_analyze"] = strings.TrimSpace(*modelAnalyze)
	}
	if strings.TrimSpace(*modelResponse) != "" {
		baseMetadata["model_response"] = strings.TrimSpace(*modelResponse)
	}
	if strings.TrimSpace(*modelSearch) != "" {
		baseMetadata["model_search"] = strings.TrimSpace(*modelSearch)
	}
	if strings.TrimSpace(*modelTagger) != "" {
		baseMetadata["model_tagger"] = strings.TrimSpace(*modelTagger)
	}
	if strings.TrimSpace(*modelPlan) != "" {
		baseMetadata["model_plan"] = strings.TrimSpace(*modelPlan)
	}
	if strings.TrimSpace(*modelVerify) != "" {
		baseMetadata["model_verify"] = strings.TrimSpace(*modelVerify)
	}
	if strings.TrimSpace(*modelMemory) != "" {
		baseMetadata["model_memory"] = strings.TrimSpace(*modelMemory)
	}
	agentOverrides.ApplyToMetadata(baseMetadata)
	if err := persistHostCapabilityMemory(c, hostSnapshot); err != nil {
		fmt.Fprintf(os.Stderr, "warn: capability memory sync failed: %v\n", err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	input := newChatInputReader(scanner)

	lastJobID := int64(0)
	pendingInputs := make([]string, 0, 4)
	if initialInput := strings.TrimSpace(strings.Join(fs.Args(), " ")); initialInput != "" {
		pendingInputs = append(pendingInputs, initialInput)
	}
	shellState := &localShellState{}
	var pendingAction *chatActionCandidate
	ui := newChatUI()
	restorePermissionPrompt := installPermissionPromptFunc(func(key, reason, storePath, description string) (bool, error) {
		return promptChatPermissionDecision(input, ui, key, reason, storePath, description)
	})
	defer restorePermissionPrompt()
	ui.printBanner(session, architectMode)
	if len(agentOverrides.ToMap()) > 0 {
		emitSystem(ui, agentOverrides.Summary())
	}

	for {
		var line string
		if len(pendingInputs) > 0 {
			line = pendingInputs[0]
			pendingInputs = pendingInputs[1:]
			emitUser(ui, line)
		} else {
			fmt.Print(userPrompt(ui))
			rawLine, eof, err := input.readBlocking()
			if err != nil {
				die(err.Error())
			}
			if eof {
				fmt.Println("")
				return
			}
			line = strings.TrimSpace(rawLine)
		}

		if line == "" {
			continue
		}

		if pendingAction != nil {
			decision, feedback := interpretConfirmationReply(line)
			if decision == confirmationDecisionApprove {
				quit := executeConfirmedChatAction(
					c,
					input,
					session,
					baseMetadata,
					&lastJobID,
					&pendingInputs,
					pendingAction,
					*interval,
					*progress,
					*verbose,
					*maxChars,
					*localShell,
					shellState,
					ui,
				)
				pendingAction = nil
				if quit {
					return
				}
				continue
			}
			if strings.HasPrefix(line, "/") {
				pendingAction = nil
			} else {
				rejected := pendingAction
				pendingAction = nil
				feedback = strings.TrimSpace(feedback)
				if decision == confirmationDecisionReject && feedback == "" {
					emitAssistant(ui, "Action canceled. Tell me what you want me to do instead.")
					continue
				}
				revised := revisedChatActionCandidate(
					rejected,
					feedback,
					*localMedia,
					*localBrowser,
					*localScreen,
					*localShell,
					*localAudio,
					shellState,
				)
				if revised == nil || strings.TrimSpace(revised.Input) == "" {
					emitAssistant(ui, "I couldn't derive a revised action from that feedback. Tell me the exact action to run.")
					continue
				}
				pendingAction = revised
				emitAssistant(ui, fmt.Sprintf("Proposed action: %s. Reply `yes` to proceed, reply `no` to cancel, or provide feedback to revise.", candidateSummaryWithSpecialist(pendingAction)))
				continue
			}
		}

		if strings.HasPrefix(line, "/") {
			handled, quit := handleChatReplCommand(line, &session, &lastJobID, baseMetadata, agentOverrides, ui)
			if quit {
				return
			}
			if handled {
				continue
			}
		}

		candidate := buildChatActionCandidate(
			line,
			*localMedia,
			*localBrowser,
			*localScreen,
			*localShell,
			*localAudio,
			shellState,
		)
		if requiresActionConfirmation(*confirmActions, candidate) {
			emitAssistant(ui, fmt.Sprintf("Proposed action: %s. Reply `yes` to proceed, reply `no` to cancel, or provide feedback to revise.", candidateSummaryWithSpecialist(candidate)))
			pendingAction = candidate
			continue
		}

		quit := executeConfirmedChatAction(
			c,
			input,
			session,
			baseMetadata,
			&lastJobID,
			&pendingInputs,
			candidate,
			*interval,
			*progress,
			*verbose,
			*maxChars,
			*localShell,
			shellState,
			ui,
		)
		if quit {
			return
		}
	}
}
