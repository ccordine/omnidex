package codex

import "github.com/gryph/omnidex/internal/codingagent"

// Code dispatches execution only after ledger freeze. That task already has
// the owner's execution grant; ordinary host operations need no reapproval.
func agentPermissions(call codingagent.Assignment) (sandbox, approval string, policy map[string]any) {
	if call.Role == codingagent.Execute {
		return "danger-full-access", "never", map[string]any{"type": "dangerFullAccess"}
	}
	if call.Role == codingagent.Review {
		return "workspace-write", "on-request", map[string]any{
			"type": "workspaceWrite", "writableRoots": []string{call.Root}, "networkAccess": false,
		}
	}
	return "read-only", "never", map[string]any{"type": "readOnly", "networkAccess": false}
}
