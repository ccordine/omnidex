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
	"syscall"
	"time"
	"unsafe"

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
		_, err = runStructuredCommandDecisionWithConfig(
			context.Background(),
			string(promptBytes),
			nil,
			a.structuredPlannerClient(),
			a.out,
			a.errOut,
			nil,
			nil,
			structuredCommandDecisionRunConfig{
				CurrentWorkingDirectory: workspacePathOrCurrentDir(),
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
	memoryCtx := a.loadInteractiveMemoryContext(signalCtx, input, activeDirectory, emitEvent)
	sessionMemories := append([]SessionMemory(nil), session.Memories...)
	sessionMemories = append(sessionMemories, memoryCtx.Memories...)
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

	assistantResponse := formatStructuredCommandChatResponse(result, stdoutBuf.String(), stderrBuf.String(), "")
	if execErr != nil {
		assistantResponse = formatStructuredCommandChatResponse(result, stdoutBuf.String(), stderrBuf.String(), execErr.Error())
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
			"stdout":    truncateOutput(stdoutBuf.String()),
			"stderr":    truncateOutput(stderrBuf.String()),
		})
	}
	for _, memory := range rememberCapabilityMemoriesFromObservations(session, result.Observations) {
		emitEvent("capability_memory_stored", "Stored structured self-correction capability memory", map[string]string{
			"kind":    memory.Kind,
			"content": truncateOutput(memory.Content),
		})
	}
	assistantResponse = a.reviewFinalResponse(context.Background(), input, assistantResponse, structuredResponseReviewEvidence(result, stdoutBuf.String(), stderrBuf.String(), execErr), emitEvent)
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
	statusLabel := "Exit code"
	if result.PartialProgress && strings.TrimSpace(errText) != "" {
		statusLabel = "Last command exit code"
	}
	lines := []string{
		"Command: " + result.Command,
		fmt.Sprintf("%s: %d", statusLabel, result.ExitCode),
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
		if result.PartialProgress {
			if pending := pendingStructuredObjectiveIDs(result.ObjectiveLedger); pending != "" {
				lines = append(lines, "Pending objectives: "+pending)
			}
			lines = append(lines, "Planner error after progress: "+errText)
		} else {
			lines = append(lines, "Error: "+errText)
		}
		if diagnosis := classifyStructuredLLMFailure(errors.New(errText)); diagnosis != "ollama_request_failure" {
			lines = append(lines, "Diagnosis: "+diagnosis)
		}
	}
	return strings.Join(lines, "\n")
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
			n, err := syscall.Read(fd, buffer)
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

func enableTerminalCbreak(fd int) (func(), error) {
	var original syscall.Termios
	if err := ioctlTermios(fd, syscall.TCGETS, &original); err != nil {
		return nil, err
	}
	next := original
	next.Lflag &^= syscall.ICANON | syscall.ECHO
	next.Cc[syscall.VMIN] = 1
	next.Cc[syscall.VTIME] = 0
	if err := ioctlTermios(fd, syscall.TCSETS, &next); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = ioctlTermios(fd, syscall.TCSETS, &original)
		})
	}, nil
}

func ioctlTermios(fd int, request uintptr, termios *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(unsafe.Pointer(termios)))
	if errno != 0 {
		return errno
	}
	return nil
}

func pollTerminalInput(fd int, timeout time.Duration) (bool, error) {
	var readFDs syscall.FdSet
	fdSet(fd, &readFDs)
	timeval := syscall.NsecToTimeval(timeout.Nanoseconds())
	n, err := syscall.Select(fd+1, &readFDs, nil, nil, &timeval)
	if err != nil {
		if err == syscall.EINTR {
			return false, nil
		}
		return false, err
	}
	if n <= 0 {
		return false, nil
	}
	return fdIsSet(fd, &readFDs), nil
}

func fdSet(fd int, set *syscall.FdSet) {
	index := fd / 64
	offset := uint(fd % 64)
	if index >= 0 && index < len(set.Bits) {
		set.Bits[index] |= 1 << offset
	}
}

func fdIsSet(fd int, set *syscall.FdSet) bool {
	index := fd / 64
	offset := uint(fd % 64)
	if index < 0 || index >= len(set.Bits) {
		return false
	}
	return set.Bits[index]&(1<<offset) != 0
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
	result, err := ResearchWebToMemory(ctx, query, a.web, a.memory, WebResearchMemoryConfig{
		AgentID: "omni_research_manager",
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
		"stored_agent": "omni_research_manager",
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
