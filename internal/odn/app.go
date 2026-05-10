package odn

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type App struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer

	store    SessionStore
	ollama   *OllamaClient
	registry Registry

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
	if len(args) > 0 && args[0] == "chat" {
		args = args[1:]
	}

	fs := flag.NewFlagSet("odn", flag.ContinueOnError)
	fs.SetOutput(a.errOut)

	permissionFlag := fs.String("permission", "", "permission mode: ask_permission|full_access")
	modelFlag := fs.String("model", defaultOllamaModel, "ollama model to use for conversation + routing")
	endpointFlag := fs.String("ollama-endpoint", defaultOllamaEndpoint, "ollama chat endpoint")
	noOllama := fs.Bool("no-ollama", false, "disable ollama calls")
	sessionRoot := fs.String("session-root", "", "override session root directory")
	runLogRoot := fs.String("run-log-root", "", "override run log root directory")
	skipPermissionPrompt := fs.Bool("no-permission-prompt", false, "skip startup permission prompt and keep current/default mode")

	fs.Usage = func() {
		fmt.Fprintln(a.errOut, "Usage: odn [chat] [flags]")
		fmt.Fprintln(a.errOut, "")
		fmt.Fprintln(a.errOut, "Commands:")
		fmt.Fprintln(a.errOut, "  odn          start chat loop")
		fmt.Fprintln(a.errOut, "  odn chat     start chat loop")
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

	if !*noOllama {
		a.ollama = NewOllamaClient(*endpointFlag, *modelFlag)
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

		turn, assistantMessage, err := a.handleTurn(session, input)
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

		fmt.Fprintf(a.out, "\nassistant> %s\n", assistantMessage)
		a.printTimeline(turn.Events)

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

func (a *App) handleTurn(session *Session, input string) (Turn, string, error) {
	intent := ClassifyIntent(input)
	events := []Event{a.newEvent("intent_classified", "Intent gate completed", map[string]string{
		"intent_classification": string(intent.Classification),
		"confidence":            fmt.Sprintf("%.2f", intent.Confidence),
		"reason_codes":          strings.Join(intent.ReasonCodes, ","),
	})}

	_ = a.runLogger.Log("intent", "intent_classified", map[string]interface{}{
		"classification": intent.Classification,
		"confidence":     intent.Confidence,
		"reason_codes":   strings.Join(intent.ReasonCodes, ","),
		"user_input":     input,
	})

	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	assistantResponse := ""

	switch intent.Classification {
	case IntentConversation:
		message, mode := a.conversationReply(session, input)
		assistantResponse = message
		events = append(events, a.newEvent("conversation_completed", "Conversation response generated", map[string]string{
			"response_mode": mode,
		}))
	case IntentAmbiguous:
		assistantResponse = "I’m not fully sure whether you want discussion or execution. Clarify your goal, or say: do it."
		events = append(events, a.newEvent("clarification_required", "Ambiguous intent blocked execution", nil))
	case IntentExecution:
		execCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		response, execEvents, execErr := ExecuteDeterministicPipeline(execCtx, session, input, session.Permission, a.in, a.out, a.registry, a.ollama, a.nextEventID, a.runLogger)
		cancel()
		events = append(events, execEvents...)
		if execErr != nil {
			assistantResponse = fmt.Sprintf("Execution failed: %v", execErr)
			events = append(events, a.newEvent("execution_failed", "Execution terminated with error", map[string]string{"error": execErr.Error()}))
			break
		}
		assistantResponse = response
		events = append(events, a.newEvent("execution_completed", "Execution pipeline completed", nil))
	}

	turn := Turn{
		ID:                   turnID,
		UserInput:            input,
		IntentClassification: intent.Classification,
		Confidence:           intent.Confidence,
		ReasonCodes:          sortedCopy(intent.ReasonCodes),
		Response:             assistantResponse,
		Events:               events,
		CreatedAt:            nowUTC(),
	}

	return turn, assistantResponse, nil
}

func (a *App) conversationReply(session *Session, input string) (string, string) {
	if a.ollama == nil {
		return "Conversation mode: understood. Share what you want to explore, and I’ll keep it in planning/discussion mode.", "local_fallback"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	messages := make([]OllamaMessage, 0, maxConversationHistoryMessages+2)
	messages = append(messages, OllamaMessage{
		Role: "system",
		Content: "You are OmnidexNeo. Keep responses concise and practical. " +
			"Current workspace: " + session.WorkspacePath + ".",
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
	}
	if a.runLogger != nil {
		fmt.Fprintf(a.out, "Run log: %s\n", a.runLogger.Path())
	}
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
		fmt.Fprintf(a.out, "  - [%s] %s: %s\n", evt.CreatedAt, evt.Type, evt.Summary)
		if len(evt.Details) == 0 {
			continue
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
