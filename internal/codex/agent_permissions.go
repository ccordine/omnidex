package codex

import "github.com/gryph/omnidex/internal/codingagent"

// Code dispatches execution and independent review only after ledger freeze.
// Both inherit the task execution grant; ordinary verification needs no reapproval.
func agentPermissions(call codingagent.Assignment) (sandbox, approval string, policy map[string]any) {
	if call.Role == codingagent.Execute || call.Role == codingagent.Review {
		return "danger-full-access", "never", map[string]any{"type": "dangerFullAccess"}
	}
	return "read-only", "never", map[string]any{"type": "readOnly", "networkAccess": false}
}
