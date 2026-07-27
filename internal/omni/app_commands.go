package omni

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
