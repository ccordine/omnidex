package omni

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (a *App) printBanner(session *Session, loaded bool, noOllama bool) {
	fmt.Fprintln(a.out, "\n========================================")
	fmt.Fprintln(a.out, "Omnidex (omni) - deterministic core")
	fmt.Fprintln(a.out, "========================================")
	fmt.Fprintf(a.out, "Workspace: %s\n", session.WorkspacePath)
	fmt.Fprintf(a.out, "Session ID: %s\n", session.WorkspaceHash)
	fmt.Fprintf(a.out, "Permission: %s\n", session.Permission)
	if noOllama {
		fmt.Fprintln(a.out, "Conversation model: disabled")
	} else if a.ollama != nil {
		fmt.Fprintf(a.out, "Conversation model: %s\n", a.ollama.Model)
		if a.planner != nil {
			fmt.Fprintf(a.out, "Structured planner model: %s\n", a.planner.Model)
		}
		if evaluator, ok := a.evaluator.(OllamaStructuredResponseEvaluator); ok && evaluator.Client != nil {
			if client, ok := evaluator.Client.(*OllamaClient); ok {
				fmt.Fprintf(a.out, "Evaluator model: %s (threshold %d)\n", client.Model, normalizeStructuredEvaluatorThreshold(a.evaluatorThreshold))
			}
		}
		if shellSpecialist, ok := a.shellSpecialist.(OllamaShellCommandSpecialist); ok && shellSpecialist.Client != nil {
			if client, ok := shellSpecialist.Client.(*OllamaClient); ok {
				fmt.Fprintf(a.out, "Shell specialist model: %s\n", client.Model)
			}
		}
	}
	if a.runLogger != nil {
		fmt.Fprintf(a.out, "Run ID: %s\n", a.runLogger.RunID())
		fmt.Fprintf(a.out, "Run log: %s\n", a.runLogger.Path())
	}
	if loaded {
		fmt.Fprintf(a.out, "Loaded existing session with %d turn(s).\n", len(session.Turns))
	} else {
		fmt.Fprintln(a.out, "Created new workspace session.")
	}
	fmt.Fprintln(a.out, "Type /help for commands.")
}

func (a *App) printHelp() {
	fmt.Fprintln(a.out, "")
	fmt.Fprintln(a.out, "Commands:")
	fmt.Fprintln(a.out, "  /help         show commands")
	fmt.Fprintln(a.out, "  /status       show current workspace/session status")
	fmt.Fprintln(a.out, "  /history      show recent turns")
	fmt.Fprintln(a.out, "  /mode         change permission mode")
	fmt.Fprintln(a.out, "  /clear        clear workspace session history")
	fmt.Fprintln(a.out, "  /thoughts     list internal thought logs for this workspace")
	fmt.Fprintln(a.out, "  /thoughts ID  inspect one turn's thought channels")
	fmt.Fprintln(a.out, "  /think X      reasoning only via thinking pilot (no execution)")
	fmt.Fprintln(a.out, "  /search X     quick memory + web search answer (no Postgres store)")
	fmt.Fprintln(a.out, "  /research X   web research stored in Postgres memory")
	fmt.Fprintln(a.out, "  /plan X       planning/read-only inspection (no writes)")
	fmt.Fprintln(a.out, "  /build X      force implementation/build execution path")
	fmt.Fprintln(a.out, "  /manage X     run X through manager-worker orchestration")
	fmt.Fprintln(a.out, "  /job X        alias for /manage X")
	fmt.Fprintln(a.out, "  /micro X      run X through project-profiled micro job queue")
	fmt.Fprintln(a.out, "  /queue X      alias for /micro X")
	fmt.Fprintln(a.out, "  /exit         exit")
	fmt.Fprintln(a.out, "")
	fmt.Fprintln(a.out, "Plain messages go through the thinking pilot first, then execution when needed.")
}

func (a *App) printStatus(session *Session) {
	fmt.Fprintln(a.out, "")
	fmt.Fprintf(a.out, "Workspace: %s\n", session.WorkspacePath)
	fmt.Fprintf(a.out, "Active directory: %s\n", activeDirectoryOrWorkspace(session))
	fmt.Fprintf(a.out, "Session ID: %s\n", session.WorkspaceHash)
	fmt.Fprintf(a.out, "Permission: %s\n", session.Permission)
	fmt.Fprintf(a.out, "Turns: %d\n", len(session.Turns))
	if a.ollama != nil {
		fmt.Fprintf(a.out, "Ollama model: %s\n", a.ollama.Model)
		fmt.Fprintf(a.out, "Ollama endpoint: %s\n", a.ollama.Endpoint)
		keepAlive := a.ollama.DefaultKeepAlive
		if strings.TrimSpace(keepAlive) == "" {
			keepAlive = "ollama-default"
		}
		fmt.Fprintf(a.out, "Ollama request defaults: keep_alive=%s num_ctx=%d\n", keepAlive, a.ollama.DefaultNumCtx)
		if a.planner != nil {
			fmt.Fprintf(a.out, "Structured planner model: %s num_ctx=%d\n", a.planner.Model, a.planner.DefaultNumCtx)
		}
		if evaluator, ok := a.evaluator.(OllamaStructuredResponseEvaluator); ok && evaluator.Client != nil {
			if client, ok := evaluator.Client.(*OllamaClient); ok {
				fmt.Fprintf(a.out, "Evaluator model: %s threshold=%d num_ctx=%d\n", client.Model, normalizeStructuredEvaluatorThreshold(a.evaluatorThreshold), client.DefaultNumCtx)
			}
		} else {
			fmt.Fprintln(a.out, "Evaluator model: disabled")
		}
		if shellSpecialist, ok := a.shellSpecialist.(OllamaShellCommandSpecialist); ok && shellSpecialist.Client != nil {
			if client, ok := shellSpecialist.Client.(*OllamaClient); ok {
				fmt.Fprintf(a.out, "Shell specialist model: %s num_ctx=%d\n", client.Model, client.DefaultNumCtx)
			}
		} else {
			fmt.Fprintln(a.out, "Shell specialist model: disabled")
		}
	} else {
		fmt.Fprintln(a.out, "Ollama model: disabled")
	}
	if a.runLogger != nil {
		fmt.Fprintf(a.out, "Run ID: %s\n", a.runLogger.RunID())
		fmt.Fprintf(a.out, "Run log: %s\n", a.runLogger.Path())
	}
	if a.memory != nil {
		fmt.Fprintln(a.out, "Memory DB: connected")
	} else {
		fmt.Fprintln(a.out, "Memory DB: not configured")
	}
	fmt.Fprintf(a.out, "Session memories: %d\n", len(session.Memories))

	fmt.Fprintln(a.out, "")
	fmt.Fprintln(a.out, "Execution stack:")
	fmt.Fprintln(a.out, "  normal prompts: execution-first command loop")
	fmt.Fprintln(a.out, "  context plan: auto-select web research, memory, docs, shell")
	fmt.Fprintln(a.out, "  /manage, /job: manager-worker orchestration")
	fmt.Fprintln(a.out, "  /micro, /queue: project-profiled manager-manager micro job queue")
	fmt.Fprintln(a.out, "  document search: chunked manager-worker needle finding")
	fmt.Fprintln(a.out, "  web docs: fetch, normalize, chunk, search, and cite documentation")
	fmt.Fprintln(a.out, "  memory: Postgres-backed tags + query retrieval")
	fmt.Fprintln(a.out, "  /research: search web, follow result links, store source chunks in memory")
	fmt.Fprintln(a.out, "  relay service: exact JSON handoff with checksum validation")
	fmt.Fprintf(a.out, "  structured command loop: max_steps=%d task_budget=%s ollama_request_timeout=%s\n",
		defaultCommandDecisionMaxSteps,
		defaultCommandDecisionTimeout,
		defaultOllamaRequestTimeout,
	)
	fmt.Fprintf(a.out, "  command loop: max_steps=%d max_commands_per_step=%d planner_timeout=%s command_timeout=%s\n",
		defaultAgentLoopSteps,
		defaultAgentCommandsPerStep,
		defaultPlannerTimeout,
		defaultCommandTimeout,
	)
	fmt.Fprintf(a.out, "  manager: max_workers=%d plan_timeout=%s reduce_timeout=%s\n",
		defaultManagerMaxWorkers,
		defaultManagerPlanTimeout,
		defaultManagerReduceTimeout,
	)
	fmt.Fprintf(a.out, "  document chunks: chunk_chars=%d overlap=%d\n",
		defaultDocumentChunkChars,
		defaultDocumentChunkOverlap,
	)

	implementedTools := a.registry.ToolIDs(true)
	plannedTools := a.registry.ToolIDs(false)
	fmt.Fprintln(a.out, "")
	fmt.Fprintf(a.out, "Tools: implemented=%d registered=%d\n", len(implementedTools), len(plannedTools))
	if len(implementedTools) > 0 {
		fmt.Fprintf(a.out, "  implemented: %s\n", strings.Join(implementedTools, ", "))
	}

	fmt.Fprintln(a.out, "")
	a.printLastTurnStatus(session)
}

func (a *App) printLastTurnStatus(session *Session) {
	if len(session.Turns) == 0 {
		fmt.Fprintln(a.out, "Last turn: none")
		return
	}

	last := session.Turns[len(session.Turns)-1]
	counts := countEventTypes(last.Events)
	fmt.Fprintf(a.out, "Last turn: %s at %s\n", last.ID, last.CreatedAt)
	fmt.Fprintf(a.out, "  user: %s\n", last.UserInput)
	fmt.Fprintf(a.out, "  response: %s\n", last.Response)
	fmt.Fprintf(a.out, "  reason_codes: %s\n", strings.Join(last.ReasonCodes, ","))
	fmt.Fprintf(a.out, "  events=%d commands_success=%d commands_failed=%d policy_blocked=%d manager_events=%d worker_events=%d\n",
		len(last.Events),
		counts["command_success"]+counts["command_executed"],
		counts["command_failed"],
		counts["policy_blocked"],
		counts["manager_started"]+counts["manager_plan_created"]+counts["manager_reduced"]+counts["manager_completed"],
		counts["worker_completed"],
	)

	if len(last.Events) == 0 {
		return
	}
	recent := last.Events
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}
	fmt.Fprintln(a.out, "  recent timeline:")
	for _, evt := range recent {
		fmt.Fprintf(a.out, "    - %s: %s\n", evt.Type, evt.Summary)
		if command := evt.Details["command"]; strings.TrimSpace(command) != "" {
			fmt.Fprintf(a.out, "      command=%s\n", command)
		}
		if stdout := evt.Details["stdout"]; strings.TrimSpace(stdout) != "" {
			fmt.Fprintf(a.out, "      stdout=%s\n", truncateOutput(stdout))
		}
		if reason := evt.Details["reason_code"]; strings.TrimSpace(reason) != "" {
			fmt.Fprintf(a.out, "      reason=%s\n", reason)
		}
	}
}

func countEventTypes(events []Event) map[string]int {
	counts := make(map[string]int, len(events))
	for _, evt := range events {
		counts[evt.Type]++
	}
	return counts
}

func (a *App) printHistory(session *Session) {
	fmt.Fprintln(a.out, "")
	if len(session.Turns) == 0 {
		fmt.Fprintln(a.out, "No turns yet.")
		return
	}

	start := len(session.Turns) - 8
	if start < 0 {
		start = 0
	}
	for _, turn := range session.Turns[start:] {
		fmt.Fprintf(a.out, "- %s  intent=%s  confidence=%.2f\n", turn.ID, turn.IntentClassification, turn.Confidence)
		fmt.Fprintf(a.out, "  user: %s\n", turn.UserInput)
		fmt.Fprintf(a.out, "  assistant: %s\n", turn.Response)
	}
}

func (a *App) printTimeline(events []Event) {
	if len(events) == 0 {
		return
	}
	fmt.Fprintln(a.out, "\ntimeline")
	fmt.Fprintln(a.out, "--------")
	for _, evt := range events {
		a.printTimelineEvent(evt)
	}
}

func (a *App) printTimelineEvent(evt Event) {
	prefix := "  "
	if isThinkingTimelineEvent(evt.Type) {
		prefix = "  THK> "
	}
	fmt.Fprintf(a.out, "\n[%s]\n", evt.CreatedAt)
	fmt.Fprintf(a.out, "%s%-32s %s\n", prefix, evt.Type, evt.Summary)
	if len(evt.Details) == 0 {
		return
	}
	keys := make([]string, 0, len(evt.Details))
	for k := range evt.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		value := evt.Details[k]
		if shouldTruncateTimelineValue(k, value) {
			value = value[:timelineDetailLimit(k)] + "..."
		}
		a.printTimelineDetail(k, value)
	}
}

func (a *App) printTimelineDetail(key, value string) {
	value = strings.TrimRight(value, "\n")
	if strings.Contains(value, "\n") {
		fmt.Fprintf(a.out, "  %-20s |\n", key)
		fmt.Fprintln(a.out, indentTimelineBlock(value, "    "))
		return
	}
	fmt.Fprintf(a.out, "  %-20s %s\n", key, value)
}

func indentTimelineBlock(value, prefix string) string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func shouldTruncateTimelineValue(key, value string) bool {
	return len(value) > timelineDetailLimit(key)
}

func timelineDetailLimit(key string) int {
	switch strings.TrimSpace(key) {
	case "stdout", "stderr", "command":
		return defaultStructuredObservationChars
	case "thought", "conclusion", "thinking", "result", "recovery_tool_task":
		return thoughtTimelineDetailLimit
	default:
		return 400
	}
}

func (a *App) nextEventID() string {
	a.eventSequence++
	return fmt.Sprintf("evt_%06d", a.eventSequence)
}

func (a *App) newEvent(eventType, summary string, details map[string]string) Event {
	return Event{
		ID:        a.nextEventID(),
		Type:      eventType,
		Summary:   summary,
		Details:   details,
		CreatedAt: nowUTC(),
	}
}

func sortedCopy(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return copyValues
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envIntOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func workspacePathOrCurrentDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return wd
	}
	return abs
}

func findManagedUpdateScript() (string, error) {
	roots := managedScriptRootCandidates(
		strings.TrimSpace(os.Getenv("OMNIDEX_DIR")),
		workspacePathOrCurrentDir(),
		currentExecutablePath(),
	)
	if script := locateManagedScript(roots, "update.sh"); script != "" {
		return script, nil
	}
	return "", fmt.Errorf("unable to locate update.sh; run from the Omnidex install/repo root or set OMNIDEX_DIR")
}

func managedScriptRootCandidates(envRoot, cwd, executablePath string) []string {
	raw := []string{envRoot}
	if strings.TrimSpace(executablePath) != "" {
		exeDir := filepath.Dir(executablePath)
		raw = append(raw, exeDir, filepath.Dir(exeDir))
	}
	raw = append(raw, cwd)
	return dedupeCleanAbsPaths(raw)
}

func locateManagedScript(roots []string, scriptName string) string {
	scriptName = filepath.Clean(strings.TrimSpace(scriptName))
	if scriptName == "" || scriptName == "." || filepath.IsAbs(scriptName) {
		return ""
	}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		candidate := filepath.Join(root, scriptName)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func resolveOmniResourceRoot(root, defaultName string) string {
	root = strings.TrimSpace(root)
	defaultName = filepath.Clean(strings.TrimSpace(defaultName))
	if defaultName == "" || defaultName == "." || filepath.IsAbs(defaultName) {
		defaultName = ""
	}
	if root == "" {
		root = defaultName
	}
	if filepath.IsAbs(root) {
		return root
	}
	for _, candidate := range omniResourceRootCandidates(root, defaultName) {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return filepath.Clean(root)
}

func omniResourceRootCandidates(root, defaultName string) []string {
	raw := []string{}
	if strings.TrimSpace(root) != "" {
		raw = append(raw, root)
	}
	resourceName := strings.TrimSpace(root)
	if resourceName == "" {
		resourceName = defaultName
	}
	if strings.TrimSpace(resourceName) != "" && !filepath.IsAbs(resourceName) {
		roots := managedScriptRootCandidates(
			strings.TrimSpace(os.Getenv("OMNIDEX_DIR")),
			workspacePathOrCurrentDir(),
			currentExecutablePath(),
		)
		for _, base := range roots {
			raw = append(raw, filepath.Join(base, resourceName))
		}
	}
	return dedupeCleanAbsPaths(raw)
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && strings.TrimSpace(resolved) != "" {
		path = resolved
	}
	return path
}

func dedupeCleanAbsPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, raw := range paths {
		clean := strings.TrimSpace(raw)
		if clean == "" {
			continue
		}
		if abs, err := filepath.Abs(clean); err == nil {
			clean = abs
		}
		clean = filepath.Clean(clean)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func activeDirectoryOrWorkspace(session *Session) string {
	if session == nil {
		return ""
	}
	if strings.TrimSpace(session.ActiveDirectoryPath) != "" {
		return strings.TrimSpace(session.ActiveDirectoryPath)
	}
	return strings.TrimSpace(session.WorkspacePath)
}

func (a *App) resolveActiveDirectoryForTurn(session *Session, _ string, _ func(string, string, map[string]string)) string {
	activeDirectory := activeDirectoryOrWorkspace(session)
	if session == nil {
		return activeDirectory
	}
	if strings.TrimSpace(activeDirectory) == "" {
		activeDirectory = strings.TrimSpace(session.WorkspacePath)
	}
	return activeDirectory
}

func isInteractive(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func isInteractiveWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func isLiveTimelineWriter(w io.Writer) bool {
	_, ok := w.(*os.File)
	return ok
}
