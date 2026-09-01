package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

const directCodingVerificationStreamLimit = 1024 * 1024
const directCodingVerificationEvidencePersistenceTimeout = 10 * time.Second

type directCodingVerificationCommandResult struct {
	Stdout []byte
	Stderr []byte
}

type directCodingBoundedCommandBuffer struct {
	buffer   bytes.Buffer
	overflow bool
}

func (buffer *directCodingBoundedCommandBuffer) Write(value []byte) (int, error) {
	available := directCodingVerificationStreamLimit - buffer.buffer.Len()
	if available > 0 {
		written := len(value)
		if written > available {
			written = available
		}
		_, _ = buffer.buffer.Write(value[:written])
	}
	if len(value) > available {
		buffer.overflow = true
	}
	return len(value), nil
}

func (s *directCodingSession) runRecordedVerificationCommand(
	root string,
	phase queue.VerificationCommandPhase,
	command directCodingVerificationCommand,
	trackWorkspace bool,
) (result directCodingVerificationCommandResult, resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.svc == nil || s.runtime.svc.repo == nil ||
		s.runtime.claim == nil || s.runtime.ctx == nil {
		return result, fmt.Errorf("verification command requires one active persisted step authority")
	}
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return result, fmt.Errorf("verification command requires one canonical absolute working directory")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return result, fmt.Errorf("inspect verification working directory: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("verification working directory is not one exact directory")
	}
	commandDirectory := root
	var observationFence *workspacefacts.MutationFence
	ownedObservationFence := false
	defer func() {
		if ownedObservationFence {
			if releaseErr := observationFence.Release(); releaseErr != nil {
				resultErr = errors.Join(
					resultErr,
					fmt.Errorf("release isolated verification root authority: %w", releaseErr),
				)
			}
		}
	}()
	if directCodingVerificationPhaseUsesHostRoot(phase) {
		if root != s.root {
			return result, fmt.Errorf("host verification command root differs from the coding session root")
		}
		if err := s.runtime.requireWorkspaceMutationFence(); err != nil {
			return result, err
		}
		observationFence = s.runtime.workspaceFence
		commandDirectory, err = observationFence.CommandWorkingDirectory(root)
		if err != nil {
			return result, fmt.Errorf("anchor host verification command cwd: %w", err)
		}
	} else if trackWorkspace {
		observationFence, err = workspacefacts.AcquireMutationFence(s.runtime.ctx, root)
		if err != nil {
			return result, fmt.Errorf("acquire isolated verification root authority: %w", err)
		}
		ownedObservationFence = true
	}
	if len(command.Argv) == 0 || strings.TrimSpace(command.Argv[0]) == "" {
		return result, fmt.Errorf("verification command argv is empty")
	}
	if command.Timeout <= 0 || command.Timeout > 10*time.Minute {
		return result, fmt.Errorf("verification command timeout is outside the code-owned bound")
	}
	if !sort.StringsAreSorted(command.Environment) {
		return result, fmt.Errorf("verification command environment overrides are not sorted")
	}
	processEnvironment, err := directCodingVerificationProcessEnvironment(command.Environment)
	if err != nil {
		return result, err
	}
	workspaceBefore := ""
	if trackWorkspace {
		workspaceBefore, err = directCodingAuthoritativeWorkspaceSHA256(observationFence, root)
		if err != nil {
			return result, fmt.Errorf("hash authoritative workspace before verification command: %w", err)
		}
	}
	ordinal, err := s.nextVerificationCommandOrdinal()
	if err != nil {
		return result, err
	}
	commandContext, cancel := directCodingVerificationCommandContext(
		s.runtime.ctx, phase, command.Timeout,
	)
	defer cancel()
	if directCodingVerificationPhaseUsesHostRoot(phase) {
		commandDirectory, err = observationFence.CommandWorkingDirectory(root)
		if err != nil {
			return result, fmt.Errorf("reattest fd-rooted host command cwd before launch: %w", err)
		}
	}
	process := exec.Command(command.Argv[0], command.Argv[1:]...)
	process.Dir = commandDirectory
	process.Env = processEnvironment
	if command.Stdin != nil {
		process.Stdin = bytes.NewReader(command.Stdin)
	}
	var stdout directCodingBoundedCommandBuffer
	var stderr directCodingBoundedCommandBuffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	startedAt := directCodingVerificationTimestamp(time.Now())
	runErr := runDirectCodingVerificationProcess(commandContext, process)
	finishedAt := directCodingVerificationTimestamp(time.Now())
	result.Stdout = append([]byte{}, stdout.buffer.Bytes()...)
	result.Stderr = append([]byte{}, stderr.buffer.Bytes()...)
	workspaceAfter := ""
	observationError := ""
	if trackWorkspace {
		workspaceAfter, observationError = directCodingVerificationWorkspaceAfter(
			observationFence, root,
		)
	}
	if directCodingVerificationPhaseUsesHostRoot(phase) {
		if err := observationFence.Reattest(root); err != nil && observationError == "" {
			observationError = trimForBudget(
				"reattest authoritative host workspace after verification command: "+err.Error(),
				4000,
			)
		}
	}
	if ownedObservationFence {
		if err := observationFence.Release(); err != nil && observationError == "" {
			observationError = trimForBudget(
				"release isolated verification root authority: "+err.Error(), 4000,
			)
		}
		ownedObservationFence = false
	}
	exitCode, launchError := directCodingVerificationExit(runErr, commandContext.Err())
	if stdout.overflow || stderr.overflow {
		exitCode = nil
		launchError = "verification command output exceeded the immutable 1 MiB stream evidence bound"
	}
	record := queue.VerificationCommandEvidence{
		Authority:             s.runtime.claim.Authority,
		Phase:                 phase,
		Ordinal:               ordinal,
		Argv:                  append([]string(nil), command.Argv...),
		Environment:           append([]string(nil), processEnvironment...),
		Stdin:                 append([]byte{}, command.Stdin...),
		WorkingDirectory:      root,
		StartedAt:             startedAt,
		FinishedAt:            finishedAt,
		ExitCode:              exitCode,
		LaunchError:           launchError,
		ObservationError:      observationError,
		Stdout:                result.Stdout,
		StdoutComplete:        !stdout.overflow,
		Stderr:                result.Stderr,
		StderrComplete:        !stderr.overflow,
		WorkspaceSHA256Before: workspaceBefore,
		WorkspaceSHA256After:  workspaceAfter,
	}
	if command.Stdin == nil {
		record.Stdin = nil
	}
	persistenceContext, stopPersistence := directCodingVerificationEvidenceContext(s.runtime.ctx)
	defer stopPersistence()
	if err := s.runtime.svc.repo.AppendVerificationCommandEvidence(persistenceContext, record); err != nil {
		return result, fmt.Errorf("persist immutable verification command evidence %d: %w", ordinal, err)
	}
	if observationError != "" {
		return result, fmt.Errorf("verification command %d post-run observation failed: %s", ordinal, observationError)
	}
	if launchError != "" {
		return result, fmt.Errorf("verification command %d could not complete: %s", ordinal, launchError)
	}
	if exitCode == nil {
		return result, fmt.Errorf("verification command %d has no exact exit status", ordinal)
	}
	if *exitCode != 0 {
		return result, fmt.Errorf(
			"verification command %d exited %d: %s",
			ordinal, *exitCode,
			trimForBudget(strings.TrimSpace(string(result.Stderr)), 12_000),
		)
	}
	if trackWorkspace && workspaceBefore != workspaceAfter {
		return result, fmt.Errorf(
			"verification command %d changed authoritative workspace identity from %s to %s",
			ordinal, workspaceBefore, workspaceAfter,
		)
	}
	return result, nil
}

func directCodingVerificationWorkspaceAfter(
	fence *workspacefacts.MutationFence,
	root string,
) (string, string) {
	workspaceAfter, err := directCodingAuthoritativeWorkspaceSHA256(fence, root)
	if err == nil {
		return workspaceAfter, ""
	}
	return "", trimForBudget(
		"hash authoritative workspace after verification command: "+err.Error(),
		4000,
	)
}

func directCodingVerificationPhaseUsesHostRoot(phase queue.VerificationCommandPhase) bool {
	return phase == queue.VerificationHostInstall || phase == queue.VerificationHostFinal ||
		phase == queue.VerificationHostCleanup
}

func directCodingVerificationCommandContext(
	parent context.Context,
	phase queue.VerificationCommandPhase,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if phase == queue.VerificationHostCleanup {
		parent = context.WithoutCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func directCodingVerificationEvidenceContext(
	parent context.Context,
) (context.Context, context.CancelFunc) {
	return context.WithTimeout(
		context.WithoutCancel(parent),
		directCodingVerificationEvidencePersistenceTimeout,
	)
}

func directCodingVerificationExit(
	runErr error,
	contextErr error,
) (*int, string) {
	if contextErr != nil {
		return nil, trimForBudget("verification command context ended: "+contextErr.Error(), 4000)
	}
	if runErr == nil {
		code := 0
		return &code, ""
	}
	var exitError *exec.ExitError
	if errors.As(runErr, &exitError) {
		code := exitError.ExitCode()
		if code >= 0 && code <= 255 {
			return &code, ""
		}
	}
	return nil, trimForBudget("verification command launch or wait failure: "+runErr.Error(), 4000)
}

func directCodingVerificationTimestamp(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func directCodingVerificationProcessEnvironment(overrides []string) ([]string, error) {
	pathValue, exists := os.LookupEnv("PATH")
	if !exists || strings.TrimSpace(pathValue) == "" || strings.ContainsRune(pathValue, 0) {
		return nil, fmt.Errorf("verification command requires one explicit ambient PATH authority")
	}
	values := map[string]string{
		"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8", "PATH": pathValue, "TMPDIR": "/tmp",
		"NPM_CONFIG_USERCONFIG": "/dev/null",
	}
	allowed := map[string]struct{}{
		"CI": {}, "LANG": {}, "LC_ALL": {}, "PATH": {}, "TMPDIR": {},
		"GOCACHE": {}, "GOENV": {}, "GOFLAGS": {}, "GOMODCACHE": {}, "GOPROXY": {},
		"GOSUMDB": {}, "GOTELEMETRY": {}, "GOTOOLCHAIN": {}, "GOWORK": {},
		"NPM_CONFIG_AUDIT": {}, "NPM_CONFIG_CACHE": {}, "NPM_CONFIG_FUND": {},
		"NPM_CONFIG_UPDATE_NOTIFIER": {}, "NPM_CONFIG_USERCONFIG": {},
	}
	names := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		name, value, found := strings.Cut(override, "=")
		if !found || name == "" {
			return nil, fmt.Errorf("verification command environment contains an invalid override")
		}
		if _, permitted := allowed[name]; !permitted {
			return nil, fmt.Errorf("verification command environment name %s is outside the code-owned allowlist", name)
		}
		if _, duplicate := names[name]; duplicate {
			return nil, fmt.Errorf("verification command environment repeats override %s", name)
		}
		names[name] = struct{}{}
		values[name] = value
	}
	environment := make([]string, 0, len(values))
	for name, value := range values {
		if strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("verification command environment %s contains NUL", name)
		}
		environment = append(environment, name+"="+value)
	}
	sort.Strings(environment)
	return environment, nil
}
