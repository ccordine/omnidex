package omni

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
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
		}
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
		_, err = runStructuredCommandDecisionWithConfig(
			context.Background(),
			string(promptBytes),
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
				EnableCommandCache:      *enableCommandCache,
				CommandCacheRoot:        *commandCacheRoot,
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

		liveTimeline := isLiveTimelineWriter(a.out)
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

func (a *App) runUpdate(args []string) error {
	scriptPath, err := findManagedUpdateScript()
	if err != nil {
		return err
	}

	runArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command("bash", runArgs...)
	cmd.Dir = filepath.Dir(scriptPath)
	cmd.Stdout = a.out
	cmd.Stderr = a.errOut
	cmd.Stdin = a.in
	return cmd.Run()
}

func (a *App) runLedger(args []string) error {
	if len(args) == 0 || args[0] != "export" {
		return fmt.Errorf("usage: omni ledger export [--workspace PATH] [--session-root PATH] [--out PATH|-]")
	}
	fs := flag.NewFlagSet("omni ledger export", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	workspaceFlag := fs.String("workspace", "", "workspace whose session ledger should be exported; defaults to current directory")
	sessionRootFlag := fs.String("session-root", "", "override session root directory")
	outFlag := fs.String("out", "-", "output path, or - for stdout")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected ledger argument(s): %s", strings.Join(fs.Args(), " "))
	}
	workspace := strings.TrimSpace(*workspaceFlag)
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve workspace: %w", err)
		}
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return fmt.Errorf("resolve absolute workspace: %w", err)
	}
	store := NewSessionStore(*sessionRootFlag)
	session, _, err := store.LoadOrCreate(absWorkspace)
	if err != nil {
		return err
	}
	ledger := BuildEvidenceLedger(session)
	blob, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evidence ledger: %w", err)
	}
	if strings.TrimSpace(*outFlag) == "" || strings.TrimSpace(*outFlag) == "-" {
		_, err = fmt.Fprintln(a.out, string(blob))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*outFlag), 0o755); err != nil {
		return fmt.Errorf("create ledger output directory: %w", err)
	}
	if err := os.WriteFile(*outFlag, append(blob, '\n'), 0o644); err != nil {
		return fmt.Errorf("write evidence ledger: %w", err)
	}
	fmt.Fprintf(a.out, "Wrote evidence ledger: %s\n", *outFlag)
	return nil
}

func (a *App) runTrace(args []string) error {
	if len(args) == 0 || args[0] != "latest" {
		return fmt.Errorf("usage: omni run:trace latest [--workspace PATH] [--session-root PATH] [--json]")
	}
	fs := flag.NewFlagSet("omni run:trace latest", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	workspaceFlag := fs.String("workspace", "", "workspace whose latest session trace should be summarized; defaults to current directory")
	sessionRootFlag := fs.String("session-root", "", "override session root directory")
	jsonFlag := fs.Bool("json", false, "print JSON trace")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected run:trace argument(s): %s", strings.Join(fs.Args(), " "))
	}
	session, err := loadSessionForWorkspace(*workspaceFlag, *sessionRootFlag)
	if err != nil {
		return err
	}
	trace := BuildRunTrace(session)
	if *jsonFlag {
		blob, err := json.MarshalIndent(trace, "", "  ")
		if err != nil {
			return fmt.Errorf("encode run trace: %w", err)
		}
		_, err = fmt.Fprintln(a.out, string(blob))
		return err
	}
	_, err = fmt.Fprintln(a.out, formatRunTraceText(trace))
	return err
}

func (a *App) runFastPath(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: omni fastpath <git.branch|git.status|git.diffstat|package.manager|project.probe> [--workspace PATH] [--json]")
	}
	action := args[0]
	fs := flag.NewFlagSet("omni fastpath", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	workspaceFlag := fs.String("workspace", "", "workspace for deterministic probe; defaults to current directory")
	jsonFlag := fs.Bool("json", false, "print JSON result")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected fastpath argument(s): %s", strings.Join(fs.Args(), " "))
	}
	result := RunFastPath(context.Background(), action, *workspaceFlag)
	if *jsonFlag {
		blob, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("encode fastpath result: %w", err)
		}
		fmt.Fprintln(a.out, string(blob))
	} else {
		fmt.Fprintln(a.out, formatFastPathResult(result))
	}
	if !result.Success {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (a *App) runIndex(args []string) error {
	if len(args) == 0 || (args[0] != "build" && args[0] != "update") {
		return fmt.Errorf("usage: omni index <build|update> [--workspace PATH] [--out PATH] [--max-files N] [--json]")
	}
	mode := args[0]
	fs := flag.NewFlagSet("omni index "+mode, flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	workspaceFlag := fs.String("workspace", "", "workspace to index; defaults to current directory")
	outFlag := fs.String("out", "", "output path; defaults to .omni/index.json in the workspace")
	maxFilesFlag := fs.Int("max-files", 5000, "maximum files to hash")
	jsonFlag := fs.Bool("json", false, "print JSON index")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected index argument(s): %s", strings.Join(fs.Args(), " "))
	}
	workspace := strings.TrimSpace(*workspaceFlag)
	if workspace == "" {
		workspace = workspacePathOrCurrentDir()
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(*outFlag)
	if target == "" {
		target = filepath.Join(absWorkspace, ".omni", "index.json")
	}
	var index WorkspaceIndex
	if mode == "update" {
		index, err = UpdateWorkspaceIndex(absWorkspace, target, *maxFilesFlag)
	} else {
		index, err = BuildWorkspaceIndex(absWorkspace, *maxFilesFlag)
	}
	if err != nil {
		return err
	}
	if *jsonFlag {
		blob, err := json.MarshalIndent(index, "", "  ")
		if err != nil {
			return fmt.Errorf("encode workspace index: %w", err)
		}
		fmt.Fprintln(a.out, string(blob))
		return nil
	}
	if err := WriteWorkspaceIndex(index, target); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Wrote workspace index: %s\nfiles=%d manifests=%d package_manager=%s\n", target, len(index.Files), len(index.Manifests), index.PackageProbe.PackageManager)
	if mode == "update" {
		fmt.Fprintf(a.out, "reused_hashes=%d rehashed_files=%d added_files=%d removed_files=%d\n", index.Update.ReusedHashes, index.Update.RehashedFiles, index.Update.AddedFiles, index.Update.RemovedFiles)
	}
	return nil
}

func (a *App) runCodebaseMap(args []string) error {
	if len(args) == 0 || (args[0] != "build" && args[0] != "update" && args[0] != "query" && args[0] != "route") {
		return fmt.Errorf("usage: omni map <build|update|query|route> [--workspace PATH] [--out PATH] [--max-files N] [--json] [text]")
	}
	mode := args[0]
	fs := flag.NewFlagSet("omni map "+mode, flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	workspaceFlag := fs.String("workspace", "", "workspace to map; defaults to current directory")
	outFlag := fs.String("out", "", "map path; defaults to .omni/codebase-map.json in the workspace")
	maxFilesFlag := fs.Int("max-files", 5000, "maximum files to hash")
	jsonFlag := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	workspace := strings.TrimSpace(*workspaceFlag)
	if workspace == "" {
		workspace = workspacePathOrCurrentDir()
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(*outFlag)
	if target == "" {
		target = DefaultCodebaseMapPath(absWorkspace)
	}
	switch mode {
	case "build", "update":
		var cm CodebaseMap
		if mode == "update" {
			cm, err = UpdateCodebaseMap(absWorkspace, target, CodebaseMapConfig{MaxFiles: *maxFilesFlag})
		} else {
			cm, err = BuildCodebaseMap(absWorkspace, CodebaseMapConfig{MaxFiles: *maxFilesFlag, PreviousPath: target})
		}
		if err != nil {
			return err
		}
		if *jsonFlag {
			blob, err := json.MarshalIndent(cm, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(a.out, string(blob))
			return nil
		}
		if err := WriteCodebaseMap(cm, target); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Wrote codebase map: %s\nfiles=%d modules=%d symbols=%d tests=%d commands=%d\n", target, len(cm.Files), len(cm.Modules), len(cm.Symbols), len(cm.Tests), len(cm.Commands))
	case "query", "route":
		if fs.NArg() == 0 {
			return fmt.Errorf("omni map %s requires query text", mode)
		}
		cm, err := ReadCodebaseMap(target)
		if err != nil {
			return fmt.Errorf("read codebase map: %w", err)
		}
		text := strings.Join(fs.Args(), " ")
		if mode == "route" {
			route := RouteTaskWithCodebaseMap(cm, text)
			blob, err := json.MarshalIndent(route, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(a.out, string(blob))
			return nil
		}
		answer := QueryCodebaseMap(cm, text)
		blob, err := json.MarshalIndent(answer, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, string(blob))
	}
	return nil
}

func (a *App) runFingerprint(args []string) error {
	fs := flag.NewFlagSet("omni fingerprint", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	textFlag := fs.String("text", "", "failure text to classify; defaults to stdin")
	jsonFlag := fs.Bool("json", false, "print JSON result")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected fingerprint argument(s): %s", strings.Join(fs.Args(), " "))
	}
	text := *textFlag
	if strings.TrimSpace(text) == "" {
		blob, err := io.ReadAll(a.in)
		if err != nil {
			return fmt.Errorf("read failure text: %w", err)
		}
		text = string(blob)
	}
	fp := ClassifyFailure(text)
	if *jsonFlag {
		blob, err := json.MarshalIndent(fp, "", "  ")
		if err != nil {
			return fmt.Errorf("encode fingerprint: %w", err)
		}
		fmt.Fprintln(a.out, string(blob))
		return nil
	}
	fmt.Fprintf(a.out, "kind=%s\nsummary=%s\n", fp.Kind, fp.Summary)
	if fp.Remediation != "" {
		fmt.Fprintf(a.out, "remediation=%s\n", fp.Remediation)
	}
	return nil
}

func (a *App) runPatch(args []string) error {
	if len(args) == 0 || args[0] != "apply" {
		return fmt.Errorf("usage: omni patch apply [--workspace PATH] [--file PATCH|-] [--dry-run] [--json]")
	}
	fs := flag.NewFlagSet("omni patch apply", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	workspaceFlag := fs.String("workspace", "", "workspace where the patch should apply; defaults to current directory")
	fileFlag := fs.String("file", "-", "unified diff path, or - for stdin")
	dryRunFlag := fs.Bool("dry-run", false, "validate and report without writing files")
	jsonFlag := fs.Bool("json", false, "print JSON result")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected patch argument(s): %s", strings.Join(fs.Args(), " "))
	}
	patchText, err := a.readPatchInput(*fileFlag)
	if err != nil {
		return err
	}
	result, err := ApplyUnifiedPatch(PatchApplyOptions{
		Workspace: *workspaceFlag,
		Patch:     patchText,
		DryRun:    *dryRunFlag,
	})
	if err != nil {
		return err
	}
	if *jsonFlag {
		blob, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("encode patch result: %w", err)
		}
		fmt.Fprintln(a.out, string(blob))
		return nil
	}
	fmt.Fprint(a.out, FormatPatchApplyResult(result))
	return nil
}

func (a *App) readPatchInput(path string) (string, error) {
	if strings.TrimSpace(path) == "" || path == "-" {
		blob, err := io.ReadAll(a.in)
		if err != nil {
			return "", fmt.Errorf("read patch from stdin: %w", err)
		}
		return string(blob), nil
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read patch file: %w", err)
	}
	return string(blob), nil
}

func (a *App) runOllama(args []string) error {
	if len(args) == 0 || args[0] != "prewarm" {
		return fmt.Errorf("usage: omni ollama prewarm [--endpoint URL] [--model NAME] [--keep-alive DURATION] [--num-ctx N] [--json]")
	}
	fs := flag.NewFlagSet("omni ollama prewarm", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	endpointFlag := fs.String("endpoint", defaultOllamaEndpoint, "ollama chat endpoint")
	modelFlag := fs.String("model", firstNonEmpty(os.Getenv("OMNI_PLANNER_MODEL"), os.Getenv("OMNI_MODEL"), defaultOllamaPlannerModel), "model to prewarm")
	keepAliveFlag := fs.String("keep-alive", envOrDefault("OMNI_OLLAMA_KEEP_ALIVE", "30s"), "Ollama keep_alive value")
	numCtxFlag := fs.Int("num-ctx", envIntOrDefault("OMNI_PLANNER_NUM_CTX", envIntOrDefault("OMNI_OLLAMA_NUM_CTX", 4096)), "Ollama num_ctx value")
	jsonFlag := fs.Bool("json", false, "print JSON result")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected ollama argument(s): %s", strings.Join(fs.Args(), " "))
	}
	client := NewOllamaClient(*endpointFlag, *modelFlag)
	client.ConfigureRuntime(*keepAliveFlag, *numCtxFlag)
	result, err := client.Prewarm(context.Background())
	if *jsonFlag {
		blob, encodeErr := json.MarshalIndent(result, "", "  ")
		if encodeErr != nil {
			return fmt.Errorf("encode ollama prewarm result: %w", encodeErr)
		}
		fmt.Fprintln(a.out, string(blob))
		return err
	}
	fmt.Fprintf(a.out, "model=%s\nendpoint=%s\nkeep_alive=%s\nnum_ctx=%d\n", result.Model, result.Endpoint, result.KeepAlive, result.NumCtx)
	if err != nil {
		fmt.Fprintf(a.out, "diagnosis=%s\n", result.Diagnosis)
		return err
	}
	fmt.Fprintf(a.out, "done=%t\ntotal_duration=%d\nload_duration=%d\nprompt_eval_count=%d\neval_count=%d\n", result.Done, result.TotalDuration, result.LoadDuration, result.PromptEvalCount, result.EvalCount)
	return nil
}

func loadSessionForWorkspace(workspaceFlag, sessionRootFlag string) (*Session, error) {
	workspace := strings.TrimSpace(workspaceFlag)
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve workspace: %w", err)
		}
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute workspace: %w", err)
	}
	store := NewSessionStore(sessionRootFlag)
	session, _, err := store.LoadOrCreate(absWorkspace)
	if err != nil {
		return nil, err
	}
	return session, nil
}

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
	default:
		return fmt.Errorf("usage: omni bench <list|report|run>")
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

func loadOptionalRecipes(root string) []Recipe {
	recipes, err := LoadRecipes(resolveOmniResourceRoot(root, "recipes"))
	if err == nil {
		return recipes
	}
	return nil
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
	emitEvent("structured_command_started", "LLM structured command decision started", map[string]string{
		"permission_mode": string(session.Permission),
	})

	_ = a.runLogger.Log("structured_command", "turn_started", map[string]interface{}{
		"user_input":       input,
		"permission_mode":  session.Permission,
		"workspace":        session.WorkspacePath,
		"active_directory": session.ActiveDirectoryPath,
		"execution_policy": "stdin_prompt_llm_json_command_execute",
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
	activeDirectory := a.resolveActiveDirectoryForTurn(session, input, emitEvent)
	prepCtx := a.prepareInteractiveTurnContext(signalCtx, input, activeDirectory, emitEvent)
	memoryCtx := prepCtx.Memory
	sessionMemories := append([]SessionMemory(nil), session.Memories...)
	sessionMemories = append(sessionMemories, prepCtx.SessionMemories...)
	result, execErr := runStructuredCommandDecisionWithConfig(
		signalCtx,
		input,
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
			EnableCommandCache:      a.enableCommandCache,
			CommandCacheRoot:        a.commandCacheRoot,
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
	assistantResponse = a.reviewFinalResponse(context.Background(), input, assistantResponse, structuredResponseReviewEvidence(result, responseStdout, responseStderr, execErr), emitEvent)
	a.persistInteractiveTurnMemory(context.Background(), turnID, input, assistantResponse, memoryCtx.Tags, result.Observations, emitEvent)

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

func formatStructuredCommandChatResponse(result CommandDecisionResult, stdout, stderr, errText string) string {
	partialStopped := result.PartialProgress && strings.TrimSpace(errText) != ""
	statusLabel := "Exit code"
	if partialStopped {
		statusLabel = "Last command exit code"
	}
	lines := []string{}
	if partialStopped {
		lines = append(lines, "Partial result")
		lines = append(lines, "--------------")
		lines = append(lines, "Outcome: partial progress only; completion was not accepted.")
		lines = append(lines, "Completion: not accepted")
	} else {
		lines = append(lines, "Result")
		lines = append(lines, "------")
		if strings.TrimSpace(errText) == "" && result.ExitCode == 0 {
			lines = append(lines, "Outcome: complete; completion was accepted.")
		} else if strings.TrimSpace(errText) != "" {
			lines = append(lines, "Outcome: failed; no completion was accepted.")
		}
	}
	if strings.TrimSpace(result.Command) != "" {
		commandLabel := "Command"
		if partialStopped {
			commandLabel = "Last attempted command"
		}
		lines = appendFormattedResponseValue(lines, commandLabel, result.Command)
	} else if strings.TrimSpace(errText) != "" {
		lines = append(lines, "Command: (none accepted)")
	}
	lines = append(lines, fmt.Sprintf("%s: %d", statusLabel, result.ExitCode))
	if len(result.Observations) > 1 {
		lines = append(lines, fmt.Sprintf("Attempts: %d", len(result.Observations)))
	}
	if strings.TrimSpace(stdout) != "" {
		lines = append(lines, "")
		stdoutLabel := "Stdout"
		if partialStopped {
			stdoutLabel = "Latest captured stdout"
		}
		lines = appendFormattedResponseValue(lines, stdoutLabel, truncateOutput(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		lines = append(lines, "")
		stderrLabel := "Stderr"
		if partialStopped {
			stderrLabel = "Latest captured stderr"
		}
		lines = appendFormattedResponseValue(lines, stderrLabel, truncateOutput(stderr))
	}
	if strings.TrimSpace(result.Answer) != "" {
		lines = append(lines, "")
		answerLabel := "Answer"
		if partialStopped {
			answerLabel = "Latest captured answer"
		}
		lines = appendFormattedResponseValue(lines, answerLabel, result.Answer)
	}
	blocker := latestStructuredFailureSummary(result.Observations)
	if strings.TrimSpace(errText) != "" {
		lines = append(lines, "", "Status:")
		if result.PartialProgress {
			if pending := pendingStructuredObjectiveIDs(result.ObjectiveLedger); pending != "" {
				lines = append(lines, "  Pending objectives: "+pending)
			}
			if blocker != "" {
				label := "Last blocker"
				if strings.Contains(blocker, "anti_loop:") {
					label = "Loop blocker"
				}
				lines = append(lines, "  "+label+": "+blocker)
			}
			lines = append(lines, "  Stopped: "+errText)
		} else {
			if blocker != "" {
				label := "Last blocker"
				if strings.Contains(blocker, "anti_loop:") {
					label = "Loop blocker"
				}
				lines = append(lines, "  "+label+": "+blocker)
			}
			lines = append(lines, "  Error: "+errText)
		}
		if diagnosis := classifyStructuredLLMFailure(errors.New(errText)); diagnosis != "ollama_request_failure" {
			lines = append(lines, "  Diagnosis: "+diagnosis)
		}
	} else if !result.PartialProgress && result.ExitCode == 0 {
		lines = appendStructuredCompletionRecap(lines, result)
	}
	return strings.Join(lines, "\n")
}

func appendStructuredCompletionRecap(lines []string, result CommandDecisionResult) []string {
	recap := structuredCompletionRecapLines(result)
	if len(recap) == 0 {
		return lines
	}
	lines = append(lines, "", "Recap:")
	for _, line := range recap {
		lines = append(lines, "  "+line)
	}
	return lines
}

func structuredCompletionRecapLines(result CommandDecisionResult) []string {
	lines := []string{}
	if result.Elapsed > 0 {
		lines = append(lines, "Elapsed: "+formatStructuredElapsed(result.Elapsed))
	}
	if completed := completedStructuredObjectiveIDs(result.ObjectiveLedger); completed != "" {
		lines = append(lines, "Completed objectives: "+completed)
	}
	if evidence := structuredCompletionEvidenceSummary(result.ObjectiveLedger, result.Observations, 4); evidence != "" {
		lines = append(lines, "Evidence accepted: "+evidence)
	}
	if actions := structuredSuccessfulActionSummary(result.Observations, 4); actions != "" {
		lines = append(lines, "Actions: "+actions)
	}
	if decisions := structuredDecisionSummary(result.ObjectiveLedger, result.Observations, 4); decisions != "" {
		lines = append(lines, "Decisions: "+decisions)
	}
	return lines
}

func completedStructuredObjectiveIDs(ledger []StructuredObjective) string {
	ids := []string{}
	for _, objective := range ledger {
		if structuredObjectiveSatisfied(objective) {
			id := strings.TrimSpace(objective.ID)
			if id == "" {
				id = strings.TrimSpace(objective.Description)
			}
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return strings.Join(ids, ",")
}

func structuredCompletionEvidenceSummary(ledger []StructuredObjective, observations []StructuredCommandObservation, limit int) string {
	items := []string{}
	for _, objective := range ledger {
		if !structuredObjectiveSatisfied(objective) {
			continue
		}
		id := strings.TrimSpace(objective.ID)
		if id == "" {
			id = strings.TrimSpace(objective.Description)
		}
		evidence := strings.TrimSpace(objective.Evidence)
		if id == "" || evidence == "" {
			continue
		}
		items = append(items, truncateStructuredRecapItem(id+"="+evidence))
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		for _, obs := range observations {
			if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
				continue
			}
			evidence := strings.TrimSpace(firstNonEmpty(obs.Stdout, obs.Stderr, obs.Command))
			if evidence == "" {
				continue
			}
			items = append(items, truncateStructuredRecapItem(evidence))
			if len(items) >= limit {
				break
			}
		}
	}
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, "; ")
}

func structuredSuccessfulActionSummary(observations []StructuredCommandObservation, limit int) string {
	actions := []string{}
	seen := map[string]bool{}
	for _, obs := range observations {
		command := strings.TrimSpace(obs.Command)
		if command == "" || obs.ExitCode != 0 {
			continue
		}
		key := normalizeStructuredCommandForComparison(command)
		if seen[key] {
			continue
		}
		seen[key] = true
		actions = append(actions, truncateStructuredRecapItem(command))
		if len(actions) >= limit {
			break
		}
	}
	if len(actions) == 0 {
		return ""
	}
	if more := countAdditionalSuccessfulActions(observations, seen); more > 0 {
		actions = append(actions, fmt.Sprintf("+%d more", more))
	}
	return strings.Join(actions, "; ")
}

func countAdditionalSuccessfulActions(observations []StructuredCommandObservation, seen map[string]bool) int {
	count := 0
	for _, obs := range observations {
		command := strings.TrimSpace(obs.Command)
		if command == "" || obs.ExitCode != 0 {
			continue
		}
		key := normalizeStructuredCommandForComparison(command)
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
	}
	return count
}

func structuredDecisionSummary(ledger []StructuredObjective, observations []StructuredCommandObservation, limit int) string {
	decisions := []string{}
	for _, objective := range ledger {
		if objective.Source == structuredObjectiveSourceEvidenceRequiredPrerequisite {
			description := strings.TrimSpace(objective.ID)
			if description == "" {
				description = strings.TrimSpace(objective.Description)
			}
			if description != "" {
				decisions = appendUniqueRecapDecision(decisions, "added evidence-required prerequisite "+description, limit)
			}
		}
	}
	for _, obs := range observations {
		if obs.Cached {
			decisions = appendUniqueRecapDecision(decisions, "reused cached command evidence", limit)
		}
		if strings.TrimSpace(obs.RejectedCommand) != "" {
			decisions = appendUniqueRecapDecision(decisions, "rejected proposed command "+structuredCommandNameForRecap(obs.RejectedCommand), limit)
		}
		if strings.TrimSpace(obs.EvaluationFeedback) != "" {
			decisions = appendUniqueRecapDecision(decisions, "revised after evaluator feedback", limit)
		}
		if strings.TrimSpace(obs.Question) != "" {
			decisions = appendUniqueRecapDecision(decisions, "used user input for "+truncateStructuredRecapItem(obs.Question), limit)
		}
		if len(decisions) >= limit {
			break
		}
	}
	return strings.Join(decisions, "; ")
}

func appendUniqueRecapDecision(decisions []string, decision string, limit int) []string {
	if strings.TrimSpace(decision) == "" || len(decisions) >= limit {
		return decisions
	}
	for _, existing := range decisions {
		if existing == decision {
			return decisions
		}
	}
	return append(decisions, decision)
}

func truncateStructuredRecapItem(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const max = 96
	if len(value) <= max {
		return value
	}
	return value[:max-3] + "..."
}

func structuredCommandNameForRecap(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	if len(name) > 48 {
		name = name[:45] + "..."
	}
	return name
}

func formatStructuredElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return fmt.Sprintf("%dms", elapsed.Milliseconds())
	}
	return elapsed.Round(100 * time.Millisecond).String()
}

func appendFormattedResponseValue(lines []string, label, value string) []string {
	value = strings.TrimRight(value, "\n")
	if strings.Contains(value, "\n") {
		parts := strings.Split(value, "\n")
		lines = append(lines, label+": "+parts[0])
		if len(parts) > 1 {
			lines = append(lines, indentTimelineBlock(strings.Join(parts[1:], "\n"), "  "))
		}
		return lines
	}
	return append(lines, label+": "+value)
}

func structuredCommandResponseStreams(result CommandDecisionResult, stdout, stderr string, execErr error) (string, string) {
	if execErr != nil {
		return stdout, stderr
	}
	if latest, ok := latestSuccessfulCommandObservation(result.Observations); ok {
		return latest.Stdout, latest.Stderr
	}
	return stdout, stderr
}

func latestStructuredFailureSummary(observations []StructuredCommandObservation) string {
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode == 0 {
			continue
		}
		if strings.TrimSpace(obs.Stderr) != "" {
			return truncateOutput(obs.Stderr)
		}
		if strings.TrimSpace(obs.EvaluationFeedback) != "" {
			return truncateOutput(obs.EvaluationFeedback)
		}
		if strings.TrimSpace(obs.RejectedCommand) != "" {
			return "rejected command: " + truncateOutput(obs.RejectedCommand)
		}
	}
	return ""
}

func (a *App) reviewFinalResponse(ctx context.Context, userInput, response string, evidence []string, emitEvent func(string, string, map[string]string)) string {
	review := ReviewFinalAssistantResponse(FinalAssistantResponseReviewInput{
		UserInput: userInput,
		Response:  response,
		Evidence:  evidence,
	})
	finalResponse := review.Response
	details := map[string]string{
		"passed":     fmt.Sprintf("%t", review.Passed),
		"confidence": fmt.Sprintf("%d", review.Confidence),
		"feedback":   truncateOutput(review.Feedback),
	}

	if a.evaluator != nil {
		evaluation, err := a.evaluator.EvaluateStructuredLLMResponse(ctx, StructuredLLMEvaluationInput{
			Step:        0,
			UserPrompt:  userInput,
			PlannerJob:  finalResponseReviewerJobSummary(),
			LLMResponse: finalResponse,
			Observations: []StructuredCommandObservation{{
				Step:     0,
				Command:  "FINAL_RESPONSE_EVIDENCE",
				ExitCode: 0,
				Stdout:   truncateStructuredObservation(strings.Join(evidence, "\n")),
			}},
		})
		if err != nil {
			details["evaluator_error"] = truncateOutput(err.Error())
		} else if consistencyErr := validateStructuredEvaluationConsistency(evaluation); consistencyErr != nil {
			details["evaluator_error"] = truncateOutput(consistencyErr.Error())
			details["evaluator_confidence"] = fmt.Sprintf("%d", evaluation.Confidence)
			details["evaluator_feedback"] = truncateOutput(evaluation.Feedback)
		} else {
			details["evaluator_confidence"] = fmt.Sprintf("%d", evaluation.Confidence)
			details["evaluator_feedback"] = truncateOutput(evaluation.Feedback)
			if evaluation.Confidence < normalizeStructuredEvaluatorThreshold(a.evaluatorThreshold) {
				review.Passed = false
				review.Confidence = evaluation.Confidence
				review.Feedback = strings.TrimSpace(evaluation.Feedback)
				finalResponse = buildFinalReviewCorrection(userInput, finalResponse, strings.Join(evidence, "\n"))
				details["passed"] = "false"
				details["confidence"] = fmt.Sprintf("%d", evaluation.Confidence)
				details["feedback"] = truncateOutput(review.Feedback)
			}
		}
	}

	if emitEvent != nil {
		eventType := "final_response_review_passed"
		summary := "Final response self-review passed"
		if !review.Passed {
			eventType = "final_response_review_revised"
			summary = "Final response self-review revised response"
		}
		emitEvent(eventType, summary, details)
	}
	if a.runLogger != nil {
		_ = a.runLogger.Log("final_response_review", "completed", map[string]interface{}{
			"user_input": userInput,
			"response":   finalResponse,
			"details":    details,
		})
	}
	return finalResponse
}

func finalResponseReviewerJobSummary() string {
	return strings.Join([]string{
		"Review the final user-facing assistant response before it is shown.",
		"Score whether it directly answers the current user prompt, stays grounded in provided evidence, and does not drift to prior tasks.",
		"High confidence means it is on task and safe to send.",
		"Low confidence means it is empty, off-task, overclaims, ignores evidence, or falsely refuses available tool capability.",
	}, " ")
}

func structuredResponseReviewEvidence(result CommandDecisionResult, stdout, stderr string, execErr error) []string {
	evidence := []string{
		"command=" + result.Command,
		fmt.Sprintf("exit_code=%d", result.ExitCode),
	}
	if strings.TrimSpace(stdout) != "" {
		evidence = append(evidence, "stdout="+stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		evidence = append(evidence, "stderr="+stderr)
	}
	if strings.TrimSpace(result.Answer) != "" {
		evidence = append(evidence, "answer="+result.Answer)
	}
	if execErr != nil {
		evidence = append(evidence, "error="+execErr.Error())
	}
	return evidence
}

func (a *App) startTurnActivity(session *Session) *activityIndicator {
	if session.Permission != PermissionFull || !isInteractiveWriter(a.out) {
		return &activityIndicator{}
	}
	return startActivityIndicator(a.out, "working")
}

func (a *App) startEscapeInterrupt(ctx context.Context, cancel context.CancelFunc, activity *activityIndicator, emitEvent func(string, string, map[string]string)) func() {
	if a.terminalIn == nil || cancel == nil || !isInteractive(a.terminalIn) || !isInteractiveWriter(a.out) {
		return func() {}
	}
	fd := int(a.terminalIn.Fd())
	restore, err := enableTerminalCbreak(fd)
	if err != nil {
		return func() {}
	}
	done := make(chan struct{})
	stop := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		defer restore()
		buffer := []byte{0}
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			default:
			}
			ready, err := pollTerminalInput(fd, 100*time.Millisecond)
			if err != nil {
				return
			}
			if !ready {
				continue
			}
			n, err := readTerminalByte(fd, buffer)
			if err != nil || n == 0 {
				continue
			}
			if buffer[0] != 0x1b {
				continue
			}
			cancel()
			if activity != nil {
				activity.Pause()
			}
			if emitEvent != nil {
				emitEvent("user_interrupt_requested", "User pressed Esc to interrupt the active turn", map[string]string{
					"input": "esc",
				})
			}
			if activity != nil {
				activity.Pause()
			}
			return
		}
	}()
	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}
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

func (a *App) structuredPlannerClient() CommandDecisionClient {
	if a.plannerClient != nil {
		return a.plannerClient
	}
	if a.planner != nil {
		return a.planner
	}
	if a.ollama != nil {
		return a.ollama
	}
	return nil
}

func (a *App) planContextForTurn(ctx context.Context, input string) (ContextToolPlan, []Event) {
	plan, err := PlanContextTools(ctx, a.planner, input)
	plan = AugmentContextToolPlan(input, plan)
	events := []Event{a.newEvent("context_plan_created", "Context tool plan created", map[string]string{
		"tools":  strings.Join(plan.Tools, ","),
		"reason": plan.Reason,
	})}
	if err != nil {
		events = append(events, a.newEvent("context_plan_failed", "Context tool planner fell back to default", map[string]string{"error": err.Error()}))
	}
	return plan, events
}

type interactivePrepContext struct {
	Plan            ContextToolPlan
	Memory          interactiveMemoryContext
	SessionMemories []SessionMemory
	Bundle          PrepContextBundle
	Validation      PrepValidation
}

func (a *App) prepareInteractiveTurnContext(ctx context.Context, input, activeDirectory string, emitEvent func(string, string, map[string]string)) interactivePrepContext {
	prep := interactivePrepContext{}
	emitEvent("prep_started", "Preparing compact task context", map[string]string{
		"active_directory": activeDirectory,
	})
	plan, planEvents := a.planContextForTurn(ctx, input)
	prep.Plan = plan
	for _, event := range planEvents {
		emitEvent(event.Type, event.Summary, event.Details)
	}
	var route TaskRoute
	if routeMemory, preparedRoute, ok := a.prepareCodebaseRouteBrief(ctx, activeDirectory, input, emitEvent); ok {
		route = preparedRoute
		prep.SessionMemories = append(prep.SessionMemories, routeMemory)
	}
	memoryCtx := a.loadInteractiveMemoryContext(ctx, input, activeDirectory, emitEvent)
	prep.Memory = memoryCtx
	prep.SessionMemories = append(prep.SessionMemories, memoryCtx.Memories...)
	if docMemory, ok := a.prepareDocumentationBrief(ctx, input, memoryCtx.Tags, plan, emitEvent); ok {
		prep.SessionMemories = append(prep.SessionMemories, docMemory)
	}
	if webMemory, ok := a.prepareWebResearchBrief(ctx, input, plan, emitEvent); ok {
		prep.SessionMemories = append(prep.SessionMemories, webMemory)
	}
	survey := BuildWorksiteSurvey(activeDirectory)
	prep.Bundle = CompactPrepContextBundle(NewPrepContextBundle("interactive-turn", activeDirectory, survey, plan, route, prep.SessionMemories), defaultPrepContextBudgetLimit)
	prep.Validation = ValidatePrepContextBundle(prep.Bundle, plan)
	emitEvent("prep_context_built", "Preparation context bundle built", map[string]string{
		"briefs":       fmt.Sprintf("%d", len(allPrepBriefs(prep.Bundle))),
		"evidence":     fmt.Sprintf("%d", len(prep.Bundle.Evidence)),
		"budget_used":  fmt.Sprintf("%d", prep.Bundle.ContextBudgetUsed),
		"budget_limit": fmt.Sprintf("%d", prep.Bundle.ContextBudgetLimit),
		"compressed":   fmt.Sprintf("%t", prep.Bundle.Compressed),
	})
	validationType := "prep_context_validated"
	validationSummary := "Preparation context bundle validated"
	if !prep.Validation.Valid {
		validationType = "prep_context_validation_failed"
		validationSummary = "Preparation context bundle failed validation"
	}
	emitEvent(validationType, validationSummary, map[string]string{
		"valid":    fmt.Sprintf("%t", prep.Validation.Valid),
		"failures": strings.Join(prep.Validation.Failures, ","),
		"warnings": strings.Join(prep.Validation.Warnings, ","),
	})
	emitEvent("prep_completed", "Preparation context ready", map[string]string{
		"tools":              strings.Join(plan.Tools, ","),
		"memory_records":     fmt.Sprintf("%d", len(memoryCtx.Records)),
		"handoff_briefs":     fmt.Sprintf("%d", len(prep.SessionMemories)),
		"evidence_items":     fmt.Sprintf("%d", len(prep.Bundle.Evidence)),
		"context_policy":     "minimum_necessary_advisory_context",
		"continuation_ready": "true",
	})
	return prep
}

func (a *App) prepareCodebaseRouteBrief(ctx context.Context, activeDirectory, input string, emitEvent func(string, string, map[string]string)) (SessionMemory, TaskRoute, bool) {
	workspace := strings.TrimSpace(activeDirectory)
	if workspace == "" {
		return SessionMemory{}, TaskRoute{}, false
	}
	emitEvent("prep_workspace_scan_started", "Inspecting workspace for codebase route", map[string]string{
		"workspace": workspace,
	})
	cm, err := UpdateCodebaseMap(workspace, DefaultCodebaseMapPath(workspace), CodebaseMapConfig{MaxFiles: 1200})
	if err != nil {
		emitEvent("prep_workspace_scan_failed", "Workspace route preparation failed", map[string]string{
			"workspace": workspace,
			"error":     truncateOutput(err.Error()),
		})
		return SessionMemory{}, TaskRoute{}, false
	}
	if err := WriteCodebaseMap(cm, DefaultCodebaseMapPath(workspace)); err != nil {
		emitEvent("prep_workspace_scan_failed", "Codebase map write failed", map[string]string{
			"workspace": workspace,
			"error":     truncateOutput(err.Error()),
		})
		return SessionMemory{}, TaskRoute{}, false
	}
	route := RouteTaskWithCodebaseMap(cm, input)
	routeContent := formatCodebaseRouteBrief(route)
	if a != nil && a.memory != nil && strings.TrimSpace(routeContent) != "" {
		if err := a.memory.EnsureSchema(ctx); err == nil {
			if _, err := a.memory.AddMemory(ctx, "codebase_context_manager", "codebase_route_brief", routeContent, []string{"prep-context", "codebase-route", "file-chunks", "workspace:" + workspaceHash(workspace)}); err == nil {
				emitEvent("prep_codebase_route_memory_stored", "Codebase route and chunk context stored in memory", map[string]string{
					"workspace": workspace,
					"chunks":    fmt.Sprintf("%d", len(route.FileChunks)),
				})
			}
		}
	}
	emitEvent("prep_workspace_scan_completed", "Codebase route prepared from workspace evidence", map[string]string{
		"workspace":      workspace,
		"files":          fmt.Sprintf("%d", len(cm.Files)),
		"modules":        fmt.Sprintf("%d", len(cm.Modules)),
		"chunks":         fmt.Sprintf("%d", len(route.FileChunks)),
		"likely_files":   strings.Join(route.LikelyFiles, ","),
		"verification":   strings.Join(route.VerificationCommands, ","),
		"confidence":     fmt.Sprintf("%d", route.Confidence),
		"evidence_file":  DefaultCodebaseMapPath(workspace),
		"continuable_by": "codebase-map",
	})
	return SessionMemory{
		Kind:    "codebase_route_brief",
		Content: routeContent,
		Tags:    []string{"prep-context", "codebase-route", "workspace:" + workspaceHash(workspace)},
	}, route, true
}

func (a *App) prepareDocumentationBrief(ctx context.Context, input string, tags []string, plan ContextToolPlan, emitEvent func(string, string, map[string]string)) (SessionMemory, bool) {
	if !plan.NeedsDocuments || a == nil {
		return SessionMemory{}, false
	}
	searchTags := cleanMemoryTags(append([]string{"documentation"}, tags...))
	if a.memory != nil {
		emitEvent("documentation_memory_search_started", "Documentation specialist checking documentation memory", map[string]string{
			"query": input,
			"tags":  strings.Join(searchTags, ","),
			"role":  "documentation_specialist",
		})
		answer, err := AnswerDocumentationQuestionFromMemory(ctx, input, a.memory, searchTags, 4)
		if err != nil {
			emitEvent("documentation_memory_search_failed", "Documentation memory search failed", map[string]string{
				"error": truncateOutput(err.Error()),
			})
		} else if !answer.NeedsScrape {
			emitEvent("documentation_brief_loaded", "Documentation specialist loaded reusable guidance", map[string]string{
				"sources": fmt.Sprintf("%d", len(answer.Brief.Sources)),
				"role":    "documentation_specialist",
			})
			return SessionMemory{
				Kind:      "documentation_brief",
				Content:   answer.Answer,
				Tags:      cleanMemoryTags(append([]string{"prep-context", "documentation"}, tags...)),
				CreatedAt: nowUTC(),
			}, true
		} else {
			emitEvent("documentation_research_needed", "Documentation specialist found no reusable brief", map[string]string{
				"query":  input,
				"reason": "no_matching_documentation_memory",
			})
		}
	} else {
		emitEvent("documentation_research_needed", "Documentation specialist has no memory store; fetching authoritative docs", map[string]string{
			"query":  input,
			"reason": "memory_unavailable",
		})
	}

	target := InferDocumentationResearchTarget(input)
	if len(target.Sources) == 0 {
		emitEvent("documentation_research_skipped", "Documentation specialist has no authoritative source route", map[string]string{
			"query": input,
		})
		return SessionMemory{}, false
	}

	emitEvent("documentation_web_research_started", "Documentation specialist fetching authoritative docs", map[string]string{
		"query":   input,
		"sources": strings.Join(webDocSourceURLs(target.Sources), "\n"),
		"role":    "documentation_specialist",
	})
	research, err := ResearchWebDocs(ctx, input, target.Sources, target.Queries, WebDocResearchConfig{
		FetchTimeout: 20 * time.Second,
		ChunkConfig:  DocumentSearchConfig{ChunkChars: 2400, ChunkOverlap: 300},
		MaxHits:      8,
	})
	if err != nil {
		emitEvent("documentation_web_research_failed", "Documentation specialist web research failed", map[string]string{
			"error": truncateOutput(err.Error()),
		})
		return SessionMemory{}, false
	}
	if len(research.Hits) == 0 {
		emitEvent("documentation_web_research_empty", "Documentation specialist found no matching excerpts", map[string]string{
			"sources": fmt.Sprintf("%d", len(research.Sources)),
			"queries": strings.Join(target.Queries, " | "),
		})
		return SessionMemory{}, false
	}

	if a.memory != nil {
		if err := storeDocResearchHits(ctx, a.memory, input, research.Hits, append(searchTags, target.Tags...)); err != nil {
			emitEvent("documentation_memory_store_failed", "Documentation specialist could not store fetched docs", map[string]string{
				"error": truncateOutput(err.Error()),
			})
		} else {
			emitEvent("documentation_memory_stored", "Documentation specialist stored fetched docs", map[string]string{
				"hits": fmt.Sprintf("%d", len(research.Hits)),
			})
		}
	}

	content := FormatDocumentationAuthorityBrief(BuildDocumentationAuthorityBrief(input, docResearchHitsAsMemories(input, research.Hits)))
	emitEvent("documentation_brief_loaded", "Documentation specialist loaded fetched guidance", map[string]string{
		"sources": fmt.Sprintf("%d", len(research.Sources)),
		"hits":    fmt.Sprintf("%d", len(research.Hits)),
		"role":    "documentation_specialist",
	})
	return SessionMemory{
		Kind:      "documentation_brief",
		Content:   content,
		Tags:      cleanMemoryTags(append([]string{"prep-context", "documentation"}, append(tags, target.Tags...)...)),
		CreatedAt: nowUTC(),
	}, true
}

func (a *App) prepareWebResearchBrief(ctx context.Context, input string, plan ContextToolPlan, emitEvent func(string, string, map[string]string)) (SessionMemory, bool) {
	if !plan.NeedsWebResearch {
		return SessionMemory{}, false
	}
	events, observation := a.autoResearchForTurn(ctx, input, plan)
	for _, event := range events {
		emitEvent(event.Type, event.Summary, event.Details)
	}
	if observation == nil || strings.TrimSpace(observation.Stdout) == "" {
		return SessionMemory{}, false
	}
	return SessionMemory{
		Kind: "web_research_brief",
		Content: strings.TrimSpace(strings.Join([]string{
			"WEB_RESEARCH_BRIEF",
			"query: " + strings.TrimSpace(input),
			"content:",
			observation.Stdout,
		}, "\n")),
		Tags:      cleanMemoryTags(append([]string{"prep-context", "web-research"}, researchTagsFromQuery(input)...)),
		CreatedAt: nowUTC(),
	}, true
}

func formatCodebaseRouteBrief(route TaskRoute) string {
	lines := []string{
		"CODEBASE_ROUTE_BRIEF",
		"intent: " + strings.TrimSpace(route.Intent),
		fmt.Sprintf("confidence: %d", route.Confidence),
	}
	if len(route.LikelyFiles) > 0 {
		lines = append(lines, "likely_files: "+strings.Join(route.LikelyFiles, ", "))
	}
	if len(route.RelevantModules) > 0 {
		lines = append(lines, "relevant_modules: "+strings.Join(route.RelevantModules, ", "))
	}
	if len(route.VerificationCommands) > 0 {
		lines = append(lines, "verification_commands: "+strings.Join(route.VerificationCommands, " | "))
	}
	if len(route.KnownRisks) > 0 {
		lines = append(lines, "known_risks: "+strings.Join(route.KnownRisks, " | "))
	}
	if len(route.Reasons) > 0 {
		lines = append(lines, "reasons: "+strings.Join(route.Reasons, " | "))
	}
	if route.ContextPolicy != "" {
		lines = append(lines, "context_policy: "+route.ContextPolicy)
	}
	if len(route.FileChunks) > 0 {
		lines = append(lines, "file_chunks:")
		for _, chunk := range route.FileChunks {
			lines = append(lines,
				fmt.Sprintf("- id: %s", chunk.ID),
				fmt.Sprintf("  path: %s", chunk.Path),
				fmt.Sprintf("  lines: %d-%d", chunk.StartLine, chunk.EndLine),
				fmt.Sprintf("  sed: %s", chunk.SedCommand),
				fmt.Sprintf("  reason: %s", chunk.Reason),
			)
			if strings.TrimSpace(chunk.Preview) != "" {
				lines = append(lines, "  preview: |")
				for _, line := range strings.Split(chunk.Preview, "\n") {
					lines = append(lines, "    "+line)
				}
			}
		}
		lines = append(lines,
			"chunk_editing_rules:",
			"- never request or load the full file when file_chunks are available",
			"- edit one chunk or adjacent chunk range at a time using the line anchors",
			"- use the provided sed command for readback before constructing sed/perl/patch edits",
			"- after each chunk edit, verify the changed line range and continue to the next needed chunk",
		)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (a *App) autoResearchForTurn(ctx context.Context, input string, plan ContextToolPlan) ([]Event, *CommandObservation) {
	if !plan.NeedsWebResearch {
		return nil, nil
	}
	queries := BuildWebResearchQueries(input, plan, 4)
	if len(queries) == 0 {
		return nil, nil
	}
	query := queries[0]
	events := []Event{}
	events = append(events, a.newEvent("auto_research_started", "Automatic web research started", map[string]string{
		"query":   query,
		"queries": strings.Join(queries, " | "),
	}))
	memoryContext := ""
	if a.memory != nil {
		memoryRecords, err := searchAutoResearchMemory(ctx, a.memory, queries, 4)
		if err != nil {
			events = append(events, a.newEvent("auto_research_memory_lookup_failed", "Automatic research memory lookup failed", map[string]string{"error": truncateOutput(err.Error())}))
		} else if len(memoryRecords) > 0 {
			memoryContext = formatAutoResearchMemoryContext(memoryRecords, 2400)
			events = append(events, a.newEvent("auto_research_memory_loaded", "Automatic research loaded relevant memory before web search", map[string]string{
				"records": fmt.Sprintf("%d", len(memoryRecords)),
				"kinds":   strings.Join(memoryRecordKinds(memoryRecords), ","),
				"ids":     strings.Join(memoryRecordIDs(memoryRecords), ","),
			}))
		}
	}
	if a.web == nil {
		events = append(events, a.newEvent("auto_research_skipped", "Web search service is unavailable", nil))
		if strings.TrimSpace(memoryContext) == "" {
			return events, nil
		}
		return events, &CommandObservation{
			Step:    0,
			Command: "AUTO_RESEARCH_MEMORY: " + query,
			Status:  "success",
			Stdout:  truncateForObservation(memoryContext, defaultAgentObservationChars),
		}
	}
	searchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	results := []websearch.Result{}
	searchErrors := []string{}
	for _, candidate := range queries {
		events = append(events, a.newEvent("web_search_started", "Web search started", map[string]string{
			"query": candidate,
			"role":  "web_research_specialist",
		}))
		candidateResults, err := a.web.SearchAll(searchCtx, candidate)
		if err != nil {
			searchErrors = append(searchErrors, candidate+": "+err.Error())
			continue
		}
		events = append(events, a.newEvent("web_search_completed", "Web search completed", webSearchTimelineDetails(candidate, candidateResults)))
		results = append(results, candidateResults...)
		if len(results) >= 8 {
			break
		}
	}
	results = dedupeWebSearchResults(results)
	if len(results) == 0 {
		detail := "no search results"
		if len(searchErrors) > 0 {
			detail = strings.Join(searchErrors, " | ")
		}
		events = append(events, a.newEvent("auto_research_failed", "Automatic web research failed", map[string]string{"error": detail}))
		return events, nil
	}
	contextText := strings.TrimSpace(strings.Join([]string{memoryContext, websearch.BuildContext(results, 5000)}, "\n\n"))
	if strings.TrimSpace(contextText) == "" {
		events = append(events, a.newEvent("auto_research_failed", "Automatic web research returned empty context", nil))
		return events, nil
	}
	events = append(events, a.newEvent("auto_research_completed", "Automatic web research context captured", map[string]string{
		"query":       query,
		"queries":     strings.Join(queries, " | "),
		"results":     fmt.Sprintf("%d", len(results)),
		"result_urls": strings.Join(webSearchResultURLs(results, 5), "\n"),
	}))

	if a.memory != nil {
		if result, storeErr := ResearchWebToMemory(ctx, query, staticWebSearchResults{results: results}, a.memory, WebResearchMemoryConfig{
			AgentID: "omni_auto_researcher",
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

func searchAutoResearchMemory(ctx context.Context, memory *PGMemoryStore, queries []string, limit int) ([]MemoryRecord, error) {
	if memory == nil {
		return nil, nil
	}
	if err := memory.EnsureSchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 4
	}
	seen := map[int64]bool{}
	out := []MemoryRecord{}
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		records, err := memory.SearchMemory(ctx, query, []string{"web", "research", "documentation"}, limit)
		if err != nil {
			continue
		}
		for _, record := range records {
			if record.ID > 0 && seen[record.ID] {
				continue
			}
			if record.ID > 0 {
				seen[record.ID] = true
			}
			out = append(out, record)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func formatAutoResearchMemoryContext(records []MemoryRecord, budget int) string {
	if len(records) == 0 {
		return ""
	}
	lines := []string{"MEMORY_RESEARCH_CONTEXT"}
	for _, record := range records {
		content := strings.TrimSpace(record.Content)
		if content == "" {
			continue
		}
		segment := strings.Join([]string{
			fmt.Sprintf("memory_id: %d", record.ID),
			"kind: " + firstNonEmpty(record.Kind, "memory"),
			"tags: " + strings.Join(cleanMemoryTags(record.Tags), ","),
			"content:",
			truncateForStructuredContext(content, 900),
		}, "\n")
		if budget > 0 && len(strings.Join(append(lines, segment), "\n\n")) > budget {
			break
		}
		lines = append(lines, segment)
	}
	return strings.TrimSpace(strings.Join(lines, "\n\n"))
}

func BuildWebResearchQueries(input string, plan ContextToolPlan, limit int) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if limit <= 0 {
		limit = 4
	}
	lower := strings.ToLower(input)
	terms := importantSearchTerms(input)
	joinedTerms := strings.Join(terms, " ")
	queries := []string{}
	add := func(query string) {
		query = strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
		if query != "" {
			queries = append(queries, query)
		}
	}
	switch {
	case strings.Contains(lower, "weather"):
		location := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(joinedTerms, "weather", ""), "current", ""))
		add(strings.TrimSpace(location + " weather forecast current conditions"))
		add(strings.TrimSpace(location + " hourly weather today"))
	case strings.Contains(lower, "news") || strings.Contains(lower, "current events"):
		add(joinedTerms + " latest news")
		add(joinedTerms + " breaking news today")
	case plan.NeedsDocuments || technicalSearchLooksLikely(lower):
		add(joinedTerms + " official documentation")
		add(joinedTerms + " getting started guide")
		add(joinedTerms + " examples best practices")
	default:
		add(joinedTerms + " official source")
		add(joinedTerms + " latest reference")
	}
	add(input)
	return limitStrings(dedupeStrings(queries), limit)
}

func importantSearchTerms(input string) []string {
	words := strings.Fields(webDocNonWordQuery.ReplaceAllString(input, " "))
	stop := map[string]bool{
		"a": true, "an": true, "and": true, "app": true, "application": true, "as": true, "be": true, "build": true,
		"can": true, "create": true, "for": true, "how": true, "i": true, "in": true, "is": true, "it": true,
		"make": true, "me": true, "of": true, "on": true, "please": true, "should": true, "the": true, "to": true,
		"use": true, "using": true, "with": true, "what": true, "right": true, "now": true,
	}
	terms := []string{}
	for _, word := range words {
		clean := strings.Trim(word, " .,;:!?\"'`()[]{}")
		if clean == "" {
			continue
		}
		lower := strings.ToLower(clean)
		if stop[lower] {
			continue
		}
		terms = append(terms, clean)
		if len(terms) >= 8 {
			break
		}
	}
	if len(terms) == 0 {
		return []string{input}
	}
	return terms
}

func technicalSearchLooksLikely(lower string) bool {
	for _, needle := range []string{"api", "sdk", "react", "vite", "node", "javascript", "typescript", "rust", "go", "golang", "zig", "php", "docker", "postgres", "pgsql", "cli", "library", "framework"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func dedupeWebSearchResults(results []websearch.Result) []websearch.Result {
	seen := map[string]struct{}{}
	out := make([]websearch.Result, 0, len(results))
	for _, result := range results {
		key := strings.TrimSpace(result.URL)
		if key == "" {
			key = strings.TrimSpace(result.Title + "\n" + result.Snippet)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, result)
	}
	return out
}

type staticWebSearchResults struct {
	results []websearch.Result
}

func (s staticWebSearchResults) SearchAll(ctx context.Context, query string) ([]websearch.Result, error) {
	return s.results, nil
}

func webSearchTimelineDetails(query string, results []websearch.Result) map[string]string {
	return map[string]string{
		"query":       strings.TrimSpace(query),
		"results":     fmt.Sprintf("%d", len(results)),
		"providers":   strings.Join(webSearchProviders(results), ","),
		"search_urls": strings.Join(webSearchSearchURLs(results, 5), "\n"),
		"result_urls": strings.Join(webSearchResultURLs(results, 5), "\n"),
	}
}

func webSearchProviders(results []websearch.Result) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, result := range results {
		provider := strings.TrimSpace(result.Provider)
		if provider == "" || seen[provider] {
			continue
		}
		seen[provider] = true
		out = append(out, provider)
	}
	return out
}

func webSearchSearchURLs(results []websearch.Result, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, result := range results {
		url := strings.TrimSpace(result.SearchURL)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		out = append(out, url)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func webSearchResultURLs(results []websearch.Result, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, result := range results {
		url := strings.TrimSpace(result.URL)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		out = append(out, url)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
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
	assistantResponse = a.reviewFinalResponse(context.Background(), "/manage "+objective, assistantResponse, []string{result.Summary}, func(eventType, summary string, details map[string]string) {
		events = append(events, a.newEvent(eventType, summary, details))
	})

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
			"hint": "set --memory-database-url or OMNI_MEMORY_DATABASE_URL",
		}))
		turn := Turn{
			ID:                   turnID,
			UserInput:            "/research " + query,
			IntentClassification: IntentExecution,
			Confidence:           1.0,
			ReasonCodes:          []string{"web_research_memory"},
			Events:               events,
			CreatedAt:            nowUTC(),
		}
		response := "Research blocked: Postgres memory is not configured. Set --memory-database-url or OMNI_MEMORY_DATABASE_URL."
		response = a.reviewFinalResponse(context.Background(), turn.UserInput, response, []string{"Postgres memory is not configured"}, func(eventType, summary string, details map[string]string) {
			turn.Events = append(turn.Events, a.newEvent(eventType, summary, details))
		})
		turn.Response = response
		return turn, turn.Response, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	events = append(events, a.newEvent("web_search_started", "Web search started", map[string]string{
		"query": query,
		"role":  "web_research_specialist",
	}))
	result, err := ResearchWebToMemory(ctx, query, a.web, a.memory, WebResearchMemoryConfig{
		AgentID: "omni_research_manager",
		Tags:    researchTagsFromQuery(query),
	})
	if err != nil {
		events = append(events, a.newEvent("research_failed", "Web research memory job failed", map[string]string{"error": err.Error()}))
		return Turn{}, "", err
	}
	events = append(events, a.newEvent("web_search_completed", "Web search completed", webSearchTimelineDetails(query, result.Results)))
	events = append(events, a.newEvent("research_completed", "Web research stored in Postgres memory", map[string]string{
		"query":        query,
		"results":      fmt.Sprintf("%d", len(result.Results)),
		"stored":       fmt.Sprintf("%d", result.StoredCount),
		"stored_agent": "omni_research_manager",
		"result_urls":  strings.Join(webSearchResultURLs(result.Results, 5), "\n"),
	}))
	response := fmt.Sprintf("Stored %d web research memory chunk(s) from %d search result(s) for: %s", result.StoredCount, len(result.Results), query)
	response = a.reviewFinalResponse(context.Background(), "/research "+query, response, []string{
		fmt.Sprintf("stored=%d results=%d", result.StoredCount, len(result.Results)),
	}, func(eventType, summary string, details map[string]string) {
		events = append(events, a.newEvent(eventType, summary, details))
	})
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
		response := "Conversation mode: understood. Share what you want to explore, and I’ll keep it in planning/discussion mode."
		return a.reviewFinalResponse(context.Background(), input, response, nil, nil), "local_fallback"
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
		response := "Conversation mode: understood. (Ollama unavailable right now, continuing with local fallback.)"
		return a.reviewFinalResponse(context.Background(), input, response, nil, nil), "local_fallback"
	}

	_ = a.runLogger.Log("conversation", "llm_call", map[string]interface{}{
		"request":           resp.RequestJSON,
		"response":          resp.ResponseJSON,
		"total_duration_ns": resp.TotalDuration,
		"prompt_eval_count": resp.PromptEvalCount,
		"eval_count":        resp.EvalCount,
	})

	return a.reviewFinalResponse(context.Background(), input, resp.Content, []string{"conversation history response"}, nil), "ollama"
}

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
	fmt.Fprintf(a.out, "\n[%s]\n", evt.CreatedAt)
	fmt.Fprintf(a.out, "  %-32s %s\n", evt.Type, evt.Summary)
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
