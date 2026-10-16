package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

// ExecuteChecks invokes the native sandbox directly. No model is asked to run
// or interpret a command whose arguments are already frozen by Omnidex.
func (client *Client) ExecuteChecks(parent context.Context, scope codingagent.Assignment, checks []codingagent.Check, activity func(model.AgentActivity) error) ([]codingagent.CheckResult, error) {
	scope, err := readOnlyCheckScope(scope)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	scratch, cleanup, err := client.agentScratch()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	checkRoot, err := stageAgentCheckWorkspace(scope, scratch)
	if err != nil {
		return nil, err
	}
	// Native execution uses the sealed physical projection. Public activity
	// retains scope.Root as the task's logical workspace authority; the private
	// scratch path is neither a new capability nor a durable project location.
	connection, err := startConfiguredConnection(ctx, client.command, checkRoot, true, agentPermissionFlags())
	if err != nil {
		return nil, err
	}
	defer connection.close()
	if _, err := connection.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "omnidex", "version": "0.5.0"}, "capabilities": map[string]bool{"experimentalApi": true}}, nil); err != nil {
		return nil, err
	}
	if err := connection.send("initialized", map[string]any{}, nil); err != nil {
		return nil, err
	}
	results := make([]codingagent.CheckResult, 0, len(checks))
	for _, check := range checks {
		result, err := client.executeAgentCheck(connection, scope, checkRoot, check, scratch, activity)
		if err != nil {
			return nil, fmt.Errorf("verify check %s: %w", check.ID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func stageAgentCheckWorkspace(scope codingagent.Assignment, scratch string) (string, error) {
	root := filepath.Join(scratch, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		return "", fmt.Errorf("create isolated verification workspace: %w", err)
	}
	for _, name := range scope.ReadFiles {
		sourcePath := filepath.Join(scope.Root, filepath.FromSlash(name))
		destinationPath := filepath.Join(root, filepath.FromSlash(name))
		info, err := os.Stat(sourcePath)
		if err != nil {
			return "", fmt.Errorf("inspect verification file %s: %w", name, err)
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
			return "", fmt.Errorf("create verification directory for %s: %w", name, err)
		}
		source, err := os.Open(sourcePath)
		if err != nil {
			return "", fmt.Errorf("open verification file %s: %w", name, err)
		}
		mode := os.FileMode(0o400)
		if info.Mode().Perm()&0o111 != 0 {
			mode |= 0o100
		}
		destination, createErr := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if createErr != nil {
			_ = source.Close()
			return "", fmt.Errorf("stage verification file %s: %w", name, createErr)
		}
		_, copyErr := io.Copy(destination, source)
		closeErr := destination.Close()
		sourceCloseErr := source.Close()
		if copyErr != nil || closeErr != nil || sourceCloseErr != nil {
			return "", fmt.Errorf("copy verification file %s: %w", name, errors.Join(copyErr, closeErr, sourceCloseErr))
		}
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o500)
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("seal isolated verification workspace: %w", err)
	}
	return root, nil
}

func readOnlyCheckScope(scope codingagent.Assignment) (codingagent.Assignment, error) {
	switch scope.Role {
	case codingagent.Execute:
		scope.Role = codingagent.Review
	case codingagent.Review:
	default:
		return scope, fmt.Errorf("deterministic checks require one task scope")
	}
	scope.WriteFiles = nil
	scope.DeleteFiles = nil
	return scope, validateAgentAssignment(scope)
}
