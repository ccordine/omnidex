package odn

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer

	store    SessionStore
	ollama   *OllamaClient
	registry Registry
	memory   *PGMemoryStore
	pgPool   *pgxpool.Pool
	web      WebSearchService

	runLogger *RunLogger

	eventSequence int
}

func NewApp(in io.Reader, out, errOut io.Writer) *App {
	return &App{in: in, out: out, errOut: errOut, registry: DefaultRegistry()}
}

func (a *App) Run(args []string) error {
	if len(args) > 0 && args[0] == "migrate" {
		return a.runMigrate(args[1:])
	}
	strictOneShot := false
	if len(args) > 0 && args[0] == "run" {
		strictOneShot = true
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "chat" {
		args = args[1:]
	}

	fs := flag.NewFlagSet("odn", flag.ContinueOnError)
	fs.SetOutput(a.errOut)

	permissionFlag := fs.String("permission", "", "permission mode: ask_permission|full_access")
	modelFlag := fs.String("model", defaultOllamaModel, "ollama model to use for command decisions")
	endpointFlag := fs.String("ollama-endpoint", defaultOllamaEndpoint, "ollama chat endpoint")
	ollamaKeepAlive := fs.String("ollama-keep-alive", envOrDefault("ODN_OLLAMA_KEEP_ALIVE", "30s"), "default Ollama keep_alive for chat requests; use 0 to unload after each response")
	ollamaNumCtx := fs.Int("ollama-num-ctx", envIntOrDefault("ODN_OLLAMA_NUM_CTX", 2048), "default Ollama num_ctx option; set 0 to use Ollama default")
	noOllama := fs.Bool("no-ollama", false, "disable ollama calls")
	sessionRoot := fs.String("session-root", "", "override session root directory")
	runLogRoot := fs.String("run-log-root", "", "override run log root directory")
	memoryDatabaseURL := fs.String("memory-database-url", "", "Postgres URL for /research memory storage")
	skipPermissionPrompt := fs.Bool("no-permission-prompt", false, "skip startup permission prompt and keep current/default mode")

	fs.Usage = func() {
		fmt.Fprintln(a.errOut, "Usage: odn [chat|run] [flags]")
		fmt.Fprintln(a.errOut, "")
		fmt.Fprintln(a.errOut, "Commands:")
		fmt.Fprintln(a.errOut, "  odn          start chat when interactive; run one-shot when stdin is piped")
		fmt.Fprintln(a.errOut, "  odn chat     start interactive chat")
		fmt.Fprintln(a.errOut, "  odn run      strict stdin -> LLM JSON command -> execute")
		fmt.Fprintln(a.errOut, "  odn migrate  run migration commands")
		fmt.Fprintln(a.errOut, "")
		fmt.Fprintln(a.errOut, "Flags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument(s): %s", strings.Join(fs.Args(), " "))
	}

	if !*noOllama {
		a.ollama = NewOllamaClient(*endpointFlag, *modelFlag)
		a.ollama.ConfigureRuntime(*ollamaKeepAlive, *ollamaNumCtx)
	}

	if strictOneShot || !isInteractive(a.in) {
		promptBytes, err := io.ReadAll(a.in)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		_, err = RunStructuredCommandDecision(context.Background(), string(promptBytes), a.ollama, a.out, a.errOut)
		return err
	}

	workspacePath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	workspacePath, err = filepath.Abs(workspacePath)
	if err != nil {
		return fmt.Errorf("resolve absolute workspace: %w", err)
	}

	a.store = NewSessionStore(*sessionRoot)
	session, loaded, err := a.store.LoadOrCreate(workspacePath)
	if err != nil {
		return err
	}

	if strings.TrimSpace(*permissionFlag) != "" {
		parsed, err := ParsePermissionMode(strings.TrimSpace(*permissionFlag))
		if err != nil {
			return err
		}
		session.Permission = parsed
	} else if !*skipPermissionPrompt && isInteractive(a.in) && isInteractiveWriter(a.out) {
		selected, err := PromptPermissionMode(a.in, a.out, session.Permission)
		if err != nil {
			return err
		}
		session.Permission = selected
	}

	if err := a.store.Save(session); err != nil {
		return err
	}

	a.web = websearch.New([]string{"duckduckgo", "yahoo", "google"}, 20*time.Second, 3000, 7000)
	if dbURL := firstNonEmpty(*memoryDatabaseURL, os.Getenv("ODN_MEMORY_DATABASE_URL"), os.Getenv("DATABASE_URL")); dbURL != "" {
		poolCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pool, poolErr := pgxpool.New(poolCtx, dbURL)
		cancel()
		if poolErr != nil {
			return fmt.Errorf("connect memory database: %w", poolErr)
		}
		a.pgPool = pool
		a.memory = NewPGMemoryStore(NewPgxMemoryRunner(pool))
		defer a.pgPool.Close()
	}

	a.runLogger, err = NewRunLogger(*runLogRoot, session.WorkspaceHash)
	if err != nil {
		return err
	}
	defer func() {
		_ = a.runLogger.Close()
	}()

	_ = a.runLogger.Log("runtime", "app_initialized", map[string]interface{}{
		"workspace":       workspacePath,
		"permission_mode": session.Permission,
		"ollama_enabled":  !*noOllama,
		"model":           *modelFlag,
		"endpoint":        *endpointFlag,
		"loaded_session":  loaded,
	})

	a.printBanner(session, loaded, *noOllama)
	return a.loop(session)
}

func (a *App) runMigrate(args []string) error {
	fs := flag.NewFlagSet("odn migrate", flag.ContinueOnError)
	fs.SetOutput(a.errOut)

	dir := fs.String("dir", filepath.Join("database", "migrations"), "migration directory")
	steps := fs.Int("steps", 0, "number of steps (up: 0 means all pending, down: 0 means 1)")
	dbMode := fs.String("db-mode", "", "database mode: docker_exec|direct")
	dbContainer := fs.String("db-container", "", "docker container name for docker_exec mode")
	dbHost := fs.String("db-host", "", "database host for direct mode")
	dbPort := fs.String("db-port", "", "database port for direct mode")
	dbName := fs.String("db-name", "", "database name")
	dbUser := fs.String("db-user", "", "database user")
	dbPassword := fs.String("db-password", "", "database password")
	dbSSLMode := fs.String("db-sslmode", "", "database sslmode for direct mode")

	fs.Usage = func() {
		fmt.Fprintln(a.errOut, "Usage: odn migrate <create|up|down|status> [args] [flags]")
		fmt.Fprintln(a.errOut, "")
		fmt.Fprintln(a.errOut, "Examples:")
		fmt.Fprintln(a.errOut, "  odn migrate create create_runs_table")
		fmt.Fprintln(a.errOut, "  odn migrate up --steps 2")
		fmt.Fprintln(a.errOut, "  odn migrate down --steps 1")
		fmt.Fprintln(a.errOut, "  odn migrate status")
		fmt.Fprintln(a.errOut, "")
		fmt.Fprintln(a.errOut, "Flags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("migration subcommand is required")
	}

	cfg := DefaultMigrationDBConfig()
	if strings.TrimSpace(*dbMode) != "" {
		cfg.Mode = strings.TrimSpace(*dbMode)
	}
	if strings.TrimSpace(*dbContainer) != "" {
		cfg.Container = strings.TrimSpace(*dbContainer)
	}
	if strings.TrimSpace(*dbHost) != "" {
		cfg.Host = strings.TrimSpace(*dbHost)
	}
	if strings.TrimSpace(*dbPort) != "" {
		cfg.Port = strings.TrimSpace(*dbPort)
	}
	if strings.TrimSpace(*dbName) != "" {
		cfg.Database = strings.TrimSpace(*dbName)
	}
	if strings.TrimSpace(*dbUser) != "" {
		cfg.User = strings.TrimSpace(*dbUser)
	}
	if strings.TrimSpace(*dbPassword) != "" {
		cfg.Password = strings.TrimSpace(*dbPassword)
	}
	if strings.TrimSpace(*dbSSLMode) != "" {
		cfg.SSLMode = strings.TrimSpace(*dbSSLMode)
	}

	subcommand := fs.Arg(0)
	migrationsDir := strings.TrimSpace(*dir)
	if migrationsDir == "" {
		return fmt.Errorf("migration directory cannot be empty")
	}

	switch subcommand {
	case "create":
		if fs.NArg() < 2 {
			return fmt.Errorf("migration name is required for create")
		}
		name := fs.Arg(1)
		upPath, downPath, err := RunMigrateCreate(migrationsDir, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Created migration files:\n- %s\n- %s\n", upPath, downPath)
		return nil

	case "status":
		status, err := RunMigrateStatus(migrationsDir, cfg)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, status)
		return nil

	case "up":
		result, err := RunMigrateUp(migrationsDir, cfg, *steps)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, result)
		return nil

	case "down":
		result, err := RunMigrateDown(migrationsDir, cfg, *steps)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, result)
		return nil

	default:
		return fmt.Errorf("unknown migration subcommand %q", subcommand)
	}
}

func (a *App) loop(session *Session) error {
	reader := bufio.NewReader(a.in)
	// Reuse one buffered reader for prompts and command loop reads.
	a.in = reader

	for {
		fmt.Fprint(a.out, "\nodn> ")
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}

		input := strings.TrimSpace(line)
		if input == "" {
			if err == io.EOF {
				fmt.Fprintln(a.out)
				return nil
			}
			continue
		}

		switch input {
		case "/exit", "exit", "quit", "/quit":
			fmt.Fprintln(a.out, "Exiting odn.")
			_ = a.runLogger.Log("runtime", "exit_requested", nil)
			return a.store.Save(session)
		case "/help":
			a.printHelp()
			continue
		case "/status":
			a.printStatus(session)
			continue
		case "/history":
			a.printHistory(session)
			continue
		case "/clear":
			session.Messages = []Message{}
			session.Turns = []Turn{}
			if err := a.store.Save(session); err != nil {
				return err
			}
			fmt.Fprintln(a.out, "Session history cleared for this workspace.")
			_ = a.runLogger.Log("session", "history_cleared", nil)
			continue
		case "/mode":
			selected, err := PromptPermissionMode(a.in, a.out, session.Permission)
			if err != nil {
				return err
			}
			session.Permission = selected
			if err := a.store.Save(session); err != nil {
				return err
			}
			fmt.Fprintf(a.out, "Permission mode updated to %s\n", session.Permission)
			_ = a.runLogger.Log("session", "permission_mode_changed", map[string]interface{}{"mode": session.Permission})
			continue
		}
		if query, ok := researchCommandQuery(input); ok {
			activity := a.startTurnActivity(session)
			turn, assistantMessage, turnErr := a.handleResearchTurn(session, query)
			activity.Stop()
			if turnErr != nil {
				fmt.Fprintf(a.out, "[error] %v\n", turnErr)
				_ = a.runLogger.Log("runtime", "research_error", map[string]interface{}{"error": turnErr.Error()})
				continue
			}
			session.Turns = append(session.Turns, turn)
			session.Messages = append(session.Messages,
				Message{Role: "user", Content: input, CreatedAt: nowUTC()},
				Message{Role: "assistant", Content: assistantMessage, CreatedAt: nowUTC()},
			)
			if err := a.store.Save(session); err != nil {
				return err
			}
			a.printTimeline(turn.Events)
			fmt.Fprintf(a.out, "\nassistant> %s\n", assistantMessage)
			continue
		}

		liveTimeline := isInteractiveWriter(a.out)
		activity := a.startTurnActivity(session)
		turn, assistantMessage, err := a.handleTurn(session, input, activity)
		activity.Stop()
		if err != nil {
			fmt.Fprintf(a.out, "[error] %v\n", err)
			_ = a.runLogger.Log("runtime", "turn_error", map[string]interface{}{"error": err.Error()})
			continue
		}

		session.Turns = append(session.Turns, turn)
		session.Messages = append(session.Messages,
			Message{Role: "user", Content: input, CreatedAt: nowUTC()},
			Message{Role: "assistant", Content: assistantMessage, CreatedAt: nowUTC()},
		)

		if err := a.store.Save(session); err != nil {
			return err
		}

		if !liveTimeline {
			a.printTimeline(turn.Events)
		}
		fmt.Fprintf(a.out, "\nassistant> %s\n", assistantMessage)

		_ = a.runLogger.Log("turn", "turn_completed", map[string]interface{}{
			"turn_id":     turn.ID,
			"intent":      turn.IntentClassification,
			"confidence":  turn.Confidence,
			"event_count": len(turn.Events),
		})

		if err == io.EOF {
			return nil
		}
	}
}

func (a *App) handleTurn(session *Session, input string, activity *activityIndicator) (Turn, string, error) {
	if objective, ok := microQueueObjective(input); ok {
		return a.handleMicroQueueTurn(session, objective)
	}
	if objective, ok := managerObjective(input); ok {
		return a.handleManagerTurn(session, objective)
	}

	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	events := []Event{}
	liveTimeline := isInteractiveWriter(a.out)
	timelineStarted := false
	emitEvent := func(eventType, summary string, details map[string]string) {
		evt := a.newEvent(eventType, summary, details)
		events = append(events, evt)
		if !liveTimeline {
			return
		}
		activity.Pause()
		if !timelineStarted {
			fmt.Fprintln(a.out, "timeline>")
			timelineStarted = true
		}
		a.printTimelineEvent(evt)
		activity.Resume()
	}
	emitEvent("structured_command_started", "LLM structured command decision started", map[string]string{
		"permission_mode": string(session.Permission),
	})

	_ = a.runLogger.Log("structured_command", "turn_started", map[string]interface{}{
		"user_input":       input,
		"permission_mode":  session.Permission,
		"workspace":        session.WorkspacePath,
		"execution_policy": "stdin_prompt_llm_json_command_execute",
	})

	if activity == nil {
		activity = &activityIndicator{}
	}
	execCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	signalCtx, stopSignal := signal.NotifyContext(execCtx, os.Interrupt)
	defer stopSignal()
	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	result, execErr := RunStructuredCommandDecisionWithHistoryEventsAndAsk(
		signalCtx,
		input,
		session.Messages,
		a.ollama,
		&stdoutBuf,
		&stderrBuf,
		func(evt StructuredCommandEvent) {
			emitEvent(evt.Type, evt.Summary, evt.Details)
		},
		func(ctx context.Context, question string) (string, error) {
			activity.Pause()
			fmt.Fprintf(a.out, "\nassistant?> %s\nuser> ", question)
			answer, err := readLineFromReader(ctx, a.in)
			activity.Resume()
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(answer), nil
		},
	)
	cancel()

	assistantResponse := formatStructuredCommandChatResponse(result, stdoutBuf.String(), stderrBuf.String(), "")
	if execErr != nil {
		assistantResponse = formatStructuredCommandChatResponse(result, stdoutBuf.String(), stderrBuf.String(), execErr.Error())
		details := map[string]string{
			"error":     execErr.Error(),
			"command":   result.Command,
			"exit_code": fmt.Sprintf("%d", result.ExitCode),
			"stdout":    truncateOutput(stdoutBuf.String()),
			"stderr":    truncateOutput(stderrBuf.String()),
		}
		if isTransientStructuredLLMError(execErr) {
			details["diagnosis"] = classifyStructuredLLMFailure(execErr)
			details["mitigation"] = "Ollama backend failed before command completion; inspect journalctl -u ollama and consider CPU library mode."
		}
		emitEvent("structured_command_failed", "Structured command execution failed", details)
	} else {
		emitEvent("structured_command_completed", "Structured command executed", map[string]string{
			"command":   result.Command,
			"exit_code": fmt.Sprintf("%d", result.ExitCode),
			"stdout":    truncateOutput(stdoutBuf.String()),
			"stderr":    truncateOutput(stderrBuf.String()),
		})
	}

	turn := Turn{
		ID:                   turnID,
		UserInput:            input,
		IntentClassification: IntentExecution,
		Confidence:           1.0,
		ReasonCodes:          []string{"structured_llm_command"},
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

func formatStructuredCommandChatResponse(result CommandDecisionResult, stdout, stderr, errText string) string {
	lines := []string{
		"Command: " + result.Command,
		fmt.Sprintf("Exit code: %d", result.ExitCode),
	}
	if len(result.Observations) > 1 {
		lines = append(lines, fmt.Sprintf("Attempts: %d", len(result.Observations)))
	}
	if strings.TrimSpace(stdout) != "" {
		lines = append(lines, "Stdout: "+truncateOutput(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		lines = append(lines, "Stderr: "+truncateOutput(stderr))
	}
	if strings.TrimSpace(result.Answer) != "" {
		lines = append(lines, "Answer: "+result.Answer)
	}
	if strings.TrimSpace(errText) != "" {
		lines = append(lines, "Error: "+errText)
		if diagnosis := classifyStructuredLLMFailure(errors.New(errText)); diagnosis != "ollama_request_failure" {
			lines = append(lines, "Diagnosis: "+diagnosis)
		}
	}
	return strings.Join(lines, "\n")
}

func (a *App) startTurnActivity(session *Session) *activityIndicator {
	if session.Permission != PermissionFull || !isInteractiveWriter(a.out) {
		return &activityIndicator{}
	}
	return startActivityIndicator(a.out, "working")
}

func readLineFromReader(ctx context.Context, reader io.Reader) (string, error) {
	type stringReader interface {
		ReadString(delim byte) (string, error)
	}
	if sr, ok := reader.(stringReader); ok {
		return readLineWithContext(ctx, sr)
	}
	return readLineWithContext(ctx, bufio.NewReader(reader))
}

func readLineWithContext(ctx context.Context, reader interface {
	ReadString(delim byte) (string, error)
}) (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		done <- result{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-done:
		if res.err != nil && res.err != io.EOF {
			return "", res.err
		}
		return res.line, nil
	}
}

func (a *App) ollamaModelName() string {
	if a.ollama == nil {
		return "disabled"
	}
	return a.ollama.Model
}

func (a *App) planContextForTurn(ctx context.Context, input string) (ContextToolPlan, []Event) {
	plan, err := PlanContextTools(ctx, a.ollama, input)
	events := []Event{a.newEvent("context_plan_created", "Context tool plan created", map[string]string{
		"tools":  strings.Join(plan.Tools, ","),
		"reason": plan.Reason,
	})}
	if err != nil {
		events = append(events, a.newEvent("context_plan_failed", "Context tool planner fell back to default", map[string]string{"error": err.Error()}))
	}
	return plan, events
}

func (a *App) autoResearchForTurn(ctx context.Context, input string, plan ContextToolPlan) ([]Event, *CommandObservation) {
	if !plan.NeedsWebResearch {
		return nil, nil
	}
	query := strings.TrimSpace(input)
	if query == "" {
		return nil, nil
	}
	events := []Event{}
	events = append(events, a.newEvent("auto_research_started", "Automatic web research started", map[string]string{"query": query}))
	if a.web == nil {
		events = append(events, a.newEvent("auto_research_skipped", "Web search service is unavailable", nil))
		return events, nil
	}
	searchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	results, err := a.web.SearchAll(searchCtx, query)
	if err != nil {
		events = append(events, a.newEvent("auto_research_failed", "Automatic web research failed", map[string]string{"error": err.Error()}))
		return events, nil
	}
	contextText := websearch.BuildContext(results, 5000)
	if strings.TrimSpace(contextText) == "" {
		events = append(events, a.newEvent("auto_research_failed", "Automatic web research returned empty context", nil))
		return events, nil
	}
	events = append(events, a.newEvent("auto_research_completed", "Automatic web research context captured", map[string]string{
		"query":   query,
		"results": fmt.Sprintf("%d", len(results)),
	}))

	if a.memory != nil {
		if result, storeErr := ResearchWebToMemory(ctx, query, staticWebSearchResults{results: results}, a.memory, WebResearchMemoryConfig{
			AgentID: "odn_auto_researcher",
			Tags:    researchTagsFromQuery(query),
		}); storeErr != nil {
			events = append(events, a.newEvent("auto_research_memory_failed", "Automatic research memory write failed", map[string]string{"error": storeErr.Error()}))
		} else {
			events = append(events, a.newEvent("auto_research_memory_stored", "Automatic research stored in Postgres memory", map[string]string{
				"stored": fmt.Sprintf("%d", result.StoredCount),
			}))
		}
	}

	return events, &CommandObservation{
		Step:    0,
		Command: "AUTO_RESEARCH: " + query,
		Status:  "success",
		Stdout:  truncateForObservation(contextText, defaultAgentObservationChars),
	}
}

type staticWebSearchResults struct {
	results []websearch.Result
}

func (s staticWebSearchResults) SearchAll(ctx context.Context, query string) ([]websearch.Result, error) {
	return s.results, nil
}

func (a *App) handleManagerTurn(session *Session, objective string) (Turn, string, error) {
	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	events := []Event{a.newEvent("manager_started", "Manager-worker job started", map[string]string{
		"permission_mode": string(session.Permission),
	})}

	_ = a.runLogger.Log("manager", "turn_started", map[string]interface{}{
		"objective":       objective,
		"permission_mode": session.Permission,
		"workspace":       session.WorkspacePath,
	})

	execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	result, execErr := ExecuteManagerWorkerJob(execCtx, session, objective, session.Permission, a.in, a.out, a.ollama, a.nextEventID, a.runLogger)
	cancel()
	events = append(events, result.Events...)

	assistantResponse := result.Summary
	if execErr != nil {
		assistantResponse = fmt.Sprintf("Manager execution failed: %v", execErr)
		events = append(events, a.newEvent("manager_failed", "Manager-worker job terminated with error", map[string]string{"error": execErr.Error()}))
	} else {
		events = append(events, a.newEvent("manager_completed", "Manager-worker job completed", map[string]string{
			"done":     fmt.Sprintf("%t", result.Done),
			"tasks":    fmt.Sprintf("%d", len(result.Tasks)),
			"workers":  fmt.Sprintf("%d", len(result.Workers)),
			"executed": fmt.Sprintf("%d", result.Executed),
			"blocked":  fmt.Sprintf("%d", result.Blocked),
			"failed":   fmt.Sprintf("%d", result.Failed),
		}))
	}

	turn := Turn{
		ID:                   turnID,
		UserInput:            "/manage " + objective,
		IntentClassification: IntentExecution,
		Confidence:           1.0,
		ReasonCodes:          []string{"manager_worker_job"},
		Response:             assistantResponse,
		Events:               events,
		CreatedAt:            nowUTC(),
	}
	return turn, assistantResponse, nil
}

func (a *App) handleResearchTurn(session *Session, query string) (Turn, string, error) {
	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	events := []Event{a.newEvent("research_started", "Web research memory job started", map[string]string{"query": query})}
	if a.web == nil {
		events = append(events, a.newEvent("research_blocked", "Web search service is unavailable", nil))
		return Turn{}, "", fmt.Errorf("web search service is unavailable")
	}
	if a.memory == nil {
		events = append(events, a.newEvent("research_blocked", "Postgres memory is not configured", map[string]string{
			"hint": "set --memory-database-url or ODN_MEMORY_DATABASE_URL",
		}))
		turn := Turn{
			ID:                   turnID,
			UserInput:            "/research " + query,
			IntentClassification: IntentExecution,
			Confidence:           1.0,
			ReasonCodes:          []string{"web_research_memory"},
			Response:             "Research blocked: Postgres memory is not configured. Set --memory-database-url or ODN_MEMORY_DATABASE_URL.",
			Events:               events,
			CreatedAt:            nowUTC(),
		}
		return turn, turn.Response, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := ResearchWebToMemory(ctx, query, a.web, a.memory, WebResearchMemoryConfig{
		AgentID: "odn_research_manager",
		Tags:    researchTagsFromQuery(query),
	})
	if err != nil {
		events = append(events, a.newEvent("research_failed", "Web research memory job failed", map[string]string{"error": err.Error()}))
		return Turn{}, "", err
	}
	events = append(events, a.newEvent("research_completed", "Web research stored in Postgres memory", map[string]string{
		"query":        query,
		"results":      fmt.Sprintf("%d", len(result.Results)),
		"stored":       fmt.Sprintf("%d", result.StoredCount),
		"stored_agent": "odn_research_manager",
	}))
	response := fmt.Sprintf("Stored %d web research memory chunk(s) from %d search result(s) for: %s", result.StoredCount, len(result.Results), query)
	turn := Turn{
		ID:                   turnID,
		UserInput:            "/research " + query,
		IntentClassification: IntentExecution,
		Confidence:           1.0,
		ReasonCodes:          []string{"web_research_memory"},
		Response:             response,
		Events:               events,
		CreatedAt:            nowUTC(),
	}
	return turn, response, nil
}

func researchCommandQuery(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	for _, prefix := range []string{"/research "} {
		if strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			query := strings.TrimSpace(trimmed[len(prefix):])
			return query, query != ""
		}
	}
	return "", false
}

func researchTagsFromQuery(query string) []string {
	parts := strings.Fields(strings.ToLower(query))
	tags := []string{"web-research"}
	for _, part := range parts {
		clean := strings.Trim(part, ".,;:!?()[]{}\"'")
		if len(clean) >= 4 {
			tags = append(tags, clean)
		}
		if len(tags) >= 8 {
			break
		}
	}
	return cleanMemoryTags(tags)
}

func managerObjective(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	for _, prefix := range []string{"/manage ", "/job "} {
		if strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			objective := strings.TrimSpace(trimmed[len(prefix):])
			return objective, objective != ""
		}
	}
	return "", false
}

func microQueueObjective(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	for _, prefix := range []string{"/micro ", "/queue "} {
		if strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			objective := strings.TrimSpace(trimmed[len(prefix):])
			return objective, objective != ""
		}
	}
	return "", false
}

func formatMicroQueueResponse(result MicroJobQueueResult, stdout, stderr, errText string) string {
	lines := []string{
		result.Summary,
		fmt.Sprintf("Jobs: %d", len(result.Jobs)),
		fmt.Sprintf("Completed: %d", len(result.Results)),
		fmt.Sprintf("Done: %t", result.Done),
	}
	if strings.TrimSpace(result.ProjectProfile.Summary) != "" {
		lines = append(lines, "Profile: "+result.ProjectProfile.Summary)
	}
	if strings.TrimSpace(stdout) != "" {
		lines = append(lines, "Stdout: "+truncateOutput(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		lines = append(lines, "Stderr: "+truncateOutput(stderr))
	}
	if strings.TrimSpace(errText) != "" {
		lines = append(lines, "Error: "+errText)
	}
	return strings.Join(lines, "\n")
}

func (a *App) conversationReply(session *Session, input string) (string, string) {
	if a.ollama == nil {
		return "Conversation mode: understood. Share what you want to explore, and I’ll keep it in planning/discussion mode.", "local_fallback"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	messages := make([]OllamaMessage, 0, maxConversationHistoryMessages+2)
	messages = append(messages, OllamaMessage{
		Role:    "system",
		Content: MinimalOutputContract + " Practical. Current workspace: " + session.WorkspacePath + ". Use conversation history to answer follow-up, reflection, and recall questions before asking the user to repeat context.",
	})

	start := 0
	if len(session.Messages) > maxConversationHistoryMessages {
		start = len(session.Messages) - maxConversationHistoryMessages
	}
	for _, msg := range session.Messages[start:] {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		messages = append(messages, OllamaMessage{Role: msg.Role, Content: msg.Content})
	}
	messages = append(messages, OllamaMessage{Role: "user", Content: input})

	resp, err := a.ollama.ChatRaw(ctx, OllamaChatRequest{
		Messages: messages,
		Options:  map[string]interface{}{"temperature": 0.25},
	})
	if err != nil {
		_ = a.runLogger.Log("conversation", "llm_error", map[string]interface{}{"error": err.Error()})
		return "Conversation mode: understood. (Ollama unavailable right now, continuing with local fallback.)", "local_fallback"
	}

	_ = a.runLogger.Log("conversation", "llm_call", map[string]interface{}{
		"request":           resp.RequestJSON,
		"response":          resp.ResponseJSON,
		"total_duration_ns": resp.TotalDuration,
		"prompt_eval_count": resp.PromptEvalCount,
		"eval_count":        resp.EvalCount,
	})

	return resp.Content, "ollama"
}

func (a *App) printBanner(session *Session, loaded bool, noOllama bool) {
	fmt.Fprintln(a.out, "\n========================================")
	fmt.Fprintln(a.out, "OmnidexNeo (odn) - deterministic core")
	fmt.Fprintln(a.out, "========================================")
	fmt.Fprintf(a.out, "Workspace: %s\n", session.WorkspacePath)
	fmt.Fprintf(a.out, "Session ID: %s\n", session.WorkspaceHash)
	fmt.Fprintf(a.out, "Permission: %s\n", session.Permission)
	if noOllama {
		fmt.Fprintln(a.out, "Conversation model: disabled")
	} else if a.ollama != nil {
		fmt.Fprintf(a.out, "Conversation model: %s\n", a.ollama.Model)
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
	fmt.Fprintln(a.out, "  /help      show commands")
	fmt.Fprintln(a.out, "  /status    show current workspace/session status")
	fmt.Fprintln(a.out, "  /history   show recent turns")
	fmt.Fprintln(a.out, "  /mode      change permission mode")
	fmt.Fprintln(a.out, "  /clear     clear workspace session history")
	fmt.Fprintln(a.out, "  /research X search web, fetch pages, store chunks in Postgres memory")
	fmt.Fprintln(a.out, "  /manage X  run X through manager-worker orchestration")
	fmt.Fprintln(a.out, "  /job X     alias for /manage X")
	fmt.Fprintln(a.out, "  /micro X   run X through project-profiled micro job queue")
	fmt.Fprintln(a.out, "  /queue X   alias for /micro X")
	fmt.Fprintln(a.out, "  /exit      exit")
}

func (a *App) printStatus(session *Session) {
	fmt.Fprintln(a.out, "")
	fmt.Fprintf(a.out, "Workspace: %s\n", session.WorkspacePath)
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
	fmt.Fprintln(a.out, "timeline>")
	for _, evt := range events {
		a.printTimelineEvent(evt)
	}
}

func (a *App) printTimelineEvent(evt Event) {
	fmt.Fprintf(a.out, "  - [%s] %s: %s\n", evt.CreatedAt, evt.Type, evt.Summary)
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
		if shouldTruncateTimelineValue(value) {
			value = value[:400] + "..."
		}
		fmt.Fprintf(a.out, "      %s=%s\n", k, value)
	}
}

func shouldTruncateTimelineValue(v string) bool {
	return len(v) > 400
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
