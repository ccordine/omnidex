package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/operation"
)

type repositoryGoVerificationRequest struct {
	Args    []string
	Timeout time.Duration
}

func repositoryGoVerificationRequestFromCommand(
	command testCommand,
) (repositoryGoVerificationRequest, error) {
	if command.Name != "go" {
		return repositoryGoVerificationRequest{}, fmt.Errorf(
			"repository Go verification requires the exact go executable; received %q",
			command.Name,
		)
	}
	return repositoryGoVerificationRequest{
		Args: append([]string(nil), command.Args...), Timeout: command.Timeout,
	}, nil
}

func executeRepositoryGoVerificationWithConfig(
	ctx context.Context,
	projection repositoryWorkspaceProjection,
	request repositoryGoVerificationRequest,
	config repositoryGoSandboxConfig,
	moduleView *repositoryGoModuleView,
) (operation.Result, error) {
	if ctx == nil {
		return operation.Result{}, fmt.Errorf("repository Go verification requires a context")
	}
	if err := ctx.Err(); err != nil {
		return operation.Result{}, fmt.Errorf(
			"repository Go verification canceled because step authority ended: %w", err,
		)
	}
	args, timeout, err := prepareRepositoryGoVerificationRequest(request)
	if err != nil {
		return operation.Result{}, err
	}
	if err := config.validateExecution(); err != nil {
		return operation.Result{}, err
	}
	if err := projection.VerifyExact(ctx); err != nil {
		return operation.Result{}, err
	}
	if err := moduleView.requireSource(projection.source.Root); err != nil {
		return operation.Result{}, err
	}
	if err := moduleView.VerifyExact(ctx); err != nil {
		return operation.Result{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, runErr := runRepositoryGoSandbox(runCtx, projection, args, config, moduleView)
	driftErr := projection.VerifyExact(ctx)
	moduleDriftErr := moduleView.VerifyExact(ctx)
	if driftErr != nil {
		runErr = errors.Join(runErr, driftErr)
	}
	if moduleDriftErr != nil {
		runErr = errors.Join(runErr, moduleDriftErr)
	}
	return result, runErr
}

func prepareRepositoryGoVerificationRequest(
	request repositoryGoVerificationRequest,
) ([]string, time.Duration, error) {
	args := append([]string(nil), request.Args...)
	if err := validateV3Command("go", args); err != nil {
		return nil, 0, operation.Reject(err)
	}
	if len(args) < 1 || args[0] != "test" {
		return nil, 0, operation.Reject(fmt.Errorf("repository verification sandbox permits only go test"))
	}
	if !registeredRepositoryGoTestArguments(args) {
		return nil, 0, operation.Reject(fmt.Errorf(
			"repository verification sandbox requires one code-owned structured Go test command",
		))
	}
	timeout := request.Timeout
	if timeout == 0 {
		timeout = defaultV3CommandLimit
	}
	if timeout <= 0 || timeout > maxV3CommandLimit {
		return nil, 0, operation.Reject(fmt.Errorf(
			"repository Go verification timeout must be between 1 and %d seconds",
			int(maxV3CommandLimit/time.Second),
		))
	}
	return args, timeout, nil
}

func registeredRepositoryGoTestArguments(args []string) bool {
	if len(args) == 4 {
		return args[0] == "test" && args[1] == "-json" &&
			args[2] == "-count=1" && args[3] == "./..."
	}
	if len(args) != 6 || args[0] != "test" || args[1] != "-json" ||
		args[2] != "-count=1" || args[3] != "-run" {
		return false
	}
	selector := args[4]
	packageArgument := args[5]
	_, selectorErr := regexp.Compile(selector)
	return selectorErr == nil && len([]byte(selector)) <= maxExistingRepositoryTestSelectorBytes &&
		strings.HasPrefix(selector, "^") && strings.HasSuffix(selector, "$") &&
		(packageArgument == "." || strings.HasPrefix(packageArgument, "./")) &&
		packageArgument != "./..." && !strings.Contains(packageArgument, "...")
}

func runRepositoryGoSandbox(
	ctx context.Context,
	projection repositoryWorkspaceProjection,
	goArgs []string,
	config repositoryGoSandboxConfig,
	moduleView *repositoryGoModuleView,
) (operation.Result, error) {
	rootHandle, err := openRepositorySandboxDirectory(projection.source.Root, "repository projection source")
	if err != nil {
		return operation.Result{}, err
	}
	defer rootHandle.Close()
	extraFiles := []*os.File{rootHandle}
	baseFD := 3
	deltaFD := -1
	mountRoots := repositoryWorkspaceProjectionMountRoots{
		base: fmt.Sprintf("/proc/self/fd/%d", rootHandle.Fd()),
	}
	if projection.deltaRoot != "" {
		deltaHandle, openErr := openRepositorySandboxDirectory(
			projection.deltaRoot, "repository projection delta",
		)
		if openErr != nil {
			return operation.Result{}, openErr
		}
		defer deltaHandle.Close()
		deltaFD = 3 + len(extraFiles)
		mountRoots.delta = fmt.Sprintf("/proc/self/fd/%d", deltaHandle.Fd())
		extraFiles = append(extraFiles, deltaHandle)
	}
	goRootHandle, err := openRepositorySandboxDirectory(config.GoRoot, "system Go toolchain")
	if err != nil {
		return operation.Result{}, err
	}
	defer goRootHandle.Close()
	goRootFD := 3 + len(extraFiles)
	extraFiles = append(extraFiles, goRootHandle)
	cacheHandle, err := openRepositorySandboxDirectory(moduleView.Root(), "exact Go module view")
	if err != nil {
		return operation.Result{}, err
	}
	defer cacheHandle.Close()
	moduleCacheFD := 3 + len(extraFiles)
	extraFiles = append(extraFiles, cacheHandle)
	infoReader, infoWriter, err := os.Pipe()
	if err != nil {
		return operation.Result{}, fmt.Errorf("create repository sandbox status pipe: %w", err)
	}
	defer infoReader.Close()
	defer infoWriter.Close()
	argumentsFD := 3 + len(extraFiles)
	infoFD := argumentsFD + 1
	arguments, err := repositoryGoSandboxArguments(
		projection, mountRoots, baseFD, deltaFD, goRootFD, moduleCacheFD, infoFD,
	)
	if err != nil {
		return operation.Result{}, err
	}
	invocation, err := repositoryBubblewrapInvocation(arguments, argumentsFD, goArgs)
	if err != nil {
		return operation.Result{}, err
	}
	argumentsFile, err := createRepositoryBubblewrapArgumentsFile(arguments)
	if err != nil {
		return operation.Result{}, err
	}
	defer argumentsFile.Close()
	extraFiles = append(extraFiles, argumentsFile, infoWriter)
	command := exec.CommandContext(
		ctx,
		config.BubblewrapPath,
		invocation...,
	)
	command.Env = []string{"PATH=/usr/bin:/bin"}
	command.ExtraFiles = extraFiles
	stdout := newExactRepositoryCommandOutput(maxRepositoryGoVerificationStdoutBytes)
	stderr := newExactRepositoryCommandOutput(maxRepositoryGoVerificationStderrBytes)
	command.Stdout = stdout
	command.Stderr = stderr
	started := time.Now()
	if err := command.Start(); err != nil {
		_ = infoWriter.Close()
		return operation.Result{}, fmt.Errorf("start repository verification sandbox: %w", err)
	}
	if err := infoWriter.Close(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return operation.Result{}, fmt.Errorf("close repository sandbox status authority: %w", err)
	}
	infoChannel := make(chan []byte, 1)
	go func() {
		content, _ := io.ReadAll(io.LimitReader(infoReader, 16*1024))
		infoChannel <- content
	}()
	runErr := command.Wait()
	duration := time.Since(started)
	info := <-infoChannel
	if err := errors.Join(
		stdout.Validate("repository verification stdout"),
		stderr.Validate("repository verification stderr"),
	); err != nil {
		return operation.Result{}, err
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result := repositoryGoVerificationResult(goArgs, stdout.String(), stderr.String(), duration, runErr)
		return result, fmt.Errorf("repository Go verification timed out after %s", duration.Round(time.Second))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		result := repositoryGoVerificationResult(goArgs, stdout.String(), stderr.String(), duration, runErr)
		return result, fmt.Errorf("repository Go verification canceled because step authority ended: %w", ctx.Err())
	}
	if len(bytes.TrimSpace(info)) == 0 || !json.Valid(bytes.TrimSpace(info)) {
		return operation.Result{}, fmt.Errorf(
			"repository verification sandbox failed before starting the exact Go command: %s",
			repositorySandboxStartupDiagnostic(stderr.String()),
		)
	}
	var exitError *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitError) {
		return operation.Result{}, fmt.Errorf("wait for repository verification sandbox: %w", runErr)
	}
	result := repositoryGoVerificationResult(goArgs, stdout.String(), stderr.String(), duration, runErr)
	if exitError != nil && exitError.ExitCode() != 1 {
		return result, fmt.Errorf(
			"repository Go verification ended with unregistered exit semantics %d",
			exitError.ExitCode(),
		)
	}
	return result, nil
}

func repositorySandboxStartupDiagnostic(value string) string {
	const edgeBytes = 1200
	if len(value) <= edgeBytes*2 {
		return value
	}
	return value[:edgeBytes] + "\n...[middle omitted]...\n" + value[len(value)-edgeBytes:]
}

func createRepositoryBubblewrapArgumentsFile(arguments []string) (*os.File, error) {
	if err := validateRepositoryWorkspaceProjectionArguments(arguments); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "omnidex-bwrap-arguments-*")
	if err != nil {
		return nil, fmt.Errorf("create repository sandbox argument authority: %w", err)
	}
	path := file.Name()
	if err := os.Remove(path); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("unlink repository sandbox argument authority: %w", err)
	}
	for _, argument := range arguments {
		if _, err := file.Write(append([]byte(argument), 0)); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("write repository sandbox argument authority: %w", err)
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("rewind repository sandbox argument authority: %w", err)
	}
	return file, nil
}

func repositoryGoVerificationResult(
	args []string,
	stdout string,
	stderr string,
	duration time.Duration,
	runErr error,
) operation.Result {
	exitCode := 0
	var exitError *exec.ExitError
	if errors.As(runErr, &exitError) {
		exitCode = exitError.ExitCode()
	} else if runErr != nil {
		exitCode = -1
	}
	succeeded := runErr == nil
	summary := fmt.Sprintf(
		"command go exit_code=%d duration_ms=%d", exitCode, duration.Milliseconds(),
	)
	warnings := []string(nil)
	if runErr != nil {
		warnings = []string{"command failed: " + runErr.Error()}
	}
	commandText := strings.Join(append([]string{"go"}, args...), " ")
	return operation.Result{
		Summary:  summary,
		Warnings: warnings,
		Output: map[string]any{
			"summary": summary, "program": "go", "args": append([]string(nil), args...),
			"exit_code": exitCode, "stdout": stdout, "stderr": stderr,
			"duration_ms": duration.Milliseconds(), "succeeded": succeeded,
		},
		Evidence: []evidence.Record{{
			Kind: evidence.KindTestResult, SourceType: "command", SourceRef: "go",
			Command: commandText,
			Excerpt: "stdout:\n" + stdout + "\nstderr:\n" + stderr,
			Summary: summary, Confidence: 1, Warnings: warnings,
			Metadata: map[string]any{
				"execution": true, "side_effect_possible": false, "succeeded": succeeded,
				"exit_code": exitCode, "duration_ms": duration.Milliseconds(),
				"sandbox": "bubblewrap-v1", "network": "isolated",
				"stdout_bound_bytes": maxRepositoryGoVerificationStdoutBytes,
				"stderr_bound_bytes": maxRepositoryGoVerificationStderrBytes,
			},
		}},
	}
}
