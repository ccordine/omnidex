package omni

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/version"
	"github.com/gryph/omnidex/internal/websearch"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer

	store              SessionStore
	ollama             *OllamaClient
	planner            *OllamaClient
	plannerClient      CommandDecisionClient
	promptInterpreter  PromptInterpreter
	promptTagger       PromptTagger
	contextSummarizer  ContextSummarizer
	completionChecker  CompletionChecker
	evaluator          StructuredLLMResponseEvaluator
	evaluatorThreshold int
	shellSpecialist    ShellCommandSpecialist
	codeSpecialist     CodeContentSpecialist
	thinkingService    ThinkingService
	cursorArchitect    CursorArchitectAgent
	codexArchitect     CursorArchitectAgent
	recipes            []Recipe
	enableCommandCache bool
	commandCacheRoot   string
	registry           Registry
	memory             *PGMemoryStore
	pgPool             *pgxpool.Pool
	web                WebSearchService

	runLogger *RunLogger

	eventSequence int
	terminalIn    *os.File
}

func NewApp(in io.Reader, out, errOut io.Writer) *App {
	app := &App{in: in, out: out, errOut: errOut, registry: DefaultRegistry()}
	if file, ok := in.(*os.File); ok {
		app.terminalIn = file
	}
	return app
}

func (a *App) Run(args []string) error {
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		if len(args) > 1 && args[1] == "--json" {
			encoded, err := json.MarshalIndent(version.JSON(), "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(a.out, string(encoded))
			return nil
		}
		fmt.Fprintln(a.out, version.PrintName("omni"))
		return nil
	}
	if len(args) > 0 && args[0] == "update" {
		return a.runUpdate(args[1:])
	}
	if len(args) > 0 && args[0] == "migrate" {
		return a.runMigrate(args[1:])
	}
	if len(args) > 0 && args[0] == "ledger" {
		return a.runLedger(args[1:])
	}
	if len(args) > 0 && args[0] == "bench" {
		return a.runBench(args[1:])
	}
	if len(args) > 0 && args[0] == "run:trace" {
		return a.runTrace(args[1:])
	}
	if len(args) > 0 && args[0] == "fastpath" {
		return a.runFastPath(args[1:])
	}
	if len(args) > 0 && args[0] == "index" {
		return a.runIndex(args[1:])
	}
	if len(args) > 0 && args[0] == "map" {
		return a.runCodebaseMap(args[1:])
	}
	if len(args) > 0 && args[0] == "fingerprint" {
		return a.runFingerprint(args[1:])
	}
	if len(args) > 0 && args[0] == "patch" {
		return a.runPatch(args[1:])
	}
	if len(args) > 0 && args[0] == "ollama" {
		return a.runOllama(args[1:])
	}
	if len(args) > 0 && args[0] == "host" {
		return a.runHost(args[1:])
	}
	if len(args) > 0 && args[0] == "agent" {
		return a.runAgentMode(args[1:])
	}
	strictOneShot := false
	if len(args) > 0 && args[0] == "run" {
		strictOneShot = true
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "chat" {
		args = args[1:]
	}

	fs := flag.NewFlagSet("omni", flag.ContinueOnError)
	fs.SetOutput(a.errOut)

	permissionFlag := fs.String("permission", "", "permission mode: ask_permission|full_access")
	modelFlag := fs.String("model", firstNonEmpty(os.Getenv("OMNI_MODEL"), os.Getenv("OMNI_CONVERSATION_MODEL"), os.Getenv("OLLAMA_MODEL_RESPONDER"), os.Getenv("OLLAMA_MODEL"), defaultOllamaModel), "ollama model to use for conversation responses")
	plannerModel := fs.String("planner-model", firstNonEmpty(os.Getenv("OMNI_PLANNER_MODEL"), os.Getenv("OMNI_STRUCTURED_PLANNER_MODEL"), os.Getenv("OLLAMA_MODEL_PLANNER"), defaultOllamaPlannerModel), "ollama model for structured command planning")
	endpointFlag := fs.String("ollama-endpoint", defaultOllamaEndpoint, "ollama chat endpoint")
	ollamaKeepAlive := fs.String("ollama-keep-alive", envOrDefault("OMNI_OLLAMA_KEEP_ALIVE", "30s"), "default Ollama keep_alive for chat requests; use 0 to unload after each response")
	ollamaNumCtx := fs.Int("ollama-num-ctx", envIntOrDefault("OMNI_OLLAMA_NUM_CTX", 2048), "default Ollama num_ctx option; set 0 to use Ollama default")
	plannerNumCtx := fs.Int("planner-num-ctx", envIntOrDefault("OMNI_PLANNER_NUM_CTX", envIntOrDefault("OMNI_OLLAMA_NUM_CTX", 4096)), "Ollama num_ctx option for structured planner requests; set 0 to use Ollama default")
	evaluatorModel := fs.String("evaluator-model", firstNonEmpty(os.Getenv("OMNI_EVALUATOR_MODEL"), os.Getenv("OLLAMA_MODEL_EVALUATOR"), defaultOllamaEvaluatorModel), "ollama model for structured response evaluator")
	evaluatorThreshold := fs.Int("evaluator-threshold", envIntOrDefault("OMNI_EVALUATOR_THRESHOLD", defaultEvaluatorThreshold), "minimum evaluator confidence 0..100 before planner output is accepted")
	evaluatorNumCtx := fs.Int("evaluator-num-ctx", envIntOrDefault("OMNI_EVALUATOR_NUM_CTX", 2048), "Ollama num_ctx option for evaluator requests; set 0 to use Ollama default")
	disableEvaluator := fs.Bool("disable-evaluator", envBoolOrDefault("OMNI_DISABLE_EVALUATOR", false), "disable structured response self-evaluator")
	shellSpecialistModel := fs.String("shell-specialist-model", firstNonEmpty(os.Getenv("OMNI_SHELL_SPECIALIST_MODEL"), os.Getenv("OLLAMA_MODEL_SPECIALIST_SHELL_EXECUTION"), os.Getenv("OLLAMA_MODEL_SHELL"), defaultOllamaModel), "ollama model for shell execution specialist")
	shellSpecialistNumCtx := fs.Int("shell-specialist-num-ctx", envIntOrDefault("OMNI_SHELL_SPECIALIST_NUM_CTX", 2048), "Ollama num_ctx option for shell specialist requests; set 0 to use Ollama default")
	disableShellSpecialist := fs.Bool("disable-shell-specialist", envBoolOrDefault("OMNI_DISABLE_SHELL_SPECIALIST", false), "disable delegated shell execution specialist")
	thinkingModel := fs.String("thinking-model", thinkingModelFromEnv(), "ollama model for internal thinking/pilot channel")
	thinkingNumCtx := fs.Int("thinking-num-ctx", thinkingNumCtxFromEnv(), "Ollama num_ctx for thinking channel; set 0 to use Ollama default")
	thinkingMaxSteps := fs.Int("thinking-max-steps", thinkingMaxStepsFromEnv(), "maximum internal reasoning steps per thought channel")
	disableThinking := fs.Bool("disable-thinking", envBoolOrDefault("OMNI_DISABLE_THINKING", false), "disable internal thinking/pilot channel")
	noOllama := fs.Bool("no-ollama", false, "disable ollama calls")
	sessionRoot := fs.String("session-root", "", "override session root directory")
	runLogRoot := fs.String("run-log-root", "", "override run log root directory")
	memoryDatabaseURL := fs.String("memory-database-url", "", "Postgres URL for /research memory storage")
	recipeRoot := fs.String("recipe-root", envOrDefault("OMNI_RECIPE_ROOT", "recipes"), "recipe manifest root; missing roots are ignored")
	enableCommandCache := fs.Bool("enable-command-cache", envBoolOrDefault("OMNI_ENABLE_COMMAND_CACHE", false), "reuse eligible command results when workspace inputs are unchanged")
	commandCacheRoot := fs.String("command-cache-root", os.Getenv("OMNI_COMMAND_CACHE_ROOT"), "command cache root; defaults to .omni/command-cache in the workspace")
	skipPermissionPrompt := fs.Bool("no-permission-prompt", false, "skip startup permission prompt and keep current/default mode")

	fs.Usage = func() {
		fmt.Fprintln(a.errOut, "Usage: omni [chat|run] [flags]")
		fmt.Fprintln(a.errOut, "")
		fmt.Fprintln(a.errOut, "Commands:")
		fmt.Fprintln(a.errOut, "  omni          start chat when interactive; run one-shot when stdin is piped")
		fmt.Fprintln(a.errOut, "  omni chat     start interactive chat")
		fmt.Fprintln(a.errOut, "  omni run      strict stdin -> LLM JSON command -> execute")
		fmt.Fprintln(a.errOut, "  omni update   run the managed update.sh for this install")
		fmt.Fprintln(a.errOut, "  omni migrate  run migration commands")
		fmt.Fprintln(a.errOut, "  omni ledger   export evidence ledgers")
		fmt.Fprintln(a.errOut, "  omni bench    list benchmark manifests and report session metrics")
		fmt.Fprintln(a.errOut, "  omni run:trace latest run telemetry for this workspace")
		fmt.Fprintln(a.errOut, "  omni fastpath run explicit deterministic probes")
		fmt.Fprintln(a.errOut, "  omni index    build deterministic workspace index")
		fmt.Fprintln(a.errOut, "  omni map      build, update, query, or route codebase maps")
		fmt.Fprintln(a.errOut, "  omni fingerprint classify failure output")
		fmt.Fprintln(a.errOut, "  omni patch    inspect or apply unified diffs")
		fmt.Fprintln(a.errOut, "  omni ollama   prewarm/profile local model calls")
		fmt.Fprintln(a.errOut, "  omni host     host bridge for native directory picker + browse")
		fmt.Fprintln(a.errOut, "  omni agent    core-backed interactive agent chat (Cursor/Codex/Omnidex switchable)")
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
	a.recipes = loadOptionalRecipes(*recipeRoot)
	a.enableCommandCache = *enableCommandCache
	a.commandCacheRoot = *commandCacheRoot
	if cursorArchitect := NewCursorSDKArchitectAgentFromEnv(); cursorArchitect != nil {
		a.cursorArchitect = cursorArchitect
	}
	if codexArchitect := NewCodexSDKArchitectAgentFromEnv(); codexArchitect != nil {
		a.codexArchitect = codexArchitect
	}

	if !*noOllama {
		a.ollama = NewOllamaClient(*endpointFlag, *modelFlag)
		a.ollama.ConfigureRuntime(*ollamaKeepAlive, *ollamaNumCtx)
		a.planner = NewOllamaClient(*endpointFlag, *plannerModel)
		a.planner.ConfigureRuntime(*ollamaKeepAlive, *plannerNumCtx)
		a.promptInterpreter = NewOllamaPromptInterpreter(a.planner)
		a.promptTagger = NewOllamaPromptTagger(a.planner)
		a.contextSummarizer = NewOllamaContextSummarizer(a.planner)
		a.completionChecker = NewOllamaCompletionChecker(a.planner)
		a.evaluatorThreshold = normalizeStructuredEvaluatorThreshold(*evaluatorThreshold)
		if !*disableEvaluator {
			evaluatorClient := NewOllamaClient(*endpointFlag, *evaluatorModel)
			evaluatorClient.ConfigureRuntime(*ollamaKeepAlive, *evaluatorNumCtx)
			a.evaluator = NewOllamaStructuredResponseEvaluator(evaluatorClient)
		}
		if !*disableShellSpecialist {
			shellClient := NewOllamaClient(*endpointFlag, *shellSpecialistModel)
			shellClient.ConfigureRuntime(*ollamaKeepAlive, *shellSpecialistNumCtx)
			a.shellSpecialist = NewOllamaShellCommandSpecialist(shellClient)
			a.codeSpecialist = NewOllamaCodeContentSpecialist(shellClient)
		}
		if !*disableThinking {
			thinkingClient := NewOllamaClient(*endpointFlag, *thinkingModel)
			thinkingClient.ConfigureRuntime(*ollamaKeepAlive, *thinkingNumCtx)
			a.thinkingService = NewOllamaThinkingService(thinkingClient, nil, *thinkingMaxSteps)
		}
	}

	a.web = websearch.New([]string{"duckduckgo", "yahoo", "google"}, 20*time.Second, 3000, 7000)
	if dbURL := firstNonEmpty(*memoryDatabaseURL, os.Getenv("OMNI_MEMORY_DATABASE_URL"), os.Getenv("DATABASE_URL")); dbURL != "" {
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

	if strictOneShot || !isInteractive(a.in) {
		promptBytes, err := io.ReadAll(a.in)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		timelineStarted := false
		emitOneShotEvent := func(evt StructuredCommandEvent) {
			if !timelineStarted {
				fmt.Fprintln(a.out, "\ntimeline")
				fmt.Fprintln(a.out, "--------")
				timelineStarted = true
			}
			a.printTimelineEvent(a.newEvent(evt.Type, evt.Summary, evt.Details))
		}
		emitOneShotPrepEvent := func(eventType, summary string, details map[string]string) {
			emitOneShotEvent(StructuredCommandEvent{Type: eventType, Summary: summary, Details: details})
		}
		activeDirectory := workspacePathOrCurrentDir()
		prepCtx := a.prepareInteractiveTurnContext(context.Background(), string(promptBytes), activeDirectory, emitOneShotPrepEvent)
		input := string(promptBytes)
		turnID := "oneshot_001"
		execPrompt := input
		thinkingService := a.thinkingService
		if thinkingService != nil {
			if thoughtStore, _ := NewThoughtChannelStore("", workspaceHash(activeDirectory), turnID); thoughtStore != nil {
				if svc, ok := thinkingService.(OllamaThinkingService); ok {
					svc.Store = thoughtStore
					svc.Deps = a.buildThinkingToolDeps()
					thinkingService = svc
				}
			}
			emitOneShotEvent(StructuredCommandEvent{Type: "thinking_pilot_started", Summary: "Thinking pilot is the turn entry point", Details: map[string]string{"turn_id": turnID}})
			pilotOutcome, pilotErr := thinkingService.OrchestrateTurn(context.Background(), ThinkingInput{
				TurnID:          turnID,
				Trigger:         "turn_entry",
				UserPrompt:      input,
				WorkingDir:      activeDirectory,
				SessionMemories: prepCtx.SessionMemories,
				PrepContext:     prepCtx.Bundle,
				ActivePrompt:    NewActivePromptContext(input, "", explicitReactAppAcceptanceCriteria(input, "")),
			}, emitOneShotEvent)
			if pilotErr != nil {
				emitOneShotEvent(StructuredCommandEvent{Type: "thinking_pilot_failed", Summary: "Thinking pilot failed; falling back to execution layer", Details: map[string]string{"error": truncateOutput(pilotErr.Error())}})
			} else {
				emitOneShotEvent(StructuredCommandEvent{Type: "thinking_pilot_decision", Summary: "Thinking pilot decided next action", Details: map[string]string{
					"action":              string(pilotOutcome.Action),
					"channel_id":          pilotOutcome.ChannelID,
					"execution_prompt":    truncateOutput(pilotOutcome.ExecutionPrompt),
					"execution_tool_task": truncateOutput(pilotOutcome.ExecutionToolTask),
				}})
				if pilotOutcome.Action == ThinkingActionDirectAnswer {
					answer := strings.TrimSpace(pilotOutcome.DirectAnswer)
					fmt.Fprintln(a.out, answer)
					return nil
				}
				if strings.TrimSpace(pilotOutcome.ExecutionPrompt) != "" {
					execPrompt = strings.TrimSpace(pilotOutcome.ExecutionPrompt)
				}
				if task := strings.TrimSpace(pilotOutcome.ExecutionToolTask); task != "" {
					execPrompt = strings.TrimSpace(execPrompt + "\n\nPilot execution scope: " + task)
				}
			}
		}
		_, err = runStructuredCommandDecisionWithConfig(
			context.Background(),
			execPrompt,
			nil,
			a.structuredPlannerClient(),
			a.out,
			a.errOut,
			emitOneShotEvent,
			nil,
			structuredCommandDecisionRunConfig{
				SessionMemories:         prepCtx.SessionMemories,
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
				EnableCommandCache:      *enableCommandCache,
				CommandCacheRoot:        *commandCacheRoot,
				ThinkingService:         thinkingService,
				ThoughtTurnID:           turnID,
			},
		)
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

	a.runLogger, err = NewRunLogger(*runLogRoot, session.WorkspaceHash)
	if err != nil {
		return err
	}
	defer func() {
		_ = a.runLogger.Close()
	}()

	_ = a.runLogger.Log("runtime", "app_initialized", map[string]interface{}{
		"workspace":                workspacePath,
		"permission_mode":          session.Permission,
		"ollama_enabled":           !*noOllama,
		"model":                    *modelFlag,
		"planner_model":            *plannerModel,
		"endpoint":                 *endpointFlag,
		"evaluator_model":          *evaluatorModel,
		"evaluator_threshold":      normalizeStructuredEvaluatorThreshold(*evaluatorThreshold),
		"evaluator_enabled":        !*disableEvaluator && !*noOllama,
		"shell_specialist_model":   *shellSpecialistModel,
		"shell_specialist_enabled": !*disableShellSpecialist && !*noOllama,
		"loaded_session":           loaded,
	})

	a.printBanner(session, loaded, *noOllama)
	return a.loop(session)
}

func (a *App) runMigrate(args []string) error {
	fs := flag.NewFlagSet("omni migrate", flag.ContinueOnError)
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
		fmt.Fprintln(a.errOut, "Usage: omni migrate <create|up|down|status> [args] [flags]")
		fmt.Fprintln(a.errOut, "")
		fmt.Fprintln(a.errOut, "Examples:")
		fmt.Fprintln(a.errOut, "  omni migrate create create_runs_table")
		fmt.Fprintln(a.errOut, "  omni migrate up --steps 2")
		fmt.Fprintln(a.errOut, "  omni migrate down --steps 1")
		fmt.Fprintln(a.errOut, "  omni migrate status")
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
		fmt.Fprint(a.out, "\nomni> ")
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
			fmt.Fprintln(a.out, "Exiting omni.")
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
			session.Memories = []SessionMemory{}
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

		if cmd, ok := parseChatSlashCommand(input); ok {
			switch cmd.Kind {
			case chatSlashUsageError:
				fmt.Fprintln(a.out, cmd.Args)
				continue
			case chatSlashThoughts:
				text, thoughtsErr := a.handleThoughtsCommand(session, cmd.Args)
				if thoughtsErr != nil {
					fmt.Fprintf(a.out, "[error] %v\n", thoughtsErr)
					continue
				}
				fmt.Fprintf(a.out, "\n%s\n", text)
				continue
			case chatSlashResearch:
				if err := a.completeSlashTurn(session, input, func() (Turn, string, error) {
					return a.handleResearchTurn(session, cmd.Args)
				}); err != nil {
					fmt.Fprintf(a.out, "[error] %v\n", err)
				}
				continue
			case chatSlashSearch:
				if err := a.completeSlashTurn(session, input, func() (Turn, string, error) {
					return a.handleSearchTurn(session, cmd.Args)
				}); err != nil {
					fmt.Fprintf(a.out, "[error] %v\n", err)
				}
				continue
			case chatSlashManage:
				if err := a.completeSlashTurn(session, input, func() (Turn, string, error) {
					return a.handleManagerTurn(session, cmd.Args)
				}); err != nil {
					fmt.Fprintf(a.out, "[error] %v\n", err)
				}
				continue
			case chatSlashMicro:
				if err := a.completeSlashTurn(session, input, func() (Turn, string, error) {
					return a.handleMicroQueueTurn(session, cmd.Args)
				}); err != nil {
					fmt.Fprintf(a.out, "[error] %v\n", err)
				}
				continue
			case chatSlashTurn:
				liveTimeline := isLiveTimelineWriter(a.out)
				activity := a.startTurnActivity(session)
				turn, assistantMessage, turnErr := a.handleTurn(session, input, activity, cmd.Turn)
				activity.Stop()
				if turnErr != nil {
					fmt.Fprintf(a.out, "[error] %v\n", turnErr)
					_ = a.runLogger.Log("runtime", "turn_error", map[string]interface{}{"error": turnErr.Error()})
					continue
				}
				if err := a.persistChatTurn(session, input, turn, assistantMessage, liveTimeline); err != nil {
					return err
				}
				continue
			}
		}

		liveTimeline := isLiveTimelineWriter(a.out)
		activity := a.startTurnActivity(session)
		turn, assistantMessage, err := a.handleTurn(session, input, activity, turnRouteOptions{})
		activity.Stop()
		if err != nil {
			fmt.Fprintf(a.out, "[error] %v\n", err)
			_ = a.runLogger.Log("runtime", "turn_error", map[string]interface{}{"error": err.Error()})
			continue
		}
		if err := a.persistChatTurn(session, input, turn, assistantMessage, liveTimeline); err != nil {
			return err
		}

		if err == io.EOF {
			return nil
		}
	}
}

func (a *App) completeSlashTurn(session *Session, input string, run func() (Turn, string, error)) error {
	activity := a.startTurnActivity(session)
	turn, assistantMessage, err := run()
	activity.Stop()
	if err != nil {
		return err
	}
	return a.persistChatTurn(session, input, turn, assistantMessage, false)
}

func (a *App) persistChatTurn(session *Session, input string, turn Turn, assistantMessage string, liveTimeline bool) error {
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
	return nil
}
