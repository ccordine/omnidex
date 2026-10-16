package codex

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/codingagent"
)

type pendingAgentFrame struct {
	frame rpcFrame
	raw   json.RawMessage
}

// startAgentTurn opens one ordinary isolated turn for every assignment. Review
// deliberately does not use Codex's repository-review mode: that mode can add
// context outside the frozen task packet and owns a separate output contract.
func (connection *connection) startAgentTurn(call codingagent.Assignment, thread threadResult, prompt string) (string, json.RawMessage, []pendingAgentFrame, error) {
	_, approval, policy := agentPermissions(call)
	parameters := map[string]any{
		"threadId":       thread.Thread.ID,
		"input":          []any{map[string]string{"type": "text", "text": prompt}},
		"model":          call.Route.Model,
		"approvalPolicy": approval,
		"sandboxPolicy":  policy,
		"summary":        "auto",
	}
	if !call.Role.IsNative() && call.Role != codingagent.PlanningMutation {
		parameters["outputSchema"] = agentOutputSchema(call)
	}
	applyAgentInferenceOptions(parameters, call.Route)
	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	pending := []pendingAgentFrame{}
	raw, err := connection.callObserved("turn/start", parameters, &response, nil, func(frame rpcFrame) error {
		pending = append(pending, pendingAgentFrame{frame: frame, raw: append(json.RawMessage(nil), connection.lastFrame...)})
		return nil
	})
	if err != nil {
		return "", nil, nil, err
	}
	return response.Turn.ID, raw, pending, nil
}
