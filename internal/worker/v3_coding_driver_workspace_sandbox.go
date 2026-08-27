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
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/operation"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

type directCodingWorkspaceSandbox struct {
	projection repositoryWorkspaceProjection
	root       string
	mounts     []repositoryWorkspaceProjectionMount
	namespace  string
}

func newDirectCodingWorkspaceSandbox(
	ctx context.Context,
	projection repositoryWorkspaceProjection,
	identity string,
) (_ *directCodingWorkspaceSandbox, resultErr error) {
	if ctx == nil || strings.TrimSpace(identity) == "" {
		return nil, fmt.Errorf("direct-coding verification sandbox requires context and identity")
	}
	if err := projection.VerifyExact(ctx); err != nil {
		return nil, err
	}
	mounts, err := repositoryWorkspaceProjectionMounts(
		projection,
		repositoryWorkspaceProjectionMountRoots{
			base: projection.source.Root, delta: projection.deltaRoot,
		},
	)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "omnidex-direct-verification-*")
	if err != nil {
		return nil, fmt.Errorf("create direct-coding verification root: %w", err)
	}
	sandbox := &directCodingWorkspaceSandbox{
		projection: projection, root: root, mounts: mounts,
		namespace: "omnidex" + directCodingDigest(identity)[:16],
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, sandbox.Cleanup())
		}
	}()
	for _, mount := range mounts {
		destination := filepath.Join(root, filepath.FromSlash(mount.Path))
		if mount.Directory {
			if err := os.MkdirAll(destination, 0o700); err != nil {
				return nil, fmt.Errorf("create verification mount directory %q: %w", mount.Path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return nil, fmt.Errorf("create verification mount parent %q: %w", mount.Path, err)
		}
		if mount.Source == repositoryWorkspaceProjectionSymlink {
			if err := os.Symlink(mount.LinkTarget, destination); err != nil {
				return nil, fmt.Errorf("create verification symlink %q: %w", mount.Path, err)
			}
			continue
		}
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create verification mount target %q: %w", mount.Path, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close verification mount target %q: %w", mount.Path, err)
		}
	}
	return sandbox, nil
}

func (sandbox *directCodingWorkspaceSandbox) VerifyAuthority(ctx context.Context) error {
	if sandbox == nil || sandbox.root == "" {
		return fmt.Errorf("direct-coding verification sandbox is incomplete")
	}
	if err := sandbox.projection.validate(); err != nil {
		return err
	}
	if sandbox.projection.stage == nil {
		return sandbox.projection.source.VerifyExact(ctx)
	}
	if err := sandbox.projection.stage.VerifyExactDelta(ctx); err != nil {
		return err
	}
	current, err := workspacefacts.Capture(ctx, sandbox.projection.source.Root)
	if err != nil {
		return err
	}
	entries := make(map[string]workspacefacts.Entry, len(current.Entries))
	for _, entry := range current.Entries {
		entries[entry.Path] = entry
	}
	for _, file := range sandbox.projection.files {
		if file.Source == repositoryWorkspaceProjectionDelta {
			continue
		}
		entry, exists := entries[file.Path]
		if !exists || entry.Kind != file.Kind || entry.SHA256 != file.SHA256 ||
			entry.Size != file.Size || entry.Mode != file.Mode ||
			entry.LinkTarget != file.LinkTarget {
			return fmt.Errorf("direct-coding projection backing %q changed", file.Path)
		}
	}
	return nil
}

func (sandbox *directCodingWorkspaceSandbox) Execute(
	ctx context.Context,
	command testCommand,
) (operation.Result, error) {
	if err := sandbox.VerifyAuthority(ctx); err != nil {
		return operation.Result{}, err
	}
	if err := validateV3Command(command.Name, command.Args); err != nil {
		return operation.Result{}, operation.Reject(err)
	}
	timeout := command.Timeout
	if timeout == 0 {
		timeout = defaultV3CommandLimit
	}
	if timeout <= 0 || timeout > maxV3CommandLimit {
		return operation.Result{}, operation.Reject(fmt.Errorf("verification timeout is invalid"))
	}
	invocation, handles, infoReader, err := sandbox.invocation(ctx, command)
	if err != nil {
		return operation.Result{}, err
	}
	defer infoReader.Close()
	for _, handle := range handles {
		defer handle.Close()
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, err := newBoundedCommandOutput(maxV3CommandOutput)
	if err != nil {
		return operation.Result{}, err
	}
	stderr, err := newBoundedCommandOutput(maxV3CommandOutput)
	if err != nil {
		return operation.Result{}, err
	}
	process := exec.CommandContext(runCtx, repositoryBubblewrapPath, invocation...)
	process.Env = []string{"PATH=/usr/bin:/bin"}
	process.ExtraFiles = handles
	process.Stdout, process.Stderr = stdout, stderr
	started := time.Now()
	if err := process.Start(); err != nil {
		return operation.Result{}, fmt.Errorf("start direct-coding verification sandbox: %w", err)
	}
	infoWriter := handles[len(handles)-1]
	if err := infoWriter.Close(); err != nil {
		_ = process.Process.Kill()
		_ = process.Wait()
		return operation.Result{}, fmt.Errorf("close verification sandbox status writer: %w", err)
	}
	infoChannel := make(chan []byte, 1)
	go func() {
		content, _ := io.ReadAll(io.LimitReader(infoReader, 16*1024))
		infoChannel <- content
	}()
	runErr := process.Wait()
	duration := time.Since(started)
	info := <-infoChannel
	if runCtx.Err() != nil {
		return operation.Result{}, fmt.Errorf("direct-coding verification command ended: %w", runCtx.Err())
	}
	if len(bytes.TrimSpace(info)) == 0 || !json.Valid(bytes.TrimSpace(info)) {
		stderrText, _, _ := stderr.Result()
		return operation.Result{}, fmt.Errorf(
			"direct-coding verification sandbox failed before command start: %s",
			trimForBudget(stderrText, 1200),
		)
	}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return operation.Result{}, fmt.Errorf("wait for direct-coding verification sandbox: %w", runErr)
	}
	result := directCodingSandboxResult(command, stdout, stderr, duration, runErr)
	if err := sandbox.VerifyAuthority(ctx); err != nil {
		return operation.Result{}, fmt.Errorf("verify direct-coding command isolation: %w", err)
	}
	return result, nil
}

func directCodingSandboxResult(
	command testCommand,
	stdout, stderr *boundedCommandOutput,
	duration time.Duration,
	runErr error,
) operation.Result {
	succeeded := runErr == nil
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	label := directCodingCommandLabel(command)
	warnings := []string(nil)
	if runErr != nil {
		warnings = []string{"command failed: " + runErr.Error()}
	}
	stdoutText, stdoutBytes, stdoutTruncated := stdout.Result()
	stderrText, stderrBytes, stderrTruncated := stderr.Result()
	summary := fmt.Sprintf("command %s exit_code=%d duration_ms=%d", command.Name, exitCode, duration.Milliseconds())
	return operation.Result{
		Summary: summary, Warnings: warnings,
		Output: map[string]any{
			"succeeded": succeeded, "exit_code": exitCode,
			"stdout": stdoutText, "stderr": stderrText,
			"stdout_observed_bytes": stdoutBytes, "stderr_observed_bytes": stderrBytes,
			"stdout_truncated": stdoutTruncated, "stderr_truncated": stderrTruncated,
			"duration_ms": duration.Milliseconds(),
		},
		Evidence: []evidence.Record{{
			Kind: evidence.KindCommandOutput, ToolName: "command.run", Command: label,
			Excerpt: trimForBudget("stdout:\n"+stdoutText+"\nstderr:\n"+stderrText, maxV3CommandOutput),
			Summary: summary, Warnings: warnings, Confidence: 1,
			Metadata: map[string]any{
				"execution": true, "side_effect_possible": true, "succeeded": succeeded,
				"exit_code": exitCode, "duration_ms": duration.Milliseconds(),
				"stdout_observed_bytes": stdoutBytes, "stderr_observed_bytes": stderrBytes,
				"stdout_truncated": stdoutTruncated, "stderr_truncated": stderrTruncated,
			},
		}},
	}
}

func (sandbox *directCodingWorkspaceSandbox) Cleanup() error {
	if sandbox == nil || sandbox.root == "" {
		return nil
	}
	err := os.RemoveAll(sandbox.root)
	if err != nil {
		return fmt.Errorf("clean direct-coding verification sandbox: %w", err)
	}
	sandbox.root = ""
	return nil
}
