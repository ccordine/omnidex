package codex

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

// ExecuteChecks invokes the native sandbox directly. No model is asked to run
// or interpret a command whose arguments are already frozen by Omnidex.
func (client *Client) ExecuteChecks(parent context.Context, scope codingagent.Assignment, checks []codingagent.Check) ([]codingagent.CheckResult, error) {
	scope.WriteFiles = nil
	scope.Role = codingagent.Review
	if err := validateAgentAssignment(scope); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	scratch, cleanup, err := client.agentScratch()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	profile, err := client.executionPermissionProfile(scope, scratch)
	if err != nil {
		return nil, err
	}
	connection, err := startConfiguredConnection(ctx, client.command, scope.Root, true, agentProfileFlags(profile))
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
		var response struct {
			ExitCode *int   `json:"exitCode"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		}
		_, err := connection.call("command/exec", map[string]any{"command": check.Argv, "cwd": scope.Root, "permissionProfile": "omnidex-task",
			"timeoutMs": int64(client.timeout / time.Millisecond), "outputBytesCap": 1 << 20,
			"env": agentToolEnvironment(scratch)}, &response)
		if err != nil {
			return nil, fmt.Errorf("verify check %s: %w", check.ID, err)
		}
		if response.ExitCode == nil {
			return nil, fmt.Errorf("verification check %s lacks its actual exit status", check.ID)
		}
		results = append(results, codingagent.CheckResult{ID: check.ID, ExitCode: *response.ExitCode, Output: response.Stdout + response.Stderr})
	}
	return results, nil
}
