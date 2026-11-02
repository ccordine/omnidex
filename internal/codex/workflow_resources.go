package codex

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/workflow"
)

func (client *Client) workflowScope(root string, job int64, task string) (workflow.Scope, error) {
	scope, err := workflow.Open(client.directory, root)
	scope.Job, scope.Task = job, task
	return scope, err
}

func (client *Client) workflowEnvironment(call codingagent.Assignment) ([]string, error) {
	if call.Role != codingagent.Execute && call.Role != codingagent.Review {
		return os.Environ(), nil
	}
	task := "goal-review"
	if call.Task != nil {
		task = call.Task.ID
	}
	scope, err := client.workflowScope(call.Root, call.JobID, task)
	if err != nil {
		return nil, err
	}
	helper, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// Native qualification tests use the same built command driver as the server.
	if built := filepath.Join(client.directory, "bin", "omnidex"); built != helper {
		if info, err := os.Stat(built); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			helper = built
		}
	}
	return scope.Environment(os.Environ(), helper)
}

// CleanupWorkflow is called by the existing database-owned task loop only after
// acceptance, and on recovery with the retained accepted task IDs.
func (client *Client) CleanupWorkflow(ctx context.Context, root string, job int64, accepted []string) error {
	scope, err := client.workflowScope(root, job, "")
	if err != nil {
		return err
	}
	if err = scope.Complete(accepted); err != nil {
		return err
	}
	for _, task := range accepted {
		if err = scope.CleanTemporary(task); err != nil {
			return err
		}
	}
	return scope.Cleanup(ctx)
}
