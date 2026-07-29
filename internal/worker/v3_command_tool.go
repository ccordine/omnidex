package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

const (
	maxV3CommandArgs      = 32
	maxV3CommandOutput    = 12 * 1024
	defaultV3CommandLimit = 120 * time.Second
	maxV3CommandLimit     = 5 * time.Minute
)

var (
	v3GoModulePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~+/-]*$`)
	v3CargoNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
)

func commandRunToolSpec() toolruntime.Spec {
	return toolruntime.Spec{
		Name:        "command.run",
		Description: "Run one shell-free, allowlisted workspace initializer, build, or verification command in the server-authoritative job workspace.",
		InputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"program", "args"},
			Properties: map[string]toolruntime.Schema{
				"program": {Type: "string"}, "args": {Type: "array", Items: &toolruntime.Schema{Type: "string"}}, "timeout_seconds": {Type: "integer"},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"summary", "program", "args", "exit_code", "stdout", "stderr", "duration_ms", "succeeded"},
			Properties: map[string]toolruntime.Schema{
				"summary": {Type: "string"}, "program": {Type: "string"}, "args": {Type: "array", Items: &toolruntime.Schema{Type: "string"}},
				"exit_code": {Type: "integer"}, "stdout": {Type: "string"}, "stderr": {Type: "string"}, "duration_ms": {Type: "integer"}, "succeeded": {Type: "boolean"},
			},
		},
		Examples: []toolruntime.Example{
			{When: "Initialize a Go module in an empty workspace.", Input: map[string]any{"program": "go", "args": []string{"mod", "init", "example"}}},
			{When: "Initialize a Rust binary crate without creating nested version-control state.", Input: map[string]any{"program": "cargo", "args": []string{"init", "--name", "example", "--vcs", "none", "."}}},
			{When: "Initialize package metadata without an interactive prompt.", Input: map[string]any{"program": "npm", "args": []string{"init", "--yes"}}},
			{When: "Verify a Go workspace after applying a patch.", Input: map[string]any{"program": "go", "args": []string{"test", "./..."}, "timeout_seconds": 180}},
			{When: "Inspect the exact source diff without invoking repository-configured external helpers.", Input: map[string]any{"program": "git", "args": []string{"diff", "--no-ext-diff", "--no-textconv", "--"}}},
		},
		RequireEvidence: true,
	}
}

func executeV3Command(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
	scope, err := v3WorkspaceScopeFromContext(ctx)
	if err != nil {
		return toolruntime.Result{}, err
	}
	program := strings.TrimSpace(toolInputString(call.Input, "program"))
	args, err := strictV3StringArray(call.Input["args"], "args")
	if err != nil {
		return toolruntime.Result{}, toolruntime.RejectCall(err)
	}
	if err := validateV3Command(program, args); err != nil {
		return toolruntime.Result{}, toolruntime.RejectCall(err)
	}
	timeout := time.Duration(toolInputInt(call.Input, "timeout_seconds", int(defaultV3CommandLimit/time.Second))) * time.Second
	if timeout <= 0 || timeout > maxV3CommandLimit {
		return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("command.run timeout must be between 1 and %d seconds", int(maxV3CommandLimit/time.Second)))
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	cmd := exec.CommandContext(runCtx, program, args...)
	cmd.Dir = scope.Root
	cmd.Env = v3CommandEnvironment(os.Environ())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	duration := time.Since(started)
	exitCode := 0
	if runErr != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return toolruntime.Result{}, fmt.Errorf("command.run timed out after %s", timeout)
		}
		if errors.Is(runCtx.Err(), context.Canceled) {
			return toolruntime.Result{}, fmt.Errorf("command.run canceled because step authority ended: %w", runCtx.Err())
		}
	}
	stdoutText := trimForBudget(stdout.String(), maxV3CommandOutput)
	stderrText := trimForBudget(stderr.String(), maxV3CommandOutput)
	succeeded := runErr == nil
	commandText := strings.Join(append([]string{program}, args...), " ")
	summary := fmt.Sprintf("command %s exit_code=%d duration_ms=%d", program, exitCode, duration.Milliseconds())
	warnings := []string(nil)
	if runErr != nil {
		warnings = []string{"command failed: " + runErr.Error()}
	}
	kind := evidence.KindCommandOutput
	if isV3TestCommand(program, args) {
		kind = evidence.KindTestResult
	}
	return toolruntime.Result{
		Summary: summary, Warnings: warnings,
		Output: map[string]any{
			"summary": summary, "program": program, "args": args, "exit_code": exitCode, "stdout": stdoutText,
			"stderr": stderrText, "duration_ms": duration.Milliseconds(), "succeeded": succeeded,
		},
		Evidence: []evidence.Record{{
			Kind: kind, SourceType: "command", SourceRef: program, Command: commandText,
			Excerpt: trimForBudget("stdout:\n"+stdoutText+"\nstderr:\n"+stderrText, maxV3CommandOutput), Summary: summary,
			Confidence: 1, Warnings: warnings,
			Metadata: map[string]any{"execution": true, "side_effect_possible": true, "succeeded": succeeded, "exit_code": exitCode, "duration_ms": duration.Milliseconds(), "workspace": scope.Root},
		}},
	}, nil
}

func validateV3Command(program string, args []string) error {
	if program == "" || program != strings.ToLower(program) || filepath.Base(program) != program || strings.ContainsAny(program, `/\\`) {
		return fmt.Errorf("command.run program must be a bare allowlisted executable name")
	}
	if len(args) == 0 {
		return fmt.Errorf("command.run requires an explicit subcommand")
	}
	if len(args) > maxV3CommandArgs {
		return fmt.Errorf("command.run exceeds the %d-argument limit", maxV3CommandArgs)
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, '\x00') || strings.ContainsAny(arg, "\r\n") {
			return fmt.Errorf("command.run arguments cannot contain NUL or newlines")
		}
		candidate := v3CommandPathCandidate(arg)
		cleanCandidate := filepath.Clean(candidate)
		if filepath.IsAbs(candidate) || cleanCandidate == ".." || strings.HasPrefix(cleanCandidate, ".."+string(filepath.Separator)) || strings.HasPrefix(filepath.ToSlash(cleanCandidate), "../") {
			return fmt.Errorf("command.run argument %q escapes the authoritative workspace", arg)
		}
	}
	verb := strings.ToLower(strings.TrimSpace(args[0]))
	allowed := map[string]map[string]struct{}{
		"go": {"build": {}, "fmt": {}, "list": {}, "mod": {}, "test": {}, "version": {}, "vet": {}}, "git": {"diff": {}, "log": {}, "rev-parse": {}, "show": {}, "status": {}},
		"npm": {"init": {}, "install": {}, "run": {}, "test": {}}, "pnpm": {"run": {}, "test": {}}, "yarn": {"run": {}, "test": {}},
		"cargo": {"build": {}, "check": {}, "clippy": {}, "fmt": {}, "init": {}, "test": {}}, "pytest": {verb: {}}, "python3": {"-m": {}},
		"dotnet": {"build": {}, "test": {}}, "mvn": {"test": {}, "verify": {}}, "gradle": {"build": {}, "check": {}, "test": {}},
		"phpunit": {verb: {}}, "composer": {"test": {}, "validate": {}},
	}
	verbs, ok := allowed[strings.ToLower(program)]
	if !ok {
		return fmt.Errorf("command.run program %q is not allowlisted", program)
	}
	if _, ok := verbs[verb]; !ok {
		return fmt.Errorf("command.run subcommand %q is not allowlisted for %s", verb, program)
	}
	if program == "go" && verb == "mod" {
		if len(args) != 3 || args[1] != "init" || !v3GoModulePattern.MatchString(args[2]) {
			return fmt.Errorf("command.run permits go mod only as go mod init <module>")
		}
	}
	if program == "cargo" && verb == "init" {
		if err := validateV3CargoInit(args); err != nil {
			return err
		}
	}
	if program == "npm" && verb == "init" && (len(args) != 2 || args[1] != "--yes" && args[1] != "-y") {
		return fmt.Errorf("command.run permits npm init only as npm init --yes")
	}
	if program == "npm" && verb == "install" && !slicesEqualStrings(args, directCodingNPMInstallArgs()) {
		return fmt.Errorf("command.run permits npm install only with the code-owned no-script verification arguments")
	}
	if program == "python3" && (len(args) < 2 || args[0] != "-m" || args[1] != "pytest") {
		return fmt.Errorf("command.run permits python3 only as python3 -m pytest")
	}
	if (program == "npm" || program == "pnpm" || program == "yarn") && verb == "run" && (len(args) < 2 || !v3VerificationScriptName(args[1])) {
		return fmt.Errorf("command.run permits package scripts only when their name is test, check, lint, build, typecheck, or verify oriented")
	}
	if program == "cargo" && verb == "fmt" && !containsString(args[1:], "--check") {
		return fmt.Errorf("command.run permits cargo fmt only with --check")
	}
	if program == "git" && verb == "diff" && (!containsString(args[1:], "--no-ext-diff") || !containsString(args[1:], "--no-textconv")) {
		return fmt.Errorf("command.run requires git diff --no-ext-diff --no-textconv to disable repository-configured executors")
	}
	return nil
}

func slicesEqualStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateV3CargoInit(args []string) error {
	seenName := false
	seenVCS := false
	seenTarget := false
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--bin", "--lib":
			continue
		case "--name":
			if seenName || index+1 >= len(args) || !v3CargoNamePattern.MatchString(args[index+1]) {
				return fmt.Errorf("command.run cargo init requires a valid unique --name value")
			}
			seenName = true
			index++
		case "--vcs":
			if seenVCS || index+1 >= len(args) || args[index+1] != "none" {
				return fmt.Errorf("command.run cargo init permits only --vcs none")
			}
			seenVCS = true
			index++
		case "--edition":
			if index+1 >= len(args) || !containsString([]string{"2015", "2018", "2021", "2024"}, args[index+1]) {
				return fmt.Errorf("command.run cargo init requires a supported --edition value")
			}
			index++
		case ".":
			if seenTarget {
				return fmt.Errorf("command.run cargo init accepts at most one current-directory target")
			}
			seenTarget = true
		default:
			return fmt.Errorf("command.run cargo init argument %q is not allowlisted", args[index])
		}
	}
	return nil
}

func v3CommandPathCandidate(argument string) string {
	candidate := strings.TrimSpace(argument)
	if strings.HasPrefix(candidate, "-") {
		if _, rhs, found := strings.Cut(candidate, "="); found {
			candidate = strings.TrimSpace(rhs)
		}
	}
	return candidate
}

func v3CommandEnvironment(current []string) []string {
	out := make([]string, 0, len(current)+2)
	for _, item := range current {
		key, _, _ := strings.Cut(item, "=")
		if key == "GIT_PAGER" || key == "PAGER" {
			continue
		}
		out = append(out, item)
	}
	return append(out, "GIT_PAGER=cat", "PAGER=cat")
}

func strictV3StringArray(value any, field string) ([]string, error) {
	var raw []string
	switch typed := value.(type) {
	case []string:
		raw = append(raw, typed...)
	case []any:
		raw = make([]string, 0, len(typed))
		for index, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("command.run %s[%d] must be a string", field, index)
			}
			raw = append(raw, text)
		}
	default:
		return nil, fmt.Errorf("command.run %s must be a string array", field)
	}
	return raw, nil
}

func v3VerificationScriptName(value string) bool {
	name := strings.ToLower(strings.TrimSpace(value))
	for _, token := range []string{"test", "check", "lint", "build", "typecheck", "verify"} {
		if name == token || strings.HasPrefix(name, token+":") || strings.HasSuffix(name, ":"+token) || strings.Contains(name, "-"+token) || strings.Contains(name, token+"-") {
			return true
		}
	}
	return false
}

func isV3TestCommand(program string, args []string) bool {
	joined := strings.ToLower(strings.Join(append([]string{program}, args...), " "))
	return strings.Contains(joined, " test") || strings.Contains(joined, "pytest") || strings.Contains(joined, "phpunit") || strings.Contains(joined, " verify")
}
