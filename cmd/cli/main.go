package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/media"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/version"
)

func main() {
	if err := applyInvocationCWDFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "warn: %v\n", err)
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	baseURL := getenv("CORE_URL", "http://localhost:8090")
	timeout := getenvDuration("CLI_TIMEOUT", 30*time.Second)
	apiClient := client.New(baseURL, timeout)

	if tryRunServiceShortcut(os.Args[1:]) {
		return
	}

	cmd := os.Args[1]
	if cmd == "version" || cmd == "--version" || cmd == "-v" {
		runVersion(os.Args[2:])
		return
	}
	if strings.HasPrefix(cmd, "service:") {
		runServiceWithPreset(strings.TrimPrefix(cmd, "service:"), os.Args[2:])
		return
	}

	switch cmd {
	case "enqueue":
		runEnqueue(apiClient, os.Args[2:])
	case "chat":
		runChat(apiClient, os.Args[2:])
	case "list":
		runList(apiClient, os.Args[2:])
	case "show":
		runShow(apiClient, os.Args[2:])
	case "watch":
		runWatch(apiClient, os.Args[2:])
	case "interrupt":
		runInterrupt(apiClient, os.Args[2:])
	case "cancel":
		runCancel(apiClient, os.Args[2:])
	case "replan":
		runReplan(apiClient, os.Args[2:])
	case "continue":
		runContinueJob(apiClient, os.Args[2:])
	case "remember":
		runRemember(apiClient, os.Args[2:])
	case "memory-candidates":
		runMemoryCandidates(apiClient, os.Args[2:])
	case "ingest":
		runIngest(apiClient, os.Args[2:])
	case "media-index":
		runMediaIndex(apiClient, os.Args[2:])
	case "media-search":
		runMediaSearch(os.Args[2:])
	case "browser-scan":
		runBrowserScan(os.Args[2:])
	case "screen-read":
		runScreenRead(os.Args[2:])
	case "research":
		runResearch(apiClient, os.Args[2:])
	case "audio-notes":
		runAudioNotes(apiClient, os.Args[2:])
	case "permissions":
		runPermissions(os.Args[2:])
	case "feedback":
		runFeedback(apiClient, os.Args[2:])
	case "build":
		runBuild(os.Args[2:])
	case "update":
		runUpdate(os.Args[2:])
	case "stash":
		runStash(os.Args[2:])
	case "uninstall":
		runUninstall(os.Args[2:])
	case "migrate:fresh":
		runMigrateFresh(apiClient, os.Args[2:])
	case "status":
		runStatus(apiClient, os.Args[2:])
	case "metrics":
		runMetrics(apiClient, os.Args[2:])
	case "core:status":
		runCoreStatus(os.Args[2:])
	case "queue:status":
		runQueueStatus(apiClient, os.Args[2:])
	case "ollama:status":
		runOllamaStatus(os.Args[2:])
	case "web:status":
		runWebStatus(os.Args[2:])
	case "service":
		runService(os.Args[2:])
	case "config":
		runConfig(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func runVersion(args []string) {
	jsonOut := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
		}
	}
	if jsonOut {
		payload, err := json.MarshalIndent(version.JSON(), "", "  ")
		if err != nil {
			die("encode version: " + err.Error())
		}
		fmt.Println(string(payload))
		return
	}
	fmt.Println(version.PrintName("omni"))
}

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
	_ = fs.Parse(args)
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
	if err := persistHostCapabilityMemory(c, hostSnapshot); err != nil {
		fmt.Fprintf(os.Stderr, "warn: capability memory sync failed: %v\n", err)
	}

	job, err := c.Enqueue(context.Background(), instruction, *pipeline, metadata)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("enqueued job %d (%s) status=%s\n", job.ID, job.Pipeline, job.Status)
}

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
	_ = fs.Parse(args)
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
			handled, quit := handleChatReplCommand(line, &session, &lastJobID, ui)
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

func defaultProjectScopedSessionID(cwd string) string {
	clean := strings.TrimSpace(filepath.Clean(cwd))
	if clean == "" || clean == "." {
		return fmt.Sprintf("chat-%d", time.Now().Unix())
	}

	base := strings.ToLower(strings.TrimSpace(filepath.Base(clean)))
	base = normalizeSessionSlug(base)
	if base == "" {
		base = "workspace"
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.ToLower(clean)))
	return fmt.Sprintf("chat-%s-%08x", base, hasher.Sum32())
}

func normalizeSessionSlug(value string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if prevDash {
			continue
		}
		b.WriteRune('-')
		prevDash = true
	}
	return strings.Trim(b.String(), "-")
}

type chatActionCandidate struct {
	Kind           string
	Input          string
	Summary        string
	SpecialistID   string
	SpecialistName string
}

func withCandidateSpecialist(candidate *chatActionCandidate) *chatActionCandidate {
	if candidate == nil {
		return nil
	}
	role := specialist.ForLocalCapability(candidate.Kind)
	candidate.SpecialistID = strings.TrimSpace(role.ID)
	candidate.SpecialistName = strings.TrimSpace(role.Name)
	return candidate
}

func candidateSpecialistRole(candidate *chatActionCandidate) specialist.Role {
	if candidate == nil {
		return specialist.ForLocalCapability("")
	}
	role := specialist.ForLocalCapability(candidate.Kind)
	if strings.TrimSpace(candidate.SpecialistID) != "" {
		role.ID = strings.TrimSpace(candidate.SpecialistID)
	}
	if strings.TrimSpace(candidate.SpecialistName) != "" {
		role.Name = strings.TrimSpace(candidate.SpecialistName)
	}
	return role
}

func candidateSummaryWithSpecialist(candidate *chatActionCandidate) string {
	if candidate == nil {
		return ""
	}
	role := candidateSpecialistRole(candidate)
	if strings.TrimSpace(role.ID) == "" {
		return candidate.Summary
	}
	return fmt.Sprintf("[%s] %s", strings.TrimSpace(role.ID), candidate.Summary)
}

func adoptFreshLocalShellSuggestionCandidate(
	candidate *chatActionCandidate,
	shellState *localShellState,
	previousSuggestedCommand string,
	previousSuggestedAt time.Time,
) *chatActionCandidate {
	if candidate == nil || shellState == nil {
		return candidate
	}
	if strings.TrimSpace(candidate.Kind) != "local_shell" {
		return candidate
	}
	if !shellState.LastSuggestedAt.After(previousSuggestedAt) {
		return candidate
	}
	suggested := strings.TrimSpace(shellState.LastSuggestedCommand)
	if suggested == "" {
		return candidate
	}
	if strings.EqualFold(strings.TrimSpace(previousSuggestedCommand), suggested) {
		return candidate
	}
	intent, ok := parseLocalShellIntent(suggested, nil)
	if !ok || strings.TrimSpace(intent.Action) != "run_command" || strings.TrimSpace(intent.Command) == "" {
		return candidate
	}
	return withCandidateSpecialist(&chatActionCandidate{
		Kind:    "local_shell",
		Input:   strings.TrimSpace(intent.Command),
		Summary: describeLocalShellIntent(intent),
	})
}

func localAutomationSourceLine(kind string, detail string) string {
	role := specialist.ForLocalCapability(kind)
	if strings.TrimSpace(role.ID) == "" {
		return strings.TrimSpace(kind) + ": " + strings.TrimSpace(detail)
	}
	return fmt.Sprintf("%s (%s): %s", strings.TrimSpace(kind), strings.TrimSpace(role.ID), strings.TrimSpace(detail))
}

func specialistModelOverrideForRole(roleID string) string {
	key := strings.TrimSpace(specialist.EnvVarForRoleID(roleID))
	if key == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(key))
}

func applySpecialistTurnOverrides(metadata map[string]any, roleID string) {
	if metadata == nil {
		return
	}
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return
	}
	metadata["specialist_role_id"] = roleID

	model := specialistModelOverrideForRole(roleID)
	if model == "" {
		return
	}
	for _, key := range []string{
		"model_plan",
		"model_analyze",
		"model_response",
		"model_verify",
	} {
		if _, exists := metadata[key]; exists {
			continue
		}
		metadata[key] = model
	}
}

func specialistRoleIDForCandidateTurn(candidate *chatActionCandidate) string {
	if candidate == nil {
		return ""
	}
	switch strings.TrimSpace(candidate.Kind) {
	case "local_media", "local_browser", "local_screen", "local_shell", "local_audio":
		return strings.TrimSpace(candidate.SpecialistID)
	default:
		return ""
	}
}

func buildChatActionCandidate(
	line string,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
	shellState *localShellState,
) *chatActionCandidate {
	clean := strings.TrimSpace(line)
	if clean == "" {
		return nil
	}

	kind := matchChatCapabilityKind(clean, localMedia, localBrowser, localScreen, localShell, localAudio)
	switch kind {
	case "local_media":
		if intent, ok := parsePlaybackControlIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_media",
				Input:   clean,
				Summary: describePlaybackControlIntent(intent),
			})
		}
		if intent, ok := parseNextEpisodeIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_media",
				Input:   clean,
				Summary: describeNextEpisodeIntent(intent),
			})
		}
		if intent, ok := parsePlaybackContextIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_media",
				Input:   clean,
				Summary: describePlaybackContextIntent(intent),
			})
		}
		return withCandidateSpecialist(&chatActionCandidate{
			Kind:    "local_media",
			Input:   clean,
			Summary: fmt.Sprintf("use local media capabilities to handle `%s`", truncateForWatch(clean, 200)),
		})
	case "local_browser":
		if intent, ok := parseBrowserScanIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_browser",
				Input:   clean,
				Summary: describeBrowserScanIntent(intent),
			})
		}
		return withCandidateSpecialist(&chatActionCandidate{
			Kind:    "local_browser",
			Input:   clean,
			Summary: fmt.Sprintf("use local browser inspection capabilities to handle `%s`", truncateForWatch(clean, 200)),
		})
	case "local_screen":
		if intent, ok := parseScreenReadIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_screen",
				Input:   clean,
				Summary: describeScreenReadIntent(intent),
			})
		}
		return withCandidateSpecialist(&chatActionCandidate{
			Kind:    "local_screen",
			Input:   clean,
			Summary: fmt.Sprintf("use local screen read capabilities to handle `%s`", truncateForWatch(clean, 200)),
		})
	case "local_shell":
		if intent, ok := parseLocalShellIntent(clean, shellState); ok {
			if shouldRouteLocalShellIntentToCore(clean, intent) {
				break
			}
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_shell",
				Input:   clean,
				Summary: describeLocalShellIntent(intent),
			})
		}
		break
	case "local_audio":
		if intent, ok := parseLocalAudioNotesIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_audio",
				Input:   clean,
				Summary: describeLocalAudioNotesIntent(intent),
			})
		}
		return withCandidateSpecialist(&chatActionCandidate{
			Kind:    "local_audio",
			Input:   clean,
			Summary: fmt.Sprintf("use local audio notes capabilities to handle `%s`", truncateForWatch(clean, 200)),
		})
	}

	return withCandidateSpecialist(&chatActionCandidate{
		Kind:    "core_job",
		Input:   clean,
		Summary: fmt.Sprintf("submit this request to the core pipeline (`%s`) and run planning -> execution -> review", truncateForWatch(clean, 220)),
	})
}

func requiresActionConfirmation(confirmActions bool, candidate *chatActionCandidate) bool {
	if !confirmActions || candidate == nil {
		return false
	}
	return strings.TrimSpace(candidate.Kind) != "core_job"
}

func shouldAutoApproveCandidate(candidate *chatActionCandidate) bool {
	if candidate == nil {
		return false
	}
	switch strings.TrimSpace(candidate.Kind) {
	case "local_shell":
		intent, ok := parseLocalShellIntent(candidate.Input, nil)
		if !ok {
			return false
		}
		switch strings.TrimSpace(intent.Action) {
		case "create_file",
			"rename_file",
			"run_command",
			"show_system_summary",
			"show_running_processes",
			"show_ip",
			"show_open_ports",
			"show_network_profile",
			"show_network_location",
			"show_vpn_status",
			"show_network_tools_catalog",
			"show_repo_walkthrough":
			return true
		default:
			return false
		}
	case "local_media":
		if intent, ok := parsePlaybackControlIntent(candidate.Input); ok {
			return strings.EqualFold(strings.TrimSpace(intent.Action), "status")
		}
		if _, ok := parsePlaybackContextIntent(candidate.Input); ok {
			return true
		}
		return false
	default:
		return false
	}
}

func shouldRouteLocalShellIntentToCore(input string, intent localShellIntent) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return false
	}
	switch strings.TrimSpace(intent.Action) {
	case "create_file":
		authoringRequest := containsAnyPhrase(lower, []string{
			"write ",
			"populate ",
			"fill ",
			"add ",
			"build ",
			"implement ",
			"design ",
			"style ",
			"template",
			"boilerplate",
			"content",
			"with html",
			"with css",
			"with javascript",
			"including html",
			"containing html",
		})
		frontendContext := containsAnyPhrase(lower, []string{
			"landing page",
			"tailwind",
			"tailwind css",
			"css",
			"javascript",
			"react",
			"vue",
			"svelte",
			"component",
			"feature",
			"codebase",
			"existing codebase",
		})
		if authoringRequest && frontendContext {
			return true
		}
	}
	return false
}

type chatInputEvent struct {
	line string
	err  error
	eof  bool
}

type chatInputReader struct {
	events chan chatInputEvent
}

func newChatInputReader(scanner *bufio.Scanner) *chatInputReader {
	reader := &chatInputReader{
		events: make(chan chatInputEvent, 64),
	}
	go func() {
		if scanner == nil {
			reader.events <- chatInputEvent{eof: true}
			close(reader.events)
			return
		}
		for scanner.Scan() {
			reader.events <- chatInputEvent{line: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			reader.events <- chatInputEvent{err: err}
		} else {
			reader.events <- chatInputEvent{eof: true}
		}
		close(reader.events)
	}()
	return reader
}

func (r *chatInputReader) readBlocking() (string, bool, error) {
	if r == nil {
		return "", true, nil
	}
	event, ok := <-r.events
	if !ok {
		return "", true, nil
	}
	if event.err != nil {
		return "", false, event.err
	}
	if event.eof {
		return "", true, nil
	}
	return event.line, false, nil
}

func (r *chatInputReader) readNonBlocking() (chatInputEvent, bool) {
	if r == nil {
		return chatInputEvent{}, false
	}
	select {
	case event, ok := <-r.events:
		if !ok {
			return chatInputEvent{eof: true}, true
		}
		return event, true
	default:
		return chatInputEvent{}, false
	}
}

func executeConfirmedChatAction(
	c *client.Client,
	input *chatInputReader,
	session string,
	baseMetadata map[string]any,
	lastJobID *int64,
	pendingInputs *[]string,
	candidate *chatActionCandidate,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	localShell bool,
	shellState *localShellState,
	ui *chatUI,
) bool {
	if candidate == nil {
		return false
	}
	if trace := formatLocalAutomationTrace(candidate); trace != "" && strings.TrimSpace(candidate.Kind) != "core_job" {
		emitSystem(ui, trace)
	}

	restoreTrace := func() {}
	if strings.TrimSpace(candidate.Kind) != "core_job" {
		restoreTrace = installLocalExecutionTraceSink(func(line string) {
			emitSystem(ui, line)
		})
		defer restoreTrace()
	}

	switch candidate.Kind {
	case "local_media":
		handled, response := tryHandleLocalMediaCommand(candidate.Input)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local host media inspection/control output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	case "local_browser":
		handled, response := tryHandleLocalBrowserCommand(candidate.Input)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local browser process/tab/console inspection output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	case "local_screen":
		handled, response := tryHandleLocalScreenCommand(candidate.Input)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local screenshot/OCR/vision output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	case "local_shell":
		handled, response := tryHandleLocalShellCommand(candidate.Input, shellState)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local command execution output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	case "local_audio":
		handled, response := tryHandleLocalAudioNotesCommand(candidate.Input)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local audio notes command output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	default:
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	}
}

func promptChatPermissionDecision(input *chatInputReader, ui *chatUI, key, reason, storePath, description string) (bool, error) {
	reason = strings.TrimSpace(reason)
	description = strings.TrimSpace(description)
	if ui != nil {
		emitSystem(ui, "permission required:")
		emitSystem(ui, "  key: "+key)
		if description != "" {
			emitSystem(ui, "  description: "+description)
		}
		if reason != "" {
			emitSystem(ui, "  reason: "+reason)
		}
		if strings.TrimSpace(storePath) != "" {
			emitSystem(ui, "  store: "+storePath)
		}
	} else {
		fmt.Println("permission required:")
		fmt.Println("  key: " + key)
		if description != "" {
			fmt.Println("  description: " + description)
		}
		if reason != "" {
			fmt.Println("  reason: " + reason)
		}
		if strings.TrimSpace(storePath) != "" {
			fmt.Println("  store: " + storePath)
		}
	}
	for {
		fmt.Print("allow and save this permission? [y/n]: ")
		line, eof, err := input.readBlocking()
		if err != nil {
			return false, err
		}
		if eof {
			return false, fmt.Errorf("permission prompt closed before answer for %s", key)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			if ui != nil {
				emitSystem(ui, "please answer y or n")
			} else {
				fmt.Println("please answer y or n")
			}
		}
	}
}

func runDeterministicLocalActionReview(
	c *client.Client,
	input *chatInputReader,
	session string,
	baseMetadata map[string]any,
	lastJobID *int64,
	pendingInputs *[]string,
	candidate *chatActionCandidate,
	actionOutput string,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	localShell bool,
	shellState *localShellState,
	ui *chatUI,
) bool {
	reviewMetadata := deterministicLocalActionReviewMetadata(baseMetadata)
	prompt := buildDeterministicLocalActionReviewPrompt(candidate, actionOutput)
	if ui != nil {
		emitSystem(ui, formatLocalReviewHandoffTrace(candidate, actionOutput))
	}
	return executeChatCoreTurn(
		c,
		input,
		session,
		reviewMetadata,
		lastJobID,
		pendingInputs,
		prompt,
		specialist.RoleReviewVerificationSpecialist,
		interval,
		progress,
		verbose,
		maxChars,
		localShell,
		shellState,
		ui,
	)
}

func deterministicLocalActionReviewMetadata(baseMetadata map[string]any) map[string]any {
	reviewMetadata := cloneMetadata(baseMetadata)
	reviewMetadata["verification_mode"] = "force"
	if value, ok := reviewMetadata["verification_iterations"].(int); !ok || value < 2 {
		reviewMetadata["verification_iterations"] = 2
	}
	reviewMetadata["review_always"] = true
	return reviewMetadata
}

func buildDeterministicLocalActionReviewPrompt(candidate *chatActionCandidate, actionOutput string) string {
	request := "(missing original request)"
	kind := "unknown"
	if candidate != nil {
		if strings.TrimSpace(candidate.Input) != "" {
			request = strings.TrimSpace(candidate.Input)
		}
		if strings.TrimSpace(candidate.Kind) != "" {
			kind = strings.TrimSpace(candidate.Kind)
		}
	}
	output := strings.TrimSpace(actionOutput)
	if output == "" {
		output = "(no local action output captured)"
	}
	lines := []string{
		"Deterministic post-action review step (required):",
		"- You are in the review phase after a local action execution.",
		"- Do not skip this review.",
		"- Compare the original user request against the concrete execution output.",
		"- If the task is incomplete, explicitly start with `INCOMPLETE:` and state the exact next action required to continue from current state.",
		"- If the task is complete, explicitly start with `COMPLETE:` and provide the final answer.",
		"- If a next local shell command is required, include exactly one safe command in backticks.",
		"",
		"Original user request:",
		request,
		"",
		"Local capability kind:",
		kind,
		"",
		"Executed local action output:",
		output,
	}
	return strings.Join(lines, "\n")
}

func isDeterministicLocalActionReviewPrompt(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "Deterministic post-action review step")
}

func isLikelyCoreUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such host") ||
		strings.Contains(text, "context deadline exceeded") ||
		strings.Contains(text, "client.timeout") ||
		strings.Contains(text, "connection reset")
}

func executeChatCoreTurn(
	c *client.Client,
	input *chatInputReader,
	session string,
	baseMetadata map[string]any,
	lastJobID *int64,
	pendingInputs *[]string,
	line string,
	specialistRoleID string,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	localShell bool,
	shellState *localShellState,
	ui *chatUI,
) bool {
	turnMetadata := cloneMetadata(baseMetadata)
	applySpecialistTurnOverrides(turnMetadata, specialistRoleID)
	turnMetadata["session_id"] = session
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		turnMetadata["client_cwd"] = cwd
		turnMetadata["host_env_cwd"] = cwd
	}
	applyHostTemporalMetadata(turnMetadata, time.Now())
	if *lastJobID > 0 {
		turnMetadata["parent_job_id"] = *lastJobID
	}

	job, err := c.Enqueue(context.Background(), line, model.PipelineChat, turnMetadata)
	if err != nil {
		if isDeterministicLocalActionReviewPrompt(line) && isLikelyCoreUnavailableError(err) {
			emitSystem(ui, "core service unavailable; skipped deterministic post-action review after local action")
			return false
		}
		fmt.Fprintf(os.Stderr, "error enqueueing turn: %v\n", err)
		return false
	}
	*lastJobID = job.ID
	emitSystem(ui, fmt.Sprintf("assistant thinking (job %d)...", job.ID))
	emitSystem(ui, queuedTurnHintText())

	details, quit, err := awaitInteractiveTurn(c, input, job.ID, interval, progress, verbose, maxChars, pendingInputs, ui)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error waiting for turn: %v\n", err)
		return false
	}
	if quit {
		return true
	}

	if strings.TrimSpace(details.Job.Result) != "" {
		if localShell {
			updateLocalShellStateFromAssistant(shellState, details.Job.Result)
		}
		emitAssistant(ui, strings.TrimSpace(details.Job.Result))
	}
	if strings.TrimSpace(details.Job.Error) != "" {
		emitAssistantError(ui, strings.TrimSpace(details.Job.Error))
	}
	return false
}

type confirmationDecision string

const (
	confirmationDecisionApprove confirmationDecision = "approve"
	confirmationDecisionReject  confirmationDecision = "reject"
	confirmationDecisionRevise  confirmationDecision = "revise"
)

var confirmationRejectPrefixPattern = regexp.MustCompile(`(?i)^\s*(?:no|n|nope|nah|negative|incorrect|not quite|not exactly|don't|do not|stop|cancel|never mind|nevermind)\b[\s,;:.\-]*(?:but\b[\s,;:.\-]*)?`)
var confirmationApprovePrefixPattern = regexp.MustCompile(`(?i)^\s*(?:yes|y|ok|okay|sure|approve|approved|go ahead|proceed|do it|run it|green light)\b[\s,;:.\-!]*`)
var confirmationRevisionLeadPattern = regexp.MustCompile(`(?i)^(?:but|however|instead|actually|rather|except)\b`)

func interpretConfirmationReply(input string) (confirmationDecision, string) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return confirmationDecisionReject, ""
	}

	lower := strings.ToLower(clean)
	if match := confirmationApprovePrefixPattern.FindString(clean); match != "" {
		remainder := strings.TrimSpace(clean[len(match):])
		if remainder != "" && confirmationRevisionLeadPattern.MatchString(strings.ToLower(remainder)) {
			return confirmationDecisionRevise, clean
		}
		return confirmationDecisionApprove, ""
	}

	negativeRemainder := strings.TrimSpace(confirmationRejectPrefixPattern.ReplaceAllString(clean, ""))
	if negativeRemainder != clean {
		return confirmationDecisionReject, negativeRemainder
	}
	for _, phrase := range []string{
		"no", "n", "nope", "nah", "negative", "incorrect", "not quite", "not exactly", "don't", "do not", "stop", "cancel", "never mind", "nevermind",
	} {
		if lower == phrase {
			return confirmationDecisionReject, ""
		}
	}

	return confirmationDecisionRevise, clean
}

func revisedChatActionCandidate(
	previous *chatActionCandidate,
	feedback string,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
	shellState *localShellState,
) *chatActionCandidate {
	if previous == nil {
		return nil
	}
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return previous
	}

	revised := buildChatActionCandidate(feedback, localMedia, localBrowser, localScreen, localShell, localAudio, shellState)
	if revised != nil && strings.TrimSpace(revised.Input) != "" {
		// Reinterpretation feedback for a core chat turn should not replace the original user request.
		if previous.Kind == "core_job" && revised.Kind == "core_job" {
			return previous
		}
		if revised.Kind != "core_job" || previous.Kind == "core_job" {
			return revised
		}
	}

	combinedInput := strings.TrimSpace(previous.Input + "\n" + feedback)
	if combinedInput == "" {
		return previous
	}
	combined := buildChatActionCandidate(combinedInput, localMedia, localBrowser, localScreen, localShell, localAudio, shellState)
	if combined != nil && strings.TrimSpace(combined.Input) != "" {
		return combined
	}

	return withCandidateSpecialist(&chatActionCandidate{
		Kind:    previous.Kind,
		Input:   combinedInput,
		Summary: previous.Summary,
	})
}

func buildActionInterpretationPrompt(
	candidate *chatActionCandidate,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
) string {
	capabilities := enabledAutomationCapabilities(localMedia, localBrowser, localScreen, localShell, localAudio)
	if candidate == nil {
		lines := []string{
			"Interpret the user's request before execution.",
			"Do not execute anything yet.",
			"Use recent conversation from this chat session to preserve context.",
		}
		if len(capabilities) > 0 {
			lines = append(lines, "", "Available local capabilities:")
			lines = append(lines, capabilities...)
		}
		lines = append(lines,
			"",
			"Safety constraints:",
			"- Prefer non-sudo commands first. If elevated access is required, ask for sudo and explain why.",
			"- Do not remove/delete files.",
			"",
			"Respond in this structure:",
			"Interpretation: <your best understanding of user intent and goal>",
			"Questions: <early clarifying questions, or \"none\">",
			"Confirmation: <single concise confirmation request>",
		)
		return strings.Join(lines, "\n")
	}

	lines := []string{
		"Interpret the user's request before execution.",
		"Do not execute anything yet.",
		"Use recent conversation from this chat session to preserve context.",
		"",
		"Original request:",
		candidate.Input,
		"",
		"Preliminary routing guess:",
		candidate.Summary,
	}
	role := candidateSpecialistRole(candidate)
	lines = append(lines, "", "Assigned specialist:")
	lines = append(lines, specialist.DetailLines(role)...)
	if len(capabilities) > 0 {
		lines = append(lines, "", "Available local capabilities:")
		lines = append(lines, capabilities...)
	}
	lines = append(lines,
		"",
		"Safety constraints:",
		"- Prefer non-sudo commands first. If elevated access is required, ask for sudo and explain why.",
		"- Do not remove/delete files.",
		"",
		"Respond in this structure:",
		"Interpretation: <your best understanding of user intent and goal>",
		"Questions: <early clarifying questions, or \"none\">",
		"Confirmation: <single concise confirmation request>",
	)
	return strings.Join(lines, "\n")
}

func buildActionReinterpretationPrompt(
	candidate *chatActionCandidate,
	feedback string,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
) string {
	capabilities := enabledAutomationCapabilities(localMedia, localBrowser, localScreen, localShell, localAudio)
	if candidate == nil {
		lines := []string{
			"Please reinterpret the user's request and ask any required clarifying questions before suggesting an action.",
			"Do not execute anything yet.",
			"Use recent conversation from this chat session to preserve context.",
		}
		if len(capabilities) > 0 {
			lines = append(lines, "", "Available local capabilities:")
			lines = append(lines, capabilities...)
		}
		lines = append(lines,
			"",
			"Safety constraints:",
			"- Prefer non-sudo commands first. If elevated access is required, ask for sudo and explain why.",
			"- Do not remove/delete files.",
		)
		return strings.Join(lines, "\n")
	}
	cleanFeedback := strings.TrimSpace(feedback)
	lines := []string{
		"The user rejected an earlier interpretation for a local action request.",
		"Do not execute anything yet. Re-interpret intent and ask clarifying questions before action.",
		"Use recent conversation from this chat session to preserve context.",
		"",
		"Original request:",
		candidate.Input,
		"",
		"Rejected interpretation:",
		candidate.Summary,
	}
	role := candidateSpecialistRole(candidate)
	lines = append(lines, "", "Assigned specialist:")
	lines = append(lines, specialist.DetailLines(role)...)
	if len(capabilities) > 0 {
		lines = append(lines, "", "Available local capabilities:")
		lines = append(lines, capabilities...)
	}
	lines = append(lines,
		"",
		"Safety constraints:",
		"- Prefer non-sudo commands first. If elevated access is required, ask for sudo and explain why.",
		"- Do not remove/delete files.",
	)
	if cleanFeedback != "" {
		lines = append(lines, "", "User feedback on what was wrong:", cleanFeedback)
	} else {
		lines = append(lines, "", "User feedback on what was wrong:", "(none provided; user said no without details)")
		lines = append(lines, "Ask 2-4 targeted clarifying questions to bridge the gap before execution.")
	}
	lines = append(lines,
		"",
		"Respond in this structure:",
		"Interpretation: <your best revised interpretation>",
		"Questions: <early clarifying questions, or \"none\">",
		"Confirmation: <single prompt asking the user to confirm/correct before execution>",
	)
	return strings.Join(lines, "\n")
}

func enabledAutomationCapabilities(
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
) []string {
	lines := make([]string, 0, 5)
	if localShell {
		role := specialist.ForLocalCapability("local_shell")
		lines = append(lines, "- local_shell: run local shell commands, inspect files, build code, and run tests in the current directory. (specialist: "+role.ID+")")
	}
	if localMedia {
		role := specialist.ForLocalCapability("local_media")
		lines = append(lines, "- local_media: inspect player status/metadata and control playback when explicitly requested. (specialist: "+role.ID+")")
	}
	if localBrowser {
		role := specialist.ForLocalCapability("local_browser")
		lines = append(lines, "- local_browser: inspect local browser processes, tabs, and console activity when available. (specialist: "+role.ID+")")
	}
	if localScreen {
		role := specialist.ForLocalCapability("local_screen")
		lines = append(lines, "- local_screen: capture local screenshots and extract OCR/vision summaries. (specialist: "+role.ID+")")
	}
	if localAudio {
		role := specialist.ForLocalCapability("local_audio")
		lines = append(lines, "- local_audio: manage local audio notes capture, status, and search. (specialist: "+role.ID+")")
	}
	return lines
}

func describePlaybackControlIntent(intent playbackControlIntent) string {
	switch strings.ToLower(strings.TrimSpace(intent.Action)) {
	case "play":
		return "resume/play the active local media player (VLC via MPRIS/playerctl)"
	case "pause":
		return "pause the active local media player (VLC via MPRIS/playerctl)"
	case "play-pause":
		return "toggle play/pause on the active local media player"
	case "status":
		return "check what is currently playing on the active local media player (status/title/path)"
	default:
		return "control local media playback"
	}
}

func describeNextEpisodeIntent(intent nextEpisodeIntent) string {
	if strings.TrimSpace(intent.ShowHint) != "" {
		return fmt.Sprintf("play the next episode near the current media file, preferring show `%s`", intent.ShowHint)
	}
	return "play the next episode near the current media file"
}

func describePlaybackContextIntent(intent playbackContextIntent) string {
	if strings.TrimSpace(intent.Query) != "" {
		return fmt.Sprintf("inspect current playback subtitles around now and focus on `%s`", intent.Query)
	}
	return "inspect current playback subtitles around the current timestamp"
}

func describeBrowserScanIntent(intent browserScanIntent) string {
	if intent.EmailWatch {
		return "inspect local email tabs and report what is newly visible in inbox views"
	}
	if intent.WithConsole {
		return fmt.Sprintf("scan local browser tabs and read JavaScript console activity for %ds", intent.Seconds)
	}
	return "scan local browser processes and open tabs"
}

func describeScreenReadIntent(intent screenReadIntent) string {
	mode := "OCR text"
	if intent.WithOCR && intent.WithVision {
		mode = "OCR text + vision summary"
	} else if intent.WithVision {
		mode = "vision summary"
	}
	if strings.TrimSpace(intent.Prompt) != "" {
		return fmt.Sprintf("capture your screen and run %s focused on `%s`", mode, intent.Prompt)
	}
	return fmt.Sprintf("capture your screen and run %s", mode)
}

func describeLocalShellIntent(intent localShellIntent) string {
	switch intent.Action {
	case "create_file":
		target := strings.TrimSpace(intent.Target)
		if target == "" {
			target = "test"
		}
		return fmt.Sprintf("create file `%s` in the current directory", target)
	case "rename_file":
		return fmt.Sprintf("rename `%s` to `%s` in the current directory", strings.TrimSpace(intent.Source), strings.TrimSpace(intent.Target))
	case "run_command":
		return fmt.Sprintf("run local command `%s` in the current directory", strings.TrimSpace(intent.Command))
	case "show_system_summary":
		return "inspect local system summary (user, OS, shell, cwd, time)"
	case "show_running_processes":
		return "inspect currently running processes using multiple local strategies (top/ps/services)"
	case "show_ip":
		return "inspect local/public IP information"
	case "show_open_ports":
		return "inspect open local ports"
	case "show_open_ports_detailed":
		return "inspect open local ports with process details (may require sudo)"
	case "show_network_profile":
		return "inspect local network profile (IP/location/VPN/tools)"
	case "show_network_location":
		return "estimate location from current public IP"
	case "show_vpn_status":
		return "inspect VPN interface/process/connection status"
	case "show_network_tools_catalog":
		return "list available local/web network tools"
	case "show_repo_walkthrough":
		return "walk through git repository changes in chronological order to resume project context"
	case "install_network_tools":
		return "run the local network-tools install helper script"
	default:
		return "execute a local shell action"
	}
}

func describeLocalAudioNotesIntent(intent localAudioNotesIntent) string {
	switch intent.Action {
	case "start":
		return "start audio notes capture for this session"
	case "stop":
		return "stop audio notes capture"
	case "status":
		return "check current audio notes status"
	case "search":
		return fmt.Sprintf("search audio notes for `%s`", strings.TrimSpace(intent.Query))
	default:
		return "run an audio notes action"
	}
}

func parseQueuedTurnInput(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	trimmedTabs := strings.TrimLeft(raw, "\t")
	if len(trimmedTabs) == len(raw) {
		return "", false
	}
	message := strings.TrimSpace(trimmedTabs)
	if message == "" {
		return "", false
	}
	return message, true
}

func captureQueuedTurnInput(
	input *chatInputReader,
	pendingInputs *[]string,
	ui *chatUI,
) (bool, error) {
	if input == nil {
		return false, nil
	}
	if pendingInputs == nil {
		return false, nil
	}

	for {
		event, ok := input.readNonBlocking()
		if !ok {
			return false, nil
		}
		if event.err != nil {
			return false, event.err
		}
		if event.eof {
			return true, nil
		}

		if queuedMessage, queueOK := parseQueuedTurnInput(event.line); queueOK {
			*pendingInputs = append(*pendingInputs, queuedMessage)
			emitSystem(ui, fmt.Sprintf("TAB queue: queued next turn (#%d): %s", len(*pendingInputs), truncateForWatch(queuedMessage, 140)))
			continue
		}

		command, _ := parseSlashCommand(strings.TrimSpace(event.line))
		if command == "exit" || command == "quit" {
			return true, nil
		}

		if strings.TrimSpace(event.line) != "" {
			emitSystem(ui, "turn in progress: TAB + message + Enter queues a follow-up for the next turn")
		}
	}
}

func awaitInteractiveTurn(
	c *client.Client,
	input *chatInputReader,
	jobID int64,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	pendingInputs *[]string,
	ui *chatUI,
) (model.JobDetails, bool, error) {
	lastStatus := ""
	lastStepStatus := map[int64]string{}
	lastStepDetails := map[int64]string{}
	seenContextIDs := map[int64]struct{}{}

	for {
		details, err := c.Show(context.Background(), jobID)
		if err != nil {
			return model.JobDetails{}, false, err
		}

		status := details.Job.Status
		if status != lastStatus {
			emitSystem(ui, fmt.Sprintf("job %d status=%s", jobID, status))
			lastStatus = status
		}

		printed := false
		if progress || verbose {
			printed = printStepStatusUpdatesWithUI(details.Steps, lastStepStatus, ui) || printed
		}
		if verbose {
			printed = printStepDetailUpdates(details.Steps, lastStepDetails, maxChars) || printed
		}
		if progress || verbose {
			printed = printContextUpdatesWithUI(details.Contexts, seenContextIDs, progress, verbose, maxChars, ui) || printed
		}
		if printed {
			emitRule(ui)
		}

		if status != model.JobStatusWaiting {
			quit, err := captureQueuedTurnInput(input, pendingInputs, ui)
			if err != nil {
				return model.JobDetails{}, false, err
			}
			if quit {
				return details, true, nil
			}
		}

		if status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusCanceled {
			return details, false, nil
		}

		if status == model.JobStatusWaiting {
			question := latestContextValue(details.Contexts, "input_question")
			if strings.TrimSpace(question) != "" {
				emitNeedsInput(ui, question)
			} else {
				emitNeedsInput(ui, "assistant needs input to continue")
			}
			emitSystem(ui, "reply normally to submit feedback, or use /interrupt, /replan, /cancel, /exit")

			for {
				fmt.Print(feedbackPrompt(ui))
				rawInput, eof, err := input.readBlocking()
				if err != nil {
					return model.JobDetails{}, false, err
				}
				if eof {
					return details, true, nil
				}

				feedbackInput := strings.TrimSpace(rawInput)
				if feedbackInput == "" {
					continue
				}

				if strings.HasPrefix(feedbackInput, "/") {
					command, body := parseSlashCommand(feedbackInput)
					switch command {
					case "exit", "quit":
						return details, true, nil
					case "help":
						printInteractiveInputHelp()
						continue
					case "cancel":
						job, err := c.Cancel(context.Background(), jobID, body)
						if err != nil {
							fmt.Fprintf(os.Stderr, "error canceling job: %v\n", err)
							continue
						}
						emitSystem(ui, fmt.Sprintf("canceled job %d status=%s", job.ID, job.Status))
					case "interrupt":
						if strings.TrimSpace(body) == "" {
							emitSystem(ui, "usage: /interrupt <context>")
							continue
						}
						job, err := c.Interrupt(context.Background(), jobID, body)
						if err != nil {
							fmt.Fprintf(os.Stderr, "error interrupting job: %v\n", err)
							continue
						}
						emitSystem(ui, fmt.Sprintf("interrupt submitted for job %d status=%s", job.ID, job.Status))
					case "replan":
						if strings.TrimSpace(body) == "" {
							emitSystem(ui, "usage: /replan <context>")
							continue
						}
						job, err := c.Replan(context.Background(), jobID, body)
						if err != nil {
							fmt.Fprintf(os.Stderr, "error replanning job: %v\n", err)
							continue
						}
						emitSystem(ui, fmt.Sprintf("replan submitted for job %d status=%s", job.ID, job.Status))
					default:
						emitSystem(ui, "unknown command in feedback mode. use /help")
						continue
					}
					break
				}

				job, err := c.SubmitFeedback(context.Background(), jobID, feedbackInput)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error submitting feedback: %v\n", err)
					continue
				}
				emitSystem(ui, fmt.Sprintf("feedback submitted for job %d status=%s", job.ID, job.Status))
				break
			}
		}

		time.Sleep(interval)
	}
}

func handleChatReplCommand(line string, sessionID *string, lastJobID *int64, ui *chatUI) (bool, bool) {
	command, body := parseSlashCommand(line)
	switch command {
	case "exit", "quit":
		return true, true
	case "help":
		printInteractiveChatHelp()
		return true, false
	case "session":
		if strings.TrimSpace(body) == "" {
			emitSystem(ui, "session="+strings.TrimSpace(*sessionID))
			return true, false
		}
		next := strings.TrimSpace(body)
		if next == "" {
			emitSystem(ui, "usage: /session <id>")
			return true, false
		}
		*sessionID = next
		*lastJobID = 0
		emitSystem(ui, "switched to session="+*sessionID)
		return true, false
	case "new":
		*sessionID = fmt.Sprintf("chat-%d", time.Now().Unix())
		*lastJobID = 0
		emitSystem(ui, "started new session="+*sessionID)
		return true, false
	case "last":
		if *lastJobID > 0 {
			emitSystem(ui, fmt.Sprintf("last-job=%d", *lastJobID))
		} else {
			emitSystem(ui, "no prior turns in this chat session")
		}
		return true, false
	default:
		emitSystem(ui, "unknown command. type /help")
		return true, false
	}
}

func parseSlashCommand(line string) (string, string) {
	raw := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	if raw == "" {
		return "", ""
	}
	parts := strings.SplitN(raw, " ", 2)
	command := strings.ToLower(strings.TrimSpace(parts[0]))
	body := ""
	if len(parts) > 1 {
		body = strings.TrimSpace(parts[1])
	}
	return command, body
}

func printInteractiveChatHelp() {
	fmt.Println("interactive commands:")
	fmt.Println("  /help              show this help")
	fmt.Println("  /session           show current session id")
	fmt.Println("  /session <id>      switch to a specific session id")
	fmt.Println("  /new               start a fresh session id")
	fmt.Println("  /last              show most recent job id")
	fmt.Println("  /exit              quit interactive mode")
	fmt.Println("  progress note      live stage/event updates are shown by default (disable with --progress=false)")
	fmt.Println("  phase note         stage lines include phase=planning|execution|review")
	fmt.Println("  specialist note    each routed action is assigned to a specialist role (e.g., browser/media/shell)")
	fmt.Println("  confirm note       chat runs an AI interpretation pass for local automation actions and waits for `yes` before execution (disable with --confirm-actions=false)")
	fmt.Println("  queue note         while assistant is running, type TAB + message + Enter to queue a follow-up for the next turn")
	fmt.Println("  note               invasive local actions ask one-time permission (managed by `omni permissions ...`)")
	fmt.Println("  natural command    routing is capability-based (examples below are not exact trigger phrases)")
	fmt.Println("  natural command    'play the next episode ...' controls local VLC when --local-media is on")
	fmt.Println("  natural command    'show my browser tabs' scans local browser processes/tabs when --local-browser is on")
	fmt.Println("  natural command    'what's on my screen' captures and reads the local screen when --local-screen is on")
	fmt.Println("  natural command    'create a file named notes.txt' executes locally when --local-shell is on")
	fmt.Println("  natural command    'what is my ip' or 'what ports are open' runs local network inspection when --local-shell is on")
	fmt.Println("  local-shell note   file-edit commands include git change summaries/diff snippets when inside a git repo")
	fmt.Println("  natural command    'determine my location' / 'am I on VPN' / 'show network tools catalog' runs advanced network intelligence when --local-shell is on")
	fmt.Println("  natural command    'take notes during this call' starts local audio notes when --local-audio is on")
	fmt.Println("  profile note       start with --profile architect for stricter plan/verify/watch defaults")
}

func printInteractiveInputHelp() {
	fmt.Println("feedback mode commands:")
	fmt.Println("  /interrupt <text>  inject context into the active job")
	fmt.Println("  /replan <text>     restart the job from plan with new context")
	fmt.Println("  /cancel [reason]   stop the active job")
	fmt.Println("  /exit              quit interactive mode")
}

func formatLocalAutomationResponse(sourceLine, response string) string {
	text := strings.TrimSpace(response)
	if text == "" {
		return text
	}
	if hasSourceSection(text) {
		return text
	}
	sourceLine = strings.TrimSpace(sourceLine)
	if sourceLine == "" {
		return text
	}
	return text + "\n\nSources:\n- " + sourceLine
}

func hasSourceSection(text string) bool {
	for _, line := range strings.Split(strings.ToLower(text), "\n") {
		clean := strings.TrimSpace(line)
		if clean == "source:" || clean == "sources:" || strings.HasPrefix(clean, "source:") || strings.HasPrefix(clean, "sources:") {
			return true
		}
	}
	return false
}

func cloneMetadata(metadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func runList(c *client.Client, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	status := fs.String("status", "", "filter by status")
	limit := fs.Int("limit", 20, "max jobs")
	offset := fs.Int("offset", 0, "offset")
	_ = fs.Parse(args)

	jobs, err := c.List(context.Background(), *status, *limit, *offset)
	if err != nil {
		die(err.Error())
	}

	if len(jobs) == 0 {
		fmt.Println("no jobs")
		return
	}

	for _, job := range jobs {
		fmt.Printf("#%d [%s] pipeline=%s created=%s\n", job.ID, job.Status, job.Pipeline, job.CreatedAt.Format(time.RFC3339))
	}
}

func runShow(c *client.Client, args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	inspect := fs.Bool("inspect", false, "show v3 inspection payload")
	_ = fs.Parse(args)

	if len(fs.Args()) < 1 {
		die("show requires a job id")
	}

	id, err := strconv.ParseInt(fs.Args()[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	var payload []byte
	if *inspect {
		inspection, err := c.Inspect(context.Background(), id)
		if err != nil {
			die(err.Error())
		}
		payload, err = json.MarshalIndent(inspection, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
		return
	}

	details, err := c.Show(context.Background(), id)
	if err != nil {
		die(err.Error())
	}
	payload, err = json.MarshalIndent(details, "", "  ")
	if err != nil {
		die(err.Error())
	}
	fmt.Println(string(payload))
}

func runWatch(c *client.Client, args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	interval := fs.Duration("interval", 2*time.Second, "poll interval")
	progress := fs.Bool("progress", true, "print live stage/event updates")
	verbose := fs.Bool("verbose", false, "print full debug trace (including LLM prompts and full context dumps)")
	maxChars := fs.Int("max-chars", 1200, "max characters shown per output/context entry (0 disables truncation)")
	_ = fs.Parse(args)

	if len(fs.Args()) < 1 {
		die("watch requires a job id")
	}
	id, err := strconv.ParseInt(fs.Args()[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	lastStatus := ""
	lastStepStatus := map[int64]string{}
	lastStepDetails := map[int64]string{}
	seenContextIDs := map[int64]struct{}{}
	for {
		details, err := c.Show(context.Background(), id)
		if err != nil {
			die(err.Error())
		}

		status := details.Job.Status
		if status != lastStatus {
			fmt.Printf("job %d status=%s\n", id, status)
			lastStatus = status
		}

		printed := false
		if *progress || *verbose {
			printed = printStepStatusUpdates(details.Steps, lastStepStatus) || printed
		}
		if *verbose {
			printed = printStepDetailUpdates(details.Steps, lastStepDetails, *maxChars) || printed
		}
		if *progress || *verbose {
			printed = printContextUpdates(details.Contexts, seenContextIDs, *progress, *verbose, *maxChars) || printed
		}
		if printed {
			fmt.Println("---")
		}

		if status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusCanceled {
			if details.Job.Result != "" {
				fmt.Printf("result:\n%s\n", details.Job.Result)
			}
			if details.Job.Error != "" {
				fmt.Printf("error: %s\n", details.Job.Error)
			}
			return
		}
		if status == model.JobStatusWaiting {
			question := latestContextValue(details.Contexts, "input_question")
			if strings.TrimSpace(question) != "" {
				fmt.Printf("input requested: %s\n", question)
			} else {
				fmt.Println("input requested: core needs additional information before continuing")
			}
			fmt.Printf("provide feedback with: omni feedback %d \"...\"\n", id)
			fmt.Printf("inject extra context with: omni interrupt %d \"...\"\n", id)
			fmt.Printf("replan from scratch with: omni replan %d \"...\"\n", id)
			fmt.Printf("cancel immediately with: omni cancel %d \"...\"\n", id)
			return
		}

		time.Sleep(*interval)
	}
}

func runRemember(c *client.Client, args []string) {
	fs := flag.NewFlagSet("remember", flag.ExitOnError)
	source := fs.String("source", "manual", "memory source label")
	kind := fs.String("kind", model.MemoryKindEpisodic, "memory kind: episodic|procedural|instruction|preference|reference")
	tags := fs.String("tags", "", "comma-separated tags")
	_ = fs.Parse(args)

	content := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if content == "" {
		die("remember requires content")
	}

	tagList := splitTags(*tags)
	chunk, err := c.AddMemory(context.Background(), *source, *kind, content, tagList)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("stored memory #%d source=%s kind=%s\n", chunk.ID, chunk.Source, chunk.Kind)
}

func runMemoryCandidates(c *client.Client, args []string) {
	if len(args) == 0 {
		die("memory-candidates requires a subcommand: list|promote|reject")
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		fs := flag.NewFlagSet("memory-candidates list", flag.ExitOnError)
		jobID := fs.Int64("job-id", 0, "optional job id filter")
		status := fs.String("status", "", "optional candidate status filter")
		limit := fs.Int("limit", 50, "max candidates to return")
		_ = fs.Parse(args[1:])

		items, err := c.ListMemoryCandidates(context.Background(), *jobID, *status, *limit)
		if err != nil {
			die(err.Error())
		}
		payload, err := json.MarshalIndent(map[string]any{"memory_candidates": items}, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
	case "promote":
		fs := flag.NewFlagSet("memory-candidates promote", flag.ExitOnError)
		tier := fs.String("tier", model.MemoryCandidateStatusApproved, "target tier: approved|durable")
		_ = fs.Parse(args[1:])
		if len(fs.Args()) < 1 {
			die("memory-candidates promote requires a candidate id")
		}
		id, err := strconv.ParseInt(fs.Args()[0], 10, 64)
		if err != nil || id <= 0 {
			die("invalid candidate id")
		}
		result, err := c.PromoteMemoryCandidate(context.Background(), id, *tier)
		if err != nil {
			die(err.Error())
		}
		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
	case "reject":
		if len(args) < 2 {
			die("memory-candidates reject requires a candidate id")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || id <= 0 {
			die("invalid candidate id")
		}
		item, err := c.RejectMemoryCandidate(context.Background(), id)
		if err != nil {
			die(err.Error())
		}
		payload, err := json.MarshalIndent(map[string]any{"memory_candidate": item}, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
	default:
		die("memory-candidates requires a subcommand: list|promote|reject")
	}
}

func runIngest(c *client.Client, args []string) {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	source := fs.String("source", "file", "memory source prefix")
	kind := fs.String("kind", model.MemoryKindReference, "memory kind: episodic|procedural|instruction|preference|reference")
	tags := fs.String("tags", "", "comma-separated tags")
	chunkSize := fs.Int("chunk-size", 1800, "chunk size in characters")
	overlap := fs.Int("overlap", 220, "chunk overlap in characters")
	_ = fs.Parse(args)

	paths := fs.Args()
	if len(paths) == 0 {
		die("ingest requires one or more file paths")
	}

	baseTags := splitTags(*tags)
	totalChunks := 0

	for _, path := range paths {
		parsed, err := ingest.ParseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: %s: %v\n", path, err)
			continue
		}

		chunks := ingest.ChunkText(parsed.Content, *chunkSize, *overlap)
		if len(chunks) == 0 {
			fmt.Fprintf(os.Stderr, "warn: %s: no ingestible text\n", path)
			continue
		}

		autoTags := ingest.InferTagsFromPath(path, parsed.Format)
		allTags := mergeTags(baseTags, autoTags)
		base := filepath.Base(path)

		storedForFile := 0
		for i, chunk := range chunks {
			chunkSource := fmt.Sprintf("%s:%s#%d", strings.TrimSpace(*source), base, i+1)
			_, err := c.AddMemory(context.Background(), chunkSource, *kind, chunk, allTags)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: %s chunk %d: %v\n", path, i+1, err)
				continue
			}
			storedForFile++
			totalChunks++
		}

		fmt.Printf("ingested %s format=%s chunks=%d\n", path, parsed.Format, storedForFile)
	}

	if totalChunks == 0 {
		die("no chunks were ingested")
	}

	fmt.Printf("ingest complete: %d chunks stored\n", totalChunks)
}

func runFeedback(c *client.Client, args []string) {
	if len(args) < 2 {
		die("feedback requires <job-id> and feedback text")
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		die("invalid job id")
	}

	feedback := strings.TrimSpace(strings.Join(args[1:], " "))
	if feedback == "" {
		die("feedback text is required")
	}

	job, err := c.SubmitFeedback(context.Background(), id, feedback)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("submitted feedback for job %d, new status=%s\n", job.ID, job.Status)
}

func runMediaIndex(c *client.Client, args []string) {
	fs := flag.NewFlagSet("media-index", flag.ExitOnError)
	root := fs.String("root", ".", "media library root directory")
	source := fs.String("source", "media", "memory source prefix")
	kind := fs.String("kind", model.MemoryKindReference, "memory kind")
	tags := fs.String("tags", "", "comma-separated base tags")
	episodeLimit := fs.Int("episode-limit", 0, "max episodes to index (0 = all)")
	maxLinesPerChunk := fs.Int("lines-per-chunk", 45, "subtitle lines per memory chunk")
	includeNoSubs := fs.Bool("include-no-subs", false, "store metadata chunk even when no subtitle file is found")
	dryRun := fs.Bool("dry-run", false, "scan and summarize only; do not store memory")
	_ = fs.Parse(args)

	rootPath := strings.TrimSpace(*root)
	if rootPath == "" {
		die("--root is required")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		die(err.Error())
	}

	episodes, err := media.DiscoverEpisodes(absRoot, *episodeLimit)
	if err != nil {
		die(err.Error())
	}
	if len(episodes) == 0 {
		fmt.Println("no media episodes found")
		return
	}

	baseTags := splitTags(*tags)
	totalEpisodes := 0
	episodesWithSubs := 0
	episodesWithoutSubs := 0
	totalChunks := 0
	totalStored := 0
	totalLines := 0
	errorsCount := 0

	for _, ep := range episodes {
		totalEpisodes++
		episodeTags := mergeTags(baseTags, media.EpisodeTags(ep), ingest.InferTagsFromPath(ep.VideoPath, "subtitle"))
		metaContent := buildEpisodeMetadataContent(ep)
		slug := sanitizeMemorySourceToken(fmt.Sprintf("%s-s%02de%02d", ep.ShowSlug, ep.Season, ep.Episode))
		if slug == "" {
			slug = sanitizeMemorySourceToken(strings.TrimSuffix(filepath.Base(ep.VideoPath), filepath.Ext(ep.VideoPath)))
		}
		if slug == "" {
			slug = fmt.Sprintf("episode-%d", totalEpisodes)
		}

		if ep.SubtitlePath == "" {
			episodesWithoutSubs++
			if *includeNoSubs {
				if *dryRun {
					fmt.Printf("dry-run metadata only: %s\n", ep.VideoPath)
				} else {
					sourceLabel := fmt.Sprintf("%s:%s#meta", strings.TrimSpace(*source), slug)
					if _, err := c.AddMemory(context.Background(), sourceLabel, *kind, metaContent, episodeTags); err != nil {
						errorsCount++
						fmt.Fprintf(os.Stderr, "warn: metadata store failed for %s: %v\n", ep.VideoPath, err)
					} else {
						totalStored++
					}
				}
			}
			continue
		}

		lines, err := media.ParseSubtitleLines(ep.SubtitlePath)
		if err != nil {
			errorsCount++
			fmt.Fprintf(os.Stderr, "warn: subtitle parse failed for %s: %v\n", ep.SubtitlePath, err)
			continue
		}
		if len(lines) == 0 {
			episodesWithoutSubs++
			continue
		}

		episodesWithSubs++
		totalLines += len(lines)
		chunks := media.ChunkSubtitleLines(lines, *maxLinesPerChunk)
		totalChunks += len(chunks)

		if *dryRun {
			fmt.Printf("dry-run %s lines=%d chunks=%d subtitle=%s\n", ep.VideoPath, len(lines), len(chunks), ep.SubtitlePath)
			continue
		}

		sourceMeta := fmt.Sprintf("%s:%s#meta", strings.TrimSpace(*source), slug)
		if _, err := c.AddMemory(context.Background(), sourceMeta, *kind, metaContent, episodeTags); err != nil {
			errorsCount++
			fmt.Fprintf(os.Stderr, "warn: metadata store failed for %s: %v\n", ep.VideoPath, err)
		} else {
			totalStored++
		}

		for i, chunk := range chunks {
			payload := buildSubtitleChunkContent(ep, chunk)
			sourceChunk := fmt.Sprintf("%s:%s#%03d", strings.TrimSpace(*source), slug, i+1)
			if _, err := c.AddMemory(context.Background(), sourceChunk, *kind, payload, episodeTags); err != nil {
				errorsCount++
				fmt.Fprintf(os.Stderr, "warn: chunk store failed for %s chunk=%d: %v\n", ep.SubtitlePath, i+1, err)
				continue
			}
			totalStored++
		}
	}

	fmt.Printf("media index complete root=%s episodes=%d with_subtitles=%d without_subtitles=%d subtitle_lines=%d chunks=%d stored=%d errors=%d dry_run=%v\n",
		absRoot, totalEpisodes, episodesWithSubs, episodesWithoutSubs, totalLines, totalChunks, totalStored, errorsCount, *dryRun)
}

func runMediaSearch(args []string) {
	fs := flag.NewFlagSet("media-search", flag.ExitOnError)
	root := fs.String("root", ".", "media library root directory")
	contextWindow := fs.Int("context", 2, "subtitle lines of context before/after each match")
	limit := fs.Int("limit", 20, "max matches to print")
	_ = fs.Parse(args)

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		die("media-search requires a query text")
	}

	rootPath := strings.TrimSpace(*root)
	if rootPath == "" {
		die("--root is required")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		die(err.Error())
	}

	matches, err := media.SearchSubtitleLines(absRoot, query, *contextWindow, *limit)
	if err != nil {
		die(err.Error())
	}
	if len(matches) == 0 {
		fmt.Printf("no subtitle matches for %q under %s\n", query, absRoot)
		return
	}

	for i, match := range matches {
		fmt.Printf("[%d] file=%s", i+1, match.SubtitlePath)
		if match.Show != "" {
			fmt.Printf(" show=%s", match.Show)
		}
		if match.Season > 0 && match.Episode > 0 {
			fmt.Printf(" episode=S%02dE%02d", match.Season, match.Episode)
		}
		fmt.Println("")
		for _, before := range match.Before {
			fmt.Printf("  - [%s] %s\n", safeTimestamp(before.Start), before.Text)
		}
		fmt.Printf("  > [%s] %s\n", safeTimestamp(match.Line.Start), match.Line.Text)
		for _, after := range match.After {
			fmt.Printf("  + [%s] %s\n", safeTimestamp(after.Start), after.Text)
		}
	}
}

func buildEpisodeMetadataContent(ep media.Episode) string {
	lines := []string{
		"Episode metadata",
		"video=" + ep.VideoPath,
		"subtitle=" + safeValue(ep.SubtitlePath, "(none)"),
		"show=" + safeValue(ep.Show, "(unknown)"),
		"show_slug=" + safeValue(ep.ShowSlug, "(unknown)"),
	}
	if ep.Season > 0 {
		lines = append(lines, fmt.Sprintf("season=%d", ep.Season))
	}
	if ep.Episode > 0 {
		lines = append(lines, fmt.Sprintf("episode=%d", ep.Episode))
	}
	return strings.Join(lines, "\n")
}

func buildSubtitleChunkContent(ep media.Episode, chunk media.SubtitleChunk) string {
	lines := []string{
		"Episode subtitle chunk",
		"video=" + ep.VideoPath,
		"subtitle=" + ep.SubtitlePath,
		"show=" + safeValue(ep.Show, "(unknown)"),
		"show_slug=" + safeValue(ep.ShowSlug, "(unknown)"),
	}
	if ep.Season > 0 {
		lines = append(lines, fmt.Sprintf("season=%d", ep.Season))
	}
	if ep.Episode > 0 {
		lines = append(lines, fmt.Sprintf("episode=%d", ep.Episode))
	}
	lines = append(lines,
		fmt.Sprintf("line_range=%d-%d", chunk.FromLine, chunk.ToLine),
		"start="+safeTimestamp(chunk.Start),
		"end="+safeTimestamp(chunk.End),
		"content:",
		chunk.Text,
	)
	return strings.Join(lines, "\n")
}

func sanitizeMemorySourceToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	re := regexp.MustCompile(`[^a-z0-9]+`)
	value = re.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) > 80 {
		value = strings.Trim(value[:80], "-")
	}
	return value
}

func safeTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

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

func splitTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.ToLower(strings.TrimSpace(part))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func applyExecutionProfile(
	rawArgs []string,
	profile string,
	webMode *string,
	workspaceMode *string,
	allowMissingTools *bool,
	reasoningLevel *string,
	autonomyMode *string,
	approvalMode *string,
	verificationMode *string,
	verificationIterations *int,
	verbose *bool,
	maxChars *int,
	localShell *bool,
) (bool, error) {
	selected := strings.ToLower(strings.TrimSpace(profile))
	if selected == "" || selected == "default" {
		return false, nil
	}
	if selected != "architect" {
		return false, fmt.Errorf("invalid --profile value %q (use default|architect)", profile)
	}

	if webMode != nil && !flagProvided(rawArgs, "web") {
		*webMode = "auto"
	}
	if workspaceMode != nil && !flagProvided(rawArgs, "workspace") {
		*workspaceMode = "on"
	}
	if allowMissingTools != nil && !flagProvided(rawArgs, "allow-missing-tools") {
		*allowMissingTools = true
	}
	if reasoningLevel != nil && !flagProvided(rawArgs, "reasoning") {
		*reasoningLevel = "deep"
	}
	if autonomyMode != nil && !flagProvided(rawArgs, "autonomy") {
		*autonomyMode = "on"
	}
	if approvalMode != nil && !flagProvided(rawArgs, "approval") {
		*approvalMode = "on"
	}
	if verificationMode != nil && !flagProvided(rawArgs, "verify") {
		*verificationMode = "on"
	}
	if verificationIterations != nil && !flagProvided(rawArgs, "verify-iterations") {
		*verificationIterations = 3
	}
	if verbose != nil && !flagProvided(rawArgs, "verbose") {
		*verbose = true
	}
	if maxChars != nil && !flagProvided(rawArgs, "max-chars") {
		*maxChars = 2200
	}
	if localShell != nil && !flagProvided(rawArgs, "local-shell") {
		*localShell = true
	}
	return true, nil
}

func flagProvided(args []string, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	long := "--" + name
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == long || strings.HasPrefix(trimmed, long+"=") {
			return true
		}
	}
	return false
}

func usage() {
	fmt.Println("usage: omni <command> [flags] [args]")
	fmt.Println("")
	fmt.Println("commands:")
	fmt.Println("  enqueue [--profile default|architect] [--pipeline assistant|chat|story] [--web auto|on|off] [--workspace auto|on|off] [--allow-missing-tools] [--search-query text] [--reasoning auto|fast|deep] [--autonomy auto|on|off] [--approval auto|on|off] [--verify auto|on|off] [--verify-iterations 1-4] [--session id] [--model-plan m] [--model-analyze m] [--model-response m] [--model-search m] [--model-tagger m] [--model-verify m] [--model-memory m] <instruction>")
	fmt.Println("  chat [--profile default|architect] [--session id] [--web auto|on|off] [--workspace auto|on|off] [--local-media] [--local-browser] [--local-screen] [--local-shell] [--local-audio] [--allow-missing-tools] [--reasoning auto|fast|deep] [--autonomy auto|on|off] [--approval auto|on|off] [--verify auto|on|off] [--verify-iterations 1-4] [--confirm-actions] [--interval 2s] [--progress] [--verbose] [--max-chars 1200] [--model-plan m] [--model-analyze m] [--model-response m] [--model-search m] [--model-tagger m] [--model-verify m] [--model-memory m] [initial message]")
	fmt.Println("  list [--status status] [--limit N] [--offset N]")
	fmt.Println("  show [--inspect] <job-id>")
	fmt.Println("  watch [--interval 2s] [--progress] [--verbose] [--max-chars 1200] <job-id>")
	fmt.Println("  interrupt <job-id> <context text>")
	fmt.Println("  replan <job-id> <context text>")
	fmt.Println("  continue <job-id> <follow-up instruction>")
	fmt.Println("  cancel <job-id> [reason]")
	fmt.Println("  feedback <job-id> <text>")
	fmt.Println("  remember [--source name] [--kind episodic|procedural|instruction|preference|reference] [--tags a,b,c] <content>")
	fmt.Println("  memory-candidates <list|promote|reject> ...")
	fmt.Println("  ingest [--source name] [--kind reference] [--tags a,b,c] [--chunk-size N] [--overlap N] <file...>")
	fmt.Println("  media-index [--root dir] [--source media] [--kind reference] [--tags a,b,c] [--episode-limit N] [--lines-per-chunk N] [--include-no-subs] [--dry-run]")
	fmt.Println("  media-search [--root dir] [--context N] [--limit N] <query>")
	fmt.Println("  browser-scan [--console] [--email-watch] [--seconds N] [--limit N] [--ports csv] [--json]")
	fmt.Println("  screen-read [--ocr] [--vision] [--prompt text] [--model name] [--base-url url] [--keep] [--json]")
	fmt.Println("  research [--source research] [--kind reference] [--tags a,b,c] [--refresh-days N] [--force] [--include-web-context] [--include-analyze-context] [--chunk-size N] [--overlap N] [--max-chunks N] [--interval 2s] [--timeout 20m] [--session id] <topic>")
	fmt.Println("  audio-notes [doctor|start|stop|status|list|search] ...")
	fmt.Println("  permissions [list|path|grant|deny|unset|reset|help] ...")
	fmt.Println("  build [build-core.sh flags]         run scripts/build-core.sh")
	fmt.Println("  update [update.sh flags]            run update.sh")
	fmt.Println("  stash [stash flags]                 git stash helper for Omnidex repo")
	fmt.Println("  uninstall [uninstall.sh flags]      run uninstall.sh")
	fmt.Println("  migrate:fresh [--yes]  wipe Omnidex tables and re-run schema migrations via core")
	fmt.Println("  metrics <live|runs|models|playbooks|benchmarks|export>  query telemetry/benchmark metrics")
	fmt.Println("  status [--timeout 5s] [--queue-limit N] [--web-probe]  combined service status")
	fmt.Println("  core:status [--timeout 5s] [--core-url url]            core API health")
	fmt.Println("  queue:status [--timeout 5s] [--limit N] [--core-url url] queue sample counts")
	fmt.Println("  ollama:status [--timeout 5s] [--base-url url]          ollama connectivity + models")
	fmt.Println("  web:status [--timeout 5s] [--probe] [--providers csv]  web search provider status")
	fmt.Println("  service [--service name] <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("  service:<name> <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("  --service <name> <up|down|restart|status|logs|docker-logs|start|stop|build|migrate:fresh> [options]")
	fmt.Println("  config [--file path] [--editor cmd] [--print]          open Omnidex .env config")
	fmt.Println("  version [--json]                         print Omnidex release version")
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func mergeTags(parts ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 16)

	for _, list := range parts {
		for _, raw := range list {
			tag := strings.ToLower(strings.TrimSpace(raw))
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}

	return out
}

func latestContextValue(contexts []model.StepContext, key string) string {
	value := ""
	var lastID int64
	for _, ctx := range contexts {
		if ctx.Key != key {
			continue
		}
		if ctx.ID >= lastID {
			lastID = ctx.ID
			value = ctx.Value
		}
	}
	return value
}

func printStepStatusUpdates(steps []model.Step, lastStepStatus map[int64]string) bool {
	return printStepStatusUpdatesWithUI(steps, lastStepStatus, nil)
}

func printStepStatusUpdatesWithUI(steps []model.Step, lastStepStatus map[int64]string, ui *chatUI) bool {
	printed := false
	for _, step := range steps {
		status := strings.TrimSpace(step.Status)
		if lastStepStatus[step.ID] == status {
			continue
		}
		lastStepStatus[step.ID] = status
		if !printed {
			line := formatWorkloadQueueStatusLine(steps, ui)
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
		}
		line := formatStepStatusLine(step, ui)
		if ui != nil {
			emitSystem(ui, line)
		} else {
			fmt.Printf("  %s\n", line)
		}
		printed = true
	}
	return printed
}

func formatWorkloadQueueStatusLine(steps []model.Step, ui *chatUI) string {
	completed := 0
	incomplete := 0
	failed := 0
	active := model.Step{}
	for _, step := range steps {
		switch strings.ToLower(strings.TrimSpace(step.Status)) {
		case model.StepStatusCompleted:
			completed++
		case model.StepStatusFailed, model.StepStatusCanceled:
			failed++
			incomplete++
		default:
			incomplete++
			if active.ID == 0 && stepStatusIsActive(step.Status) {
				active = step
			}
		}
	}
	activeText := "none"
	if active.ID != 0 {
		activeText = fmt.Sprintf("#%d %s", active.ID, strings.TrimSpace(active.Action))
		if strings.TrimSpace(active.Action) == "" {
			activeText = fmt.Sprintf("#%d", active.ID)
		}
	}
	line := fmt.Sprintf("Workload queue | active=%s | completed=%d | incomplete=%d", activeText, completed, incomplete)
	if failed > 0 {
		line += fmt.Sprintf(" | failed=%d", failed)
	}
	if ui != nil && active.ID != 0 {
		return ui.paint(line, ansiBold+ansiYellow)
	}
	if ui != nil && incomplete == 0 {
		return ui.paint(line, ansiGreen)
	}
	return line
}

func formatStepStatusLine(step model.Step, ui *chatUI) string {
	marker := stepStatusMarker(step.Status)
	line := fmt.Sprintf("%s Step %d | phase=%s | action=%s | status=%s", marker, step.ID, phaseForStepAction(step.Action), step.Action, step.Status)
	if ui == nil {
		return line
	}
	switch strings.ToLower(strings.TrimSpace(step.Status)) {
	case model.StepStatusRunning, model.StepStatusWaiting:
		return ui.paint(line, ansiBold+ansiBlink+ansiYellow)
	case model.StepStatusCompleted:
		return ui.paint(line, ansiGreen)
	case model.StepStatusFailed, model.StepStatusCanceled:
		return ui.paint(line, ansiBold+ansiRed)
	case model.StepStatusPending:
		return ui.paint(line, ansiDim)
	default:
		return line
	}
}

func stepStatusMarker(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case model.StepStatusRunning, model.StepStatusWaiting:
		return ">> ACTIVE"
	case model.StepStatusCompleted:
		return "OK DONE"
	case model.StepStatusFailed, model.StepStatusCanceled:
		return "!! STOP"
	case model.StepStatusPending:
		return ".. TODO"
	default:
		return "-- STEP"
	}
}

func stepStatusIsActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case model.StepStatusRunning, model.StepStatusWaiting:
		return true
	default:
		return false
	}
}

func phaseForStepAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "plan", "tooling", "workspace_scan", "tag", "retrieve":
		return "planning"
	case "verify":
		return "review"
	default:
		return "execution"
	}
}

func printStepDetailUpdates(steps []model.Step, lastStepDetails map[int64]string, maxChars int) bool {
	printed := false
	for _, step := range steps {
		signature := strings.Join([]string{step.Status, step.Output, step.Error}, "\x00")
		if lastStepDetails[step.ID] == signature {
			continue
		}
		lastStepDetails[step.ID] = signature
		if output := strings.TrimSpace(step.Output); output != "" {
			fmt.Printf("  step %d output\n", step.ID)
			fmt.Println(indentBlock(truncateForWatch(output, maxChars), "    "))
			printed = true
		}
		if stepErr := strings.TrimSpace(step.Error); stepErr != "" {
			fmt.Printf("  step %d error\n", step.ID)
			fmt.Println(indentBlock(truncateForWatch(stepErr, maxChars), "    "))
			printed = true
		}
	}
	return printed
}

func printContextUpdates(
	contexts []model.StepContext,
	seenContextIDs map[int64]struct{},
	progress bool,
	verbose bool,
	maxChars int,
) bool {
	return printContextUpdatesWithUI(contexts, seenContextIDs, progress, verbose, maxChars, nil)
}

func printContextUpdatesWithUI(
	contexts []model.StepContext,
	seenContextIDs map[int64]struct{},
	progress bool,
	verbose bool,
	maxChars int,
	ui *chatUI,
) bool {
	printed := false
	for _, ctxValue := range contexts {
		if _, seen := seenContextIDs[ctxValue.ID]; seen {
			continue
		}
		seenContextIDs[ctxValue.ID] = struct{}{}
		value := strings.TrimSpace(ctxValue.Value)
		if value == "" {
			continue
		}
		switch ctxValue.Key {
		case "event":
			if !verbose {
				continue
			}
			event := parseStepEventPayload(value)
			eventType := strings.TrimSpace(event.EventType)
			if eventType == "" {
				eventType = "unknown"
			}
			line := fmt.Sprintf("event step=%d type=%s", ctxValue.StepID, eventType)
			block := indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line+"\n"+block)
			} else {
				fmt.Printf("  %s\n", line)
				fmt.Println(block)
			}
			printed = true
		case "tool_stdout":
			kind, summary := summarizeProgressStream("stdout", value, maxChars)
			if verbose {
				line := fmt.Sprintf("%s step=%d", strings.ToLower(kind), ctxValue.StepID)
				block := indentBlock(truncateForWatch(value, maxChars), "    ")
				if ui != nil {
					emitSystem(ui, line+"\n"+block)
				} else {
					fmt.Printf("  %s\n", line)
					fmt.Println(block)
				}
			} else if progress {
				line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
			} else {
				continue
			}
			printed = true
		case "tool_stderr":
			kind, summary := summarizeProgressStream("stderr", value, maxChars)
			if verbose {
				line := fmt.Sprintf("%s step=%d", strings.ToLower(kind), ctxValue.StepID)
				block := indentBlock(truncateForWatch(value, maxChars), "    ")
				if ui != nil {
					emitSystem(ui, line+"\n"+block)
				} else {
					fmt.Printf("  %s\n", line)
					fmt.Println(block)
				}
			} else if progress {
				line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
			} else {
				continue
			}
			printed = true
		case "workspace":
			if !verbose {
				continue
			}
			line := fmt.Sprintf("Explore step %d: scanned workspace snapshot", ctxValue.StepID)
			line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "web_search":
			if !progress && !verbose {
				continue
			}
			domains := webSearchDomainsFromContext(value)
			domainSummary := summarizeWebSearchDomains(domains, maxChars)
			line := fmt.Sprintf("Explore step %d: compiled web research context", ctxValue.StepID)
			if domainSummary != "" {
				line += " | domains: " + domainSummary
			}
			if verbose {
				if domainSummary != "" {
					line += "\n" + indentBlock("domains hit: "+domainSummary, "    ")
				}
				line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			}
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "environment", "host_environment":
			if !verbose {
				continue
			}
			line := fmt.Sprintf("Inspect step %d: captured environment details", ctxValue.StepID)
			line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "retrieved_memory", "recent_conversation":
			if !verbose {
				continue
			}
			line := fmt.Sprintf("Inspect step %d: loaded conversation/memory context", ctxValue.StepID)
			line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "llm_model_prepare":
			if !progress && !verbose {
				continue
			}
			kind, summary := summarizePreparedModelContext(value, maxChars)
			line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
			if verbose {
				line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			}
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "llm_prompt":
			if !verbose {
				continue
			}
			kind, summary := summarizeLLMTraceContext(ctxValue.Key, value, maxChars)
			line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
			block := indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line+"\n"+block)
			} else {
				fmt.Printf("  %s\n", line)
				fmt.Println(block)
			}
			printed = true
		case "llm_response":
			if !progress && !verbose {
				continue
			}
			kind, summary := summarizeLLMTraceContext(ctxValue.Key, value, maxChars)
			line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
			if verbose {
				block := indentBlock(truncateForWatch(value, maxChars), "    ")
				if ui != nil {
					emitSystem(ui, line+"\n"+block)
				} else {
					fmt.Printf("  %s\n", line)
					fmt.Println(block)
				}
			} else {
				body := llmTraceBody(value)
				if body != "" {
					line += "\n" + indentBlock(truncateForWatch(body, maxChars), "    ")
				}
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
			}
			printed = true
		case "llm_error":
			if !progress && !verbose {
				continue
			}
			kind, summary := summarizeLLMTraceContext(ctxValue.Key, value, maxChars)
			line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
			if verbose {
				block := indentBlock(truncateForWatch(value, maxChars), "    ")
				if ui != nil {
					emitSystem(ui, line+"\n"+block)
				} else {
					fmt.Printf("  %s\n", line)
					fmt.Println(block)
				}
			} else {
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
			}
			printed = true
		default:
			if !verbose {
				continue
			}
			line := fmt.Sprintf("context step=%d key=%s", ctxValue.StepID, ctxValue.Key)
			block := indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line+"\n"+block)
			} else {
				fmt.Printf("  %s\n", line)
				fmt.Println(block)
			}
			printed = true
		}
	}
	return printed
}

type stepEventPayload struct {
	Time      string
	EventType string
	Message   string
}

func parseStepEventPayload(raw string) stepEventPayload {
	fields := strings.Fields(strings.TrimSpace(raw))
	payload := stepEventPayload{}
	rest := make([]string, 0, len(fields))
	for _, field := range fields {
		switch {
		case strings.HasPrefix(field, "time="):
			payload.Time = strings.TrimSpace(strings.TrimPrefix(field, "time="))
		case strings.HasPrefix(field, "event="):
			payload.EventType = strings.TrimSpace(strings.TrimPrefix(field, "event="))
		default:
			rest = append(rest, field)
		}
	}
	payload.Message = strings.TrimSpace(strings.Join(rest, " "))
	return payload
}

func summarizeStepEvent(payload stepEventPayload) string {
	eventType := strings.ToLower(strings.TrimSpace(payload.EventType))
	message := strings.TrimSpace(payload.Message)
	switch eventType {
	case "step_start":
		return "Starting step execution"
	case "step_complete":
		return "Step completed"
	case "plan_begin":
		return "Inspecting request and drafting plan"
	case "plan_ready":
		return "Plan ready"
	case "tooling_begin":
		return "Inspecting environment and required tools"
	case "tooling_ready":
		return "Tooling check complete"
	case "workspace_scan_begin":
		return "Exploring workspace files"
	case "workspace_scan_ready":
		return "Workspace scan complete"
	case "tag_begin":
		return "Tagging instruction context"
	case "tag_ready":
		return "Tags ready"
	case "retrieve_begin":
		return "Retrieving relevant memory"
	case "retrieve_ready":
		return "Memory retrieval complete"
	case "analyze_begin":
		return "Analyzing gathered context"
	case "analyze_ready":
		return "Analysis complete"
	case "response_begin":
		return "Drafting response"
	case "response_ready":
		return "Response draft ready"
	case "verify_begin":
		return "Reviewing and verifying response"
	case "verify_ready":
		return "Verification complete"
	case "web_search_begin":
		return "Exploring web sources"
	case "web_search_ready":
		return "Web research context ready"
	case "web_search_skipped":
		reason := eventMessageField(message, "reason")
		if reason != "" {
			return "Web search skipped (" + reason + ")"
		}
		return "Web search skipped"
	}

	if strings.HasSuffix(eventType, "_waiting_input") {
		if message != "" {
			return "Waiting for your input (" + message + ")"
		}
		return "Waiting for your input"
	}
	if strings.HasSuffix(eventType, "_error") {
		if message != "" {
			return "Step error: " + message
		}
		return "Step error"
	}
	if message != "" {
		return strings.TrimSpace(eventType + " " + message)
	}
	if eventType != "" {
		return eventType
	}
	return "event update"
}

func eventMessageField(message, key string) string {
	needle := strings.TrimSpace(key) + "="
	for _, field := range strings.Fields(strings.TrimSpace(message)) {
		if !strings.HasPrefix(field, needle) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(field, needle))
	}
	return ""
}

func summarizeProgressStream(stream, value string, maxChars int) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(lower, "web search query:"):
		query := strings.TrimSpace(value[len("web search query:"):])
		return "Explore", compactProgressValue("Search web for "+query, maxChars)
	case strings.HasPrefix(lower, "web search context chars="):
		return "Explore", compactProgressValue("Collected web research context", maxChars)
	case strings.HasPrefix(lower, "tool check:"):
		check := strings.TrimSpace(value[len("tool check:"):])
		return "Inspect", compactProgressValue(check, maxChars)
	case strings.HasPrefix(lower, "running test:"):
		command := strings.TrimSpace(value[len("running test:"):])
		return "Run", compactProgressValue("Test "+command, maxChars)
	case strings.HasPrefix(lower, "plan generated chars="):
		return "Plan", compactProgressValue("Generated planning draft", maxChars)
	}

	if strings.EqualFold(stream, "stderr") {
		return "Warn", compactProgressValue(value, maxChars)
	}
	return "Inspect", compactProgressValue(value, maxChars)
}

func summarizeWebSearchDomains(domains []string, maxChars int) string {
	if len(domains) == 0 {
		return ""
	}
	return compactProgressValue(strings.Join(domains, ", "), maxChars)
}

func webSearchDomainsFromContext(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	urlPattern := regexp.MustCompile(`https?://[^\s]+`)
	matches := urlPattern.FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, raw := range matches {
		clean := strings.TrimSpace(strings.TrimRight(raw, ".,);]}>\"'"))
		if clean == "" {
			continue
		}
		parsed, err := url.Parse(clean)
		if err != nil {
			continue
		}
		host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	return out
}

func summarizeLLMTraceContext(key, value string, maxChars int) (string, string) {
	scope := traceKVField(value, "scope")
	model := traceKVField(value, "model")
	chars := traceKVField(value, "prompt_chars")
	if strings.EqualFold(strings.TrimSpace(key), "llm_response") {
		chars = traceKVField(value, "response_chars")
	}
	if chars == "" && strings.EqualFold(strings.TrimSpace(key), "llm_error") {
		chars = traceKVField(value, "error")
	}

	kind := "Trace"
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "llm_prompt":
		kind = "Prompt"
	case "llm_response":
		kind = "Response"
	case "llm_error":
		kind = "Warn"
	}

	parts := make([]string, 0, 4)
	if scope != "" {
		parts = append(parts, "scope="+scope)
	}
	if roleID := roleForLLMScope(scope); roleID != "" {
		parts = append(parts, "role="+roleID)
	}
	if model != "" {
		parts = append(parts, "model="+model)
	}
	if chars != "" {
		if strings.EqualFold(strings.TrimSpace(key), "llm_error") {
			parts = append(parts, "error="+chars)
		} else {
			parts = append(parts, "chars="+chars)
		}
	}
	if len(parts) == 0 {
		return kind, compactProgressValue(value, maxChars)
	}
	return kind, compactProgressValue(strings.Join(parts, " "), maxChars)
}

func summarizePreparedModelContext(value string, maxChars int) (string, string) {
	scope := traceKVField(value, "scope")
	baseModel := traceKVField(value, "base_model")
	contextModel := traceKVField(value, "context_model")
	modelfilePath := traceKVField(value, "modelfile_path")
	promptHint := traceKVField(value, "prompt_hint")

	parts := make([]string, 0, 6)
	if scope != "" {
		parts = append(parts, "scope="+scope)
	}
	if roleID := roleForLLMScope(scope); roleID != "" {
		parts = append(parts, "role="+roleID)
	}
	if baseModel != "" {
		parts = append(parts, "base_model="+baseModel)
	}
	if contextModel != "" {
		parts = append(parts, "context_model="+contextModel)
	}
	if modelfilePath != "" {
		parts = append(parts, "modelfile="+modelfilePath)
	}
	if promptHint != "" {
		parts = append(parts, "prompt_hint="+promptHint)
	}
	if len(parts) == 0 {
		return "Model", compactProgressValue(value, maxChars)
	}
	return "Model", compactProgressValue(strings.Join(parts, " "), maxChars)
}

func roleForLLMScope(scope string) string {
	clean := strings.ToLower(strings.TrimSpace(scope))
	switch {
	case strings.HasPrefix(clean, "plan"):
		return specialist.RolePlannerSpecialist
	case strings.HasPrefix(clean, "analyze"):
		return specialist.RoleAnalysisSpecialist
	case strings.HasPrefix(clean, "response"):
		return specialist.RoleResponseSpecialist
	case strings.HasPrefix(clean, "verify"):
		return specialist.RoleReviewVerificationSpecialist
	case strings.HasPrefix(clean, "search_query"):
		return specialist.RoleWebResearchSpecialist
	case strings.HasPrefix(clean, "memory"):
		return specialist.RoleMemoryRetrievalSpecialist
	case strings.HasPrefix(clean, "tag"):
		return specialist.RoleIntentTaggingSpecialist
	default:
		return ""
	}
}

func llmTraceBody(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) >= 4 {
		first := strings.ToLower(strings.TrimSpace(lines[0]))
		second := strings.ToLower(strings.TrimSpace(lines[1]))
		third := strings.ToLower(strings.TrimSpace(lines[2]))
		if strings.HasPrefix(first, "scope=") &&
			strings.HasPrefix(second, "model=") &&
			(strings.HasPrefix(third, "response_chars=") || strings.HasPrefix(third, "prompt_chars=") || strings.HasPrefix(third, "error=")) {
			return strings.TrimSpace(strings.Join(lines[3:], "\n"))
		}
	}
	return strings.TrimSpace(value)
}

func traceKVField(value, key string) string {
	needle := strings.ToLower(strings.TrimSpace(key)) + "="
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		clean := strings.TrimSpace(line)
		lower := strings.ToLower(clean)
		if !strings.HasPrefix(lower, needle) {
			continue
		}
		return strings.TrimSpace(clean[len(needle):])
	}
	return ""
}

func truncateForWatch(value string, maxChars int) string {
	text := strings.TrimSpace(value)
	if maxChars <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "\n...[truncated]"
}

func compactProgressValue(value string, maxChars int) string {
	text := strings.TrimSpace(strings.ReplaceAll(value, "\n", " | "))
	if maxChars <= 0 || maxChars > 320 {
		maxChars = 320
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "...[truncated]"
}

func indentBlock(value, prefix string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
