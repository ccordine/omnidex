package codex

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
)

func validateAgentItems(frame rpcFrame, role codingagent.Role, open map[string]string) error {
	if frame.Method != "item/started" && frame.Method != "item/completed" {
		return nil
	}
	var params struct {
		Item struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"item"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return err
	}
	item := params.Item
	if item.ID == "" {
		return fmt.Errorf("native agent item lacks identity")
	}
	if (item.Type == "fileChange" || item.Type == "dynamicToolCall") && role != codingagent.Execute {
		return fmt.Errorf("read-only agent attempted a source edit")
	}
	if frame.Method == "item/started" {
		if _, exists := open[item.ID]; exists {
			return fmt.Errorf("native item identity reused")
		}
		open[item.ID] = item.Type
		return nil
	}
	if open[item.ID] != item.Type {
		return fmt.Errorf("native agent completed an unbound item")
	}
	if item.Status == "inProgress" || item.Status == "in_progress" {
		return fmt.Errorf("native agent item is unfinished")
	}
	delete(open, item.ID)
	return nil
}

func recordAgentFrame(result *codingagent.Result, raw json.RawMessage, event func(json.RawMessage) error) error {
	count := len(raw)
	for _, frame := range result.Evidence {
		count += len(frame)
	}
	if count > llm.MaxExactPreparedProviderResponseBytes {
		return fmt.Errorf("native agent evidence exceeds its transport boundary")
	}
	owned := append(json.RawMessage(nil), raw...)
	if event != nil {
		if err := event(owned); err != nil {
			return err
		}
	}
	result.Evidence = append(result.Evidence, owned)
	return nil
}

func collectAgentFrame(frame rpcFrame, raw json.RawMessage, result *codingagent.Result) (bool, error) {
	var params struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		Item     struct {
			Type  string `json:"type"`
			Phase string `json:"phase"`
			Text  string `json:"text"`
		} `json:"item"`
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return false, err
	}
	if params.ThreadID != "" && params.ThreadID != result.ThreadID || params.TurnID != "" && params.TurnID != result.TurnID {
		return false, fmt.Errorf("agent event belongs to another assignment")
	}
	switch frame.Method {
	case "item/started", "item/completed":
		switch params.Item.Type {
		case "userMessage", "reasoning", "agentMessage", "commandExecution", "fileChange", "dynamicToolCall":
		default:
			return false, fmt.Errorf("agent attempted an ungranted capability %s", params.Item.Type)
		}
		if frame.Method == "item/completed" && params.Item.Type == "agentMessage" && params.Item.Phase == "final_answer" {
			if len(result.Content) != 0 || !json.Valid([]byte(params.Item.Text)) {
				return false, fmt.Errorf("agent returned missing, duplicate or invalid structured result")
			}
			result.Content = json.RawMessage(params.Item.Text)
		}
	case "turn/completed":
		if params.Turn.ID != result.TurnID || params.Turn.Status != "completed" || len(result.Content) == 0 {
			return false, fmt.Errorf("agent did not complete the exact assignment")
		}
		result.Completed = true
		return true, nil
	case "error":
		return false, fmt.Errorf("native agent reported an inference failure")
	}
	return false, nil
}
