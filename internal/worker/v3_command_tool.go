package worker

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/operation"
)

const (
	maxV3CommandArgs      = 32
	maxV3CommandOutput    = 12 * 1024
	defaultV3CommandLimit = 120 * time.Second
	maxV3CommandLimit     = 5 * time.Minute
)

var (
	v3GoModulePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~+/-]*$`)
	v3JavaClassPattern  = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
	v3JavaMethodPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
)

type codeCommand struct {
	Program string
	Args    []string
	Timeout time.Duration
}

func executeCodeCommandAtRoot(
	ctx context.Context,
	root string,
	command codeCommand,
) (operation.Result, error) {
	if ctx == nil {
		return operation.Result{}, fmt.Errorf("command.run requires a context")
	}
	program := strings.TrimSpace(command.Program)
	args := append([]string(nil), command.Args...)
	if err := validateV3Command(program, args); err != nil {
		return operation.Result{}, operation.Reject(err)
	}
	execution, err := runValidatedV3Command(ctx, root, command)
	if err != nil {
		return operation.Result{}, err
	}
	timeout := command.Timeout
	if timeout == 0 {
		timeout = defaultV3CommandLimit
	}
	if execution.ContextError == context.DeadlineExceeded {
		return operation.Result{}, fmt.Errorf("command.run timed out after %s", timeout)
	}
	if execution.ContextError == context.Canceled {
		return operation.Result{}, fmt.Errorf(
			"command.run canceled because step authority ended: %w", execution.ContextError,
		)
	}
	succeeded := execution.RunError == nil
	commandText := strings.Join(append([]string{program}, args...), " ")
	summary := fmt.Sprintf("command %s exit_code=%d duration_ms=%d", program, execution.ExitCode, execution.Duration.Milliseconds())
	warnings := []string(nil)
	if execution.RunError != nil {
		warnings = []string{"command failed: " + execution.RunError.Error()}
	}
	kind := evidence.KindCommandOutput
	if isV3TestCommand(program, args) {
		kind = evidence.KindTestResult
	}
	return operation.Result{
		Summary: summary, Warnings: warnings,
		Output: map[string]any{
			"summary": summary, "program": program, "args": args, "exit_code": execution.ExitCode, "stdout": execution.Stdout,
			"stderr": execution.Stderr, "stdout_observed_bytes": execution.StdoutBytes,
			"stderr_observed_bytes": execution.StderrBytes, "stdout_truncated": execution.StdoutTruncated,
			"stderr_truncated": execution.StderrTruncated, "duration_ms": execution.Duration.Milliseconds(), "succeeded": succeeded,
		},
		Evidence: []evidence.Record{{
			Kind: kind, SourceType: "command", SourceRef: program, Command: commandText,
			Excerpt: trimForBudget("stdout:\n"+execution.Stdout+"\nstderr:\n"+execution.Stderr, maxV3CommandOutput), Summary: summary,
			Confidence: 1, Warnings: warnings,
			Metadata: map[string]any{
				"execution": true, "side_effect_possible": true, "succeeded": succeeded,
				"exit_code": execution.ExitCode, "duration_ms": execution.Duration.Milliseconds(), "workspace": root,
				"stdout_observed_bytes": execution.StdoutBytes, "stderr_observed_bytes": execution.StderrBytes,
				"stdout_truncated": execution.StdoutTruncated, "stderr_truncated": execution.StderrTruncated,
			},
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
	if directCodingVersionProbeCommand(program, args) {
		return nil
	}
	verb := strings.ToLower(strings.TrimSpace(args[0]))
	allowed := map[string]map[string]struct{}{
		"go": {"build": {}, "fmt": {}, "list": {}, "mod": {}, "test": {}, "vet": {}}, "git": {"diff": {}, "log": {}, "rev-parse": {}, "show": {}, "status": {}},
		"node":  {"--permission": {}},
		"javac": {"--release": {}}, "java": {"-ea": {}}, "jar": {"--create": {}},
		"npm":    {"ci": {}, "init": {}, "run": {}, "test": {}}, "pnpm": {"run": {}, "test": {}}, "yarn": {"run": {}, "test": {}},
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
	if program == "cargo" {
		if err := validateV3Cargo(args); err != nil {
			return err
		}
	}
	if program == "npm" && verb == "init" && (len(args) != 2 || args[1] != "--yes" && args[1] != "-y") {
		return fmt.Errorf("command.run permits npm init only as npm init --yes")
	}
	if program == "npm" && verb == "ci" && !slicesEqualStrings(args, directCodingNPMInstallArgs()) {
		return fmt.Errorf("command.run permits npm ci only with the code-owned locked no-script verification arguments")
	}
	if program == "node" {
		if err := validateV3Node(args); err != nil {
			return err
		}
	}
	if program == "javac" {
		if err := validateV3JavaCompile(args); err != nil {
			return err
		}
	}
	if program == "java" {
		if len(args) != 6 || args[0] != "-ea" || args[1] != "-cp" ||
			args[2] != "build/classes" || args[3] != "TestRunner" ||
			!v3JavaClassPattern.MatchString(args[4]) || !v3JavaMethodPattern.MatchString(args[5]) {
			return fmt.Errorf("command.run permits java only through the code-owned TestRunner")
		}
	}
	if program == "jar" && !slicesEqualStrings(args, []string{
		"--create", "--file", "build/application.jar", "--main-class", "Main",
		"-C", "build/classes", ".",
	}) {
		return fmt.Errorf("command.run permits only the code-owned Java application archive command")
	}
	if program == "python3" && (len(args) < 2 || args[0] != "-m" || args[1] != "pytest") {
		return fmt.Errorf("command.run permits python3 only as python3 -m pytest")
	}
	if (program == "npm" || program == "pnpm" || program == "yarn") && verb == "run" && (len(args) < 2 || !v3VerificationScriptName(args[1])) {
		return fmt.Errorf("command.run permits package scripts only when their name is test, check, lint, build, typecheck, or verify oriented")
	}
	if program == "git" && verb == "diff" && (!containsString(args[1:], "--no-ext-diff") || !containsString(args[1:], "--no-textconv")) {
		return fmt.Errorf("command.run requires git diff --no-ext-diff --no-textconv to disable repository-configured executors")
	}
	return nil
}

func validateV3JavaCompile(args []string) error {
	if len(args) < 7 || args[0] != "--release" || !directCodingRegisteredJavaRelease(args[1]) ||
		args[2] != "-Xlint:all" || args[3] != "-Werror" ||
		args[4] != "-d" || args[5] != "build/classes" {
		return fmt.Errorf("command.run permits javac only with strict code-owned output arguments")
	}
	last := ""
	for _, sourcePath := range args[6:] {
		clean := filepath.ToSlash(filepath.Clean(sourcePath))
		if clean != sourcePath || !strings.HasSuffix(sourcePath, ".java") ||
			(last != "" && sourcePath <= last) {
			return fmt.Errorf("command.run javac sources must be unique normalized Java paths in order")
		}
		last = sourcePath
	}
	return nil
}

func directCodingVersionProbeCommand(program string, args []string) bool {
	switch program {
	case "node", "npm", "rustc", "cargo":
		return slicesEqualStrings(args, []string{"--version"})
	case "go":
		return slicesEqualStrings(args, []string{"version"})
	case "java":
		return slicesEqualStrings(args, []string{"-version"})
	case "jar":
		return slicesEqualStrings(args, []string{"--version"})
	case "javac":
		return len(args) == 3 && args[0] == "--release" &&
			directCodingRegisteredJavaRelease(args[1]) && args[2] == "-version"
	default:
		return false
	}
}

func directCodingRegisteredJavaRelease(release string) bool {
	for _, profile := range registeredDirectCodingProjectVersionProfiles() {
		if profile.StackID != genericJavaCommandLineAdapter {
			continue
		}
		value, err := directCodingVersionComponent(profile, "java_release")
		if err == nil && value == release {
			return true
		}
	}
	return false
}

func javascriptArtifactPath(value string) bool {
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean != value || clean == "." || strings.HasPrefix(clean, "../") {
		return false
	}
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".js", ".jsx", ".mjs", ".cjs":
		return true
	default:
		return false
	}
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

func v3CommandPathCandidate(argument string) string {
	candidate := strings.TrimSpace(argument)
	if strings.HasPrefix(candidate, "-") {
		if _, rhs, found := strings.Cut(candidate, "="); found {
			candidate = strings.TrimSpace(rhs)
		}
	}
	return candidate
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
