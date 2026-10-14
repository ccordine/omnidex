package codex

import (
	"context"
	"fmt"

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
		result, err := client.executeAgentCheck(connection, scope, check, scratch, activity)
		if err != nil {
			return nil, fmt.Errorf("verify check %s: %w", check.ID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func readOnlyCheckScope(scope codingagent.Assignment) (codingagent.Assignment, error) {
	switch scope.Role {
	case codingagent.Execute:
		scope.Role = codingagent.Review
	case codingagent.Review, codingagent.OverallReview:
	default:
		return scope, fmt.Errorf("deterministic checks require a task or overall review scope")
	}
	scope.WriteFiles = nil
	scope.DeleteFiles = nil
	return scope, validateAgentAssignment(scope)
}
