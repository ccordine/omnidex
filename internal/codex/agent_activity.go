package codex

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

type agentActivityItem struct {
	phase        string
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
		ID               string   `json:"id"`
		Type             string   `json:"type"`
		Phase            string   `json:"phase"`
		Text             string   `json:"text"`
		Summary          []string `json:"summary"`
		Command          string   `json:"command"`
		CWD              string   `json:"cwd"`
		Status           string   `json:"status"`
		AggregatedOutput *string  `json:"aggregatedOutput"`
		ExitCode         *int     `json:"exitCode"`
		CommandActions   []struct {
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"commandActions"`
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
		stream.items[params.Item.ID] = &agentActivityItem{phase: params.Item.Phase}
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
		defer delete(stream.items, params.Item.ID)
		return stream.completed(params, item)
	}
	item := stream.items[params.ItemID]
	if item == nil {
		return fmt.Errorf("native activity delta belongs to an unbound item")
	}
	switch frame.Method {
	case "item/agentMessage/delta":
		if item.phase != "commentary" {
			return nil
		}
		item.streamed = true
		item.sent += params.Delta
		return stream.text(params.ItemID, model.AgentActivityMessage, model.AgentActivityDelta, params.Delta)
	case "item/reasoning/summaryTextDelta":
		if params.SummaryIndex < 0 {
			return fmt.Errorf("native public summary has invalid section identity")
		}
		text := params.Delta
		if item.streamed && item.summaryIndex != params.SummaryIndex {
			text = "\n" + text
		}
		item.streamed, item.summaryIndex = true, params.SummaryIndex
		item.sent += text
		return stream.text(params.ItemID, model.AgentActivityReasoningSummary, model.AgentActivityDelta, text)
	case "item/commandExecution/outputDelta":
		item.streamed = true
		item.sent += params.Delta
		return stream.text(params.ItemID, model.AgentActivityCommandOutput, model.AgentActivityDelta, params.Delta)
	}
	return nil
}

func (stream *agentActivityStream) completed(params agentActivityParams, item *agentActivityItem) error {
	switch params.Item.Type {
	case "agentMessage":
		if params.Item.Phase != "commentary" {
			return nil
		}
		return stream.completeText(params.Item.ID, model.AgentActivityMessage, model.AgentActivityCompleted, item.sent, params.Item.Text)
	case "reasoning":
		return stream.completeText(params.Item.ID, model.AgentActivityReasoningSummary, model.AgentActivityCompleted, item.sent, strings.Join(params.Item.Summary, "\n"))
	case "commandExecution":
		if params.Item.AggregatedOutput != nil {
			if err := stream.completeText(params.Item.ID, model.AgentActivityCommandOutput, model.AgentActivityDelta, item.sent, *params.Item.AggregatedOutput); err != nil {
				return err
			}
		}
		status := model.AgentActivityStatus(params.Item.Status)
		if status != model.AgentActivityCompleted && status != model.AgentActivityFailed && status != model.AgentActivityDeclined {
			return fmt.Errorf("native command has invalid terminal status")
		}
		return stream.command(params, status)
	}
	return nil
}

func (stream *agentActivityStream) completeText(itemID string, kind model.AgentActivityKind, status model.AgentActivityStatus, sent, final string) error {
	if !strings.HasPrefix(final, sent) {
		return fmt.Errorf("native completed public text contradicts its streamed prefix")
	}
	return stream.text(itemID, kind, status, final[len(sent):])
}

func (stream *agentActivityStream) command(params agentActivityParams, status model.AgentActivityStatus) error {
	activity := agentActivityBase(stream.call, stream.result, params.Item.ID, model.AgentActivityCommand, status)
	activity.Command, activity.CWD, activity.ExitCode = params.Item.Command, params.Item.CWD, params.Item.ExitCode
	activity = model.BoundAgentActivityCommand(activity)
	if err := stream.publish(activity); err != nil {
		return err
	}
	if status != model.AgentActivityCompleted || activity.ExitCode == nil || *activity.ExitCode != 0 {
		return nil
	}
	for _, action := range params.Item.CommandActions {
		if action.Type != "read" || action.Path == "" {
			continue
		}
		file := action.Path
		if !filepath.IsAbs(file) {
			file = filepath.Join(params.Item.CWD, file)
		}
		relative, err := filepath.Rel(stream.call.Root, file)
		if err != nil {
			return err
		}
		file = filepath.ToSlash(relative)
		if !agentActivityReadablePath(stream.call, file) {
			continue
		}
		read := agentActivityBase(stream.call, stream.result, params.Item.ID, model.AgentActivityFileRead, model.AgentActivityCompleted)
		read.Paths = []string{file}
		if err := stream.publish(read); err != nil {
			return err
		}
	}
	return nil
}

func agentActivityReadablePath(call codingagent.Assignment, file string) bool {
	for _, granted := range call.ReadFiles {
		if file == granted {
			return true
		}
	}
	return false
}

func (stream *agentActivityStream) text(itemID string, kind model.AgentActivityKind, status model.AgentActivityStatus, text string) error {
	activity := agentActivityBase(stream.call, stream.result, itemID, kind, status)
	activity.Text = text
	return publishAgentActivityText(activity, stream.publish)
}
