package codex

import (
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Recover an already-dispatched native turn, never dispatch its prompt again.
// Interrupted or indeterminate external effects require an owner decision, not
// an inference retry that could repeat a Docker/SSH operation.
func (connection *connection) recoverExecutionResult(call codingagent.Assignment, result codingagent.Result, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	var response struct {
		Data []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Items  []struct {
				Type     string `json:"type"`
				Phase    string `json:"phase"`
				Text     string `json:"text"`
				Delivery string `json:"delivery"`
			} `json:"items"`
		} `json:"data"`
	}
	raw, err := connection.call("thread/turns/list", map[string]any{"threadId": result.ThreadID,
		"limit": 1, "sortDirection": "desc", "itemsView": "full"}, &response)
	if err != nil {
		return result, fmt.Errorf("inspect original native execution thread %s without replay: %w", result.ThreadID, err)
	}
	if err := recordAgentFrame(&result, raw, event); err != nil {
		return result, err
	}
	if len(response.Data) != 1 {
		return result, &codingagent.IncompleteTaskError{Reason: "Execution dispatch is indeterminate; inspect partial work and plan its continuation without replaying commands.", ThreadID: result.ThreadID}
	}
	turn := response.Data[0]
	result.TurnID = turn.ID
	if turn.ID != "" && turn.ID != call.Session.PreviousTurnID && (call.Session.TurnID == "" || call.Session.TurnID == turn.ID) && (turn.Status == "interrupted" || turn.Status == "failed") {
		// A terminal interrupted turn cannot be replayed. Retain its exact thread
		// and partial effects; only an explicit owner resume may start continuation.
		if err := event(codingagent.NativeEvent{Session: &codingagent.ExecutionSession{ThreadID: result.ThreadID, TurnID: turn.ID, State: "idle"}}); err != nil {
			return result, err
		}
	}
	if turn.ID == "" || turn.ID == call.Session.PreviousTurnID || call.Session.TurnID != "" && call.Session.TurnID != turn.ID || turn.Status != "completed" {
		return result, &codingagent.IncompleteTaskError{Reason: fmt.Sprintf("Original execution is %q, not complete; inspect partial work before planning continuation, without replaying commands.", turn.Status), ThreadID: result.ThreadID, TurnID: turn.ID}
	}
	result.TurnID = turn.ID
	for _, item := range turn.Items {
		if err := collectAgentResultItem(&result, call.Role, item.Type, item.Phase, item.Delivery, item.Text); err != nil {
			return result, err
		}
	}
	if len(result.Content) == 0 {
		return result, fmt.Errorf("original completed turn lacks its final response; no action was replayed")
	}
	result.Completed = true
	return result, nil
}
