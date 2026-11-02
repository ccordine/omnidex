package codex

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/codingagent"
)

type pendingAgentFrame struct {
	frame rpcFrame
	raw   json.RawMessage
}

// Review uses Codex's native /review operation on its independent thread.
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
	method := "turn/start"
	if call.Role == codingagent.Review {
		method = "review/start"
		parameters = map[string]any{
			"threadId": thread.Thread.ID,
			"delivery": "inline",
			"target":   map[string]string{"type": "custom", "instructions": prompt},
		}
	}
	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	pending := []pendingAgentFrame{}
	raw, err := connection.callObserved(method, parameters, &response, nil, func(frame rpcFrame) error {
		pending = append(pending, pendingAgentFrame{frame: frame, raw: append(json.RawMessage(nil), connection.lastFrame...)})
		return nil
	})
	if err != nil {
		return "", nil, nil, err
	}
	return response.Turn.ID, raw, pending, nil
}
