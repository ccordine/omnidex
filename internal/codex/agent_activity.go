package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

type agentActivityItem struct {
	kind         string
	phase        string
	provisional  bool
	streamed     bool
	sent         string
	summaryIndex int
}

type agentActivityStream struct {
	call   codingagent.Assignment
	result *codingagent.Result
	sink   func(codingagent.NativeEvent) error
	items  map[string]*agentActivityItem
}

type agentActivityParams struct {
	ItemID       string `json:"itemId"`
	Delta        string `json:"delta"`
	SummaryIndex int    `json:"summaryIndex"`
	Item         struct {
		ID       string   `json:"id"`
		Type     string   `json:"type"`
		Phase    string   `json:"phase"`
		Text     string   `json:"text"`
		Summary  []string `json:"summary"`
		Command  string   `json:"command"`
		CWD      string   `json:"cwd"`
		Status   string   `json:"status"`
		ExitCode *int     `json:"exitCode"`
		Changes  []struct {
			Path string `json:"path"`
		} `json:"changes"`
	} `json:"item"`
}

func newAgentActivityStream(call codingagent.Assignment, result *codingagent.Result, sink func(codingagent.NativeEvent) error) *agentActivityStream {
	return &agentActivityStream{call: call, result: result, sink: sink, items: map[string]*agentActivityItem{}}
}

func agentActivityBase(call codingagent.Assignment, result *codingagent.Result, itemID string, kind model.AgentActivityKind, status model.AgentActivityStatus) model.AgentActivity {
	activity := model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: kind, Status: status, ThreadID: result.ThreadID, TurnID: result.TurnID, ItemID: itemID, Role: string(call.Role)}
	if call.Task != nil {
		activity.TaskID = call.Task.ID
	}
	return activity
}

func (stream *agentActivityStream) publish(activity model.AgentActivity) error {
	if err := activity.Validate(); err != nil {
		return err
	}
	if stream.sink != nil {
		return stream.sink(codingagent.NativeEvent{Activity: &activity})
	}
	return nil
}

func (stream *agentActivityStream) observe(event codingagent.NativeEvent) error {
	if event.Activity != nil {
		return stream.publish(*event.Activity)
	}
	if stream.sink != nil {
		if err := stream.sink(event); err != nil {
			return err
		}
	}
	if len(event.Frame) == 0 {
		return nil
	}
	var frame rpcFrame
	if err := json.Unmarshal(event.Frame, &frame); err != nil {
		return err
	}
	if frame.Method != "item/started" && frame.Method != "item/completed" && agentDeltaItemKind(frame.Method) == "" {
		return nil
	}
	var params agentActivityParams
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return err
	}
	if frame.Method == "item/started" {
		if existing := stream.items[params.Item.ID]; existing != nil && existing.provisional {
			existing.kind, existing.phase, existing.provisional = params.Item.Type, params.Item.Phase, false
		} else {
			stream.items[params.Item.ID] = &agentActivityItem{kind: params.Item.Type, phase: params.Item.Phase}
		}
		if params.Item.Type == "commandExecution" {
			return stream.command(params, model.AgentActivityStarted)
		}
		return nil
	}
	if frame.Method == "item/completed" {
		item := stream.items[params.Item.ID]
		if item == nil {
			return fmt.Errorf("native activity completed an unbound item")
		}
		if err := stream.completed(params, item); err != nil {
			return err
		}
		delete(stream.items, params.Item.ID)
		return nil
	}
	item := stream.items[params.ItemID]
	if item == nil {
		item = &agentActivityItem{kind: agentDeltaItemKind(frame.Method), provisional: true}
		stream.items[params.ItemID] = item
	}
	switch frame.Method {
	case "item/commandExecution/outputDelta":
		return stream.text(params.ItemID, model.AgentActivityCommandOutput, model.AgentActivityDelta, params.Delta)
	case "item/agentMessage/delta":
		if item.provisional {
			item.sent += params.Delta
			return nil
		}
		if item.phase != "commentary" {
			return nil
		}
		item.streamed = true
		item.sent += params.Delta
		return stream.text(params.ItemID, model.AgentActivityMessage, model.AgentActivityDelta, params.Delta)
	case "item/plan/delta":
		item.streamed = true
		item.sent += params.Delta
		return stream.text(params.ItemID, model.AgentActivityMessage, model.AgentActivityDelta, params.Delta)
	case "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded":
		if params.SummaryIndex < item.summaryIndex || params.SummaryIndex > item.summaryIndex+1 {
			return fmt.Errorf("native public summary has invalid section identity")
		}
		text := params.Delta
		if frame.Method == "item/reasoning/summaryPartAdded" {
			text = ""
		}
		if item.summaryIndex != params.SummaryIndex {
			text = "\n" + text
		}
		item.streamed, item.summaryIndex = true, params.SummaryIndex
		item.sent += text
		return stream.text(params.ItemID, model.AgentActivityReasoningSummary, model.AgentActivityDelta, text)
	}
	return nil
}

func (stream *agentActivityStream) completed(params agentActivityParams, item *agentActivityItem) error {
	switch params.Item.Type {
	case "commandExecution":
		status := model.AgentActivityCompleted
		if params.Item.Status != "completed" {
			status = model.AgentActivityFailed
		}
		return stream.command(params, status)
	case "fileChange":
		if params.Item.Status != "completed" {
			return nil
		}
		for _, change := range params.Item.Changes {
			activity := agentActivityBase(stream.call, stream.result, params.Item.ID, model.AgentActivityFileEdit, model.AgentActivityCompleted)
			activity.Paths = []string{change.Path}
			if err := stream.publish(activity); err != nil {
				return err
			}
		}
		return nil
	case "agentMessage":
		if params.Item.Phase != "commentary" {
			return nil
		}
		return stream.completeText(params.Item.ID, model.AgentActivityMessage, model.AgentActivityCompleted, item.sent, params.Item.Text)
	case "plan":
		return stream.completeText(params.Item.ID, model.AgentActivityMessage, model.AgentActivityCompleted, item.sent, params.Item.Text)
	case "reasoning":
		return stream.completeText(params.Item.ID, model.AgentActivityReasoningSummary, model.AgentActivityCompleted, item.sent, strings.Join(params.Item.Summary, "\n"))
	}
	return nil
}

func (stream *agentActivityStream) command(params agentActivityParams, status model.AgentActivityStatus) error {
	activity := agentActivityBase(stream.call, stream.result, params.Item.ID, model.AgentActivityCommand, status)
	activity.Command, activity.CWD, activity.ExitCode = params.Item.Command, params.Item.CWD, params.Item.ExitCode
	return stream.publish(model.BoundAgentActivityCommand(activity))
}

func (stream *agentActivityStream) completeText(itemID string, kind model.AgentActivityKind, status model.AgentActivityStatus, sent, final string) error {
	if !strings.HasPrefix(final, sent) {
		return fmt.Errorf("native completed public text contradicts its streamed prefix: kind=%s item=%.64q sentBytes=%d finalBytes=%d", kind, itemID, len(sent), len(final))
	}
	return stream.text(itemID, kind, status, final[len(sent):])
}

func (stream *agentActivityStream) text(itemID string, kind model.AgentActivityKind, status model.AgentActivityStatus, text string) error {
	activity := agentActivityBase(stream.call, stream.result, itemID, kind, status)
	activity.Text = text
	return publishAgentActivityText(activity, stream.publish)
}
