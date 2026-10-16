package codex

import "github.com/gryph/omnidex/internal/codingagent"

// Planning can inspect the workspace but cannot execute proposed changes.
// Task execution and independent verification can write their workspace;
// network and host changes go through the existing owner-input protocol.
func agentPermissions(call codingagent.Assignment) (sandbox, approval string, policy map[string]any) {
	if call.Role == codingagent.Execute || call.Role == codingagent.Review {
		return "workspace-write", "on-request", map[string]any{
			"type": "workspaceWrite", "writableRoots": []string{call.Root}, "networkAccess": false,
		}
	}
	return "read-only", "never", map[string]any{"type": "readOnly", "networkAccess": false}
}
