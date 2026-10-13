package codex

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
)

type pendingAgentFrame struct {
	frame rpcFrame
	raw   json.RawMessage
}

// startAgentTurn maps Omnidex roles onto Codex's native collaboration modes.
// Execution remains an ordinary tool-capable turn, while planning and review
// use the same native protocol surfaces as Codex's Plan and Review modes.
func (connection *connection) startAgentTurn(call codingagent.Assignment, thread threadResult, prompt string) (string, json.RawMessage, []pendingAgentFrame, error) {
	method := "turn/start"
	parameters := map[string]any{
		"threadId":       thread.Thread.ID,
		"input":          []any{map[string]string{"type": "text", "text": prompt}},
		"model":          call.Route.Model,
		"permissions":    "omnidex-task",
		"approvalPolicy": "never",
		"summary":        "auto",
		"outputSchema":   agentOutputSchema(call),
	}
	applyAgentInferenceOptions(parameters, call.Route)
	if call.Role == codingagent.Plan {
		parameters["collaborationMode"] = map[string]any{
			"mode": "plan",
			"settings": map[string]any{
				"model":                  thread.Model,
				"reasoning_effort":       thread.ReasoningEffort,
				"developer_instructions": nil,
			},
		}
	}
	if call.Role == codingagent.Review || call.Role == codingagent.OverallReview {
		method = "review/start"
		parameters = map[string]any{
			"threadId": thread.Thread.ID,
			"delivery": "inline",
			"target": map[string]any{
				"type":         "custom",
				"instructions": prompt,
			},
		}
	}
	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
		ReviewThreadID string `json:"reviewThreadId"`
	}
	pending := []pendingAgentFrame{}
	raw, err := connection.callObserved(method, parameters, &response, nil, func(frame rpcFrame) error {
		pending = append(pending, pendingAgentFrame{frame: frame, raw: append(json.RawMessage(nil), connection.lastFrame...)})
		return nil
	})
	if err != nil {
		return "", nil, nil, err
	}
	if method == "review/start" && response.ReviewThreadID != thread.Thread.ID {
		return "", nil, nil, fmt.Errorf("native inline review returned a different thread")
	}
	return response.Turn.ID, raw, pending, nil
}
