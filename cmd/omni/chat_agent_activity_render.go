package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

const (
	maxAgentRenderItems = 16
	maxAgentRenderBytes = 2048
)

type agentRenderKey struct {
	source, task, check string
	thread, turn, item  string
	kind                model.AgentActivityKind
}

type agentRenderItem struct {
	pending      string
	order        uint64
	headingShown bool
	truncated    bool
}

// Only bounded, unfinished public display fragments live here. The runtime
// owns native identities, execution, complete output and every task decision.
type chatAgentStream struct {
	items map[agentRenderKey]*agentRenderItem
	next  uint64
	files []agentObservedFile
}

type agentObservedFile struct {
	job, step int64
	path      string
}

func (renderer *chatRenderer) nativeActivity(event client.RealtimeEvent) error {
	activity := *event.AgentActivity
	if renderer.agentStream == nil {
		renderer.agentStream = &chatAgentStream{items: make(map[agentRenderKey]*agentRenderItem)}
	}
	if renderer.verbosity == chatQuiet {
		clear(renderer.agentStream.items)
		if activity.Kind != model.AgentActivityCommand || activity.Status != model.AgentActivityFailed && activity.Status != model.AgentActivityDeclined {
			return nil
		}
	}
	if err := renderer.retireAgentTurn(activity); err != nil {
		return err
	}
	switch activity.Kind {
	case model.AgentActivityMessage, model.AgentActivityReasoningSummary:
		return renderer.agentText(event, activity)
	case model.AgentActivityCommandOutput:
		if renderer.verbosity == chatVerbose || renderer.verbosity == chatDebug {
			return renderer.agentText(event, activity)
		}
	case model.AgentActivityCommand:
		command := activity.Command
		if activity.Source == model.AgentActivityVerification {
			arguments := make([]string, len(activity.Argv))
			for index, argument := range activity.Argv {
				arguments[index] = strconv.Quote(argument)
			}
			command = strings.Join(arguments, " ")
		}
		if err := renderer.finishCommandOutput(activity); err != nil {
			return err
		}
		var rendered strings.Builder
		if activity.Status == model.AgentActivityStarted {
			renderBlock(&rendered, "command>", command)
			if renderer.verbosity == chatVerbose || renderer.verbosity == chatDebug {
				fmt.Fprintf(&rendered, "      in %s\n", activity.CWD)
			}
		} else {
			fmt.Fprintf(&rendered, "[omni] command %s", activity.Status)
			if activity.ExitCode != nil {
				fmt.Fprintf(&rendered, " · exit %d", *activity.ExitCode)
			}
			fmt.Fprintf(&rendered, " · %s\n", command)
		}
		if activity.Truncated {
			fmt.Fprintln(&rendered, "[omni] Command preview truncated.")
		}
		if err := renderer.console.WriteOutput(rendered.String()); err != nil {
			return err
		}
		if activity.Status != model.AgentActivityStarted {
			return renderer.console.SetActivityDetail(event.JobID, event.StepID, agentActivityRoleLabel(activity))
		}
		return renderer.console.SetActivityDetail(event.JobID, event.StepID, "Running: "+shortAgentExcerpt(command))
	case model.AgentActivityFileRead, model.AgentActivityFileEdit:
		operation := "read"
		paths := activity.Paths
		if activity.Kind == model.AgentActivityFileEdit {
			operation = "edit"
			paths = nil
			for _, path := range activity.Paths {
				if !renderer.observedWorkspaceFile(event.JobID, event.StepID, path) {
					paths = append(paths, path)
				}
			}
		}
		if len(paths) > 0 {
			quoted := make([]string, len(paths))
			for index, path := range paths {
				quoted[index] = strconv.Quote(path)
			}
			return renderer.system("file %s · %s", operation, strings.Join(quoted, ", "))
		}
	}
	return nil
}

func (renderer *chatRenderer) observeWorkspaceFile(event client.RealtimeEvent) {
	if event.RuntimeEvent != "workspace_file_changed" || event.FilePath == "" || renderer.verbosity == chatQuiet {
		return
	}
	if renderer.agentStream == nil {
		renderer.agentStream = &chatAgentStream{items: make(map[agentRenderKey]*agentRenderItem)}
	}
	if len(renderer.agentStream.files) == maxAgentRenderItems {
		renderer.agentStream.files = renderer.agentStream.files[1:]
	}
	renderer.agentStream.files = append(renderer.agentStream.files, agentObservedFile{event.JobID, event.StepID, event.FilePath})
}

func (renderer *chatRenderer) observedWorkspaceFile(job, step int64, path string) bool {
	for _, file := range renderer.agentStream.files {
		if file.job == job && file.step == step && file.path == path {
			return true
		}
	}
	return false
}

func (renderer *chatRenderer) retireAgentTurn(activity model.AgentActivity) error {
	for key, item := range renderer.agentStream.items {
		if activity.Source == model.AgentActivityNative && key.source == string(model.AgentActivityNative) && key.thread == activity.ThreadID && key.turn != activity.TurnID {
			if err := renderer.flushAgentItem(key, item); err != nil {
				return err
			}
			delete(renderer.agentStream.items, key)
		}
	}
	return nil
}

func (renderer *chatRenderer) agentText(event client.RealtimeEvent, activity model.AgentActivity) error {
	key := keyForAgentActivity(activity)
	item := renderer.agentStream.items[key]
	if item == nil {
		if len(renderer.agentStream.items) == maxAgentRenderItems {
			var oldest agentRenderKey
			var order uint64
			for candidate, stored := range renderer.agentStream.items {
				if order == 0 || stored.order < order {
					oldest, order = candidate, stored.order
				}
			}
			if err := renderer.flushAgentItem(oldest, renderer.agentStream.items[oldest]); err != nil {
				return err
			}
			delete(renderer.agentStream.items, oldest)
		}
		renderer.agentStream.next++
		item = &agentRenderItem{order: renderer.agentStream.next}
		renderer.agentStream.items[key] = item
	}
	item.pending += activity.Text
	for {
		end := strings.IndexByte(item.pending, '\n')
		if end >= 0 {
			if err := renderer.writeAgentSegment(key, item, item.pending[:end]); err != nil {
				return err
			}
			item.pending = item.pending[end+1:]
			continue
		}
		if len(item.pending) <= maxAgentRenderBytes {
			break
		}
		end = maxAgentRenderBytes
		for !utf8.RuneStart(item.pending[end]) {
			end--
		}
		if err := renderer.writeAgentSegment(key, item, item.pending[:end]); err != nil {
			return err
		}
		item.pending = item.pending[end:]
	}
	if activity.Truncated && !item.truncated {
		item.truncated = true
		if err := renderer.system("Native output was truncated."); err != nil {
			return err
		}
	}
	if activity.Status == model.AgentActivityCompleted {
		if err := renderer.flushAgentItem(key, item); err != nil {
			return err
		}
		delete(renderer.agentStream.items, key)
		return renderer.console.SetActivityDetail(event.JobID, event.StepID, agentActivityRoleLabel(activity))
	}
	if item.pending != "" && !(key.kind == model.AgentActivityReasoningSummary && item.headingShown && renderer.verbosity == chatNormal) {
		excerpt := liveAgentExcerpt(item.pending)
		if key.kind == model.AgentActivityCommandOutput {
			excerpt = "Output: " + excerpt
		}
		if role := agentActivityRoleLabel(activity); role != "" {
			excerpt = role + " · " + excerpt
		}
		return renderer.console.SetActivityDetail(event.JobID, event.StepID, excerpt)
	}
	return nil
}

func (renderer *chatRenderer) writeAgentSegment(key agentRenderKey, item *agentRenderItem, text string) error {
	label := "omni>"
	switch key.kind {
	case model.AgentActivityReasoningSummary:
		label = "thinking>"
		if renderer.verbosity == chatNormal {
			if item.headingShown || strings.TrimSpace(text) == "" {
				return nil
			}
			text = shortAgentExcerpt(text)
		}
		item.headingShown = item.headingShown || strings.TrimSpace(text) != ""
	case model.AgentActivityCommandOutput:
		if renderer.verbosity != chatVerbose && renderer.verbosity != chatDebug {
			return nil
		}
		label = "output>"
	}
	var rendered strings.Builder
	renderBlock(&rendered, label, text)
	return renderer.console.WriteOutput(rendered.String())
}

func (renderer *chatRenderer) flushAgentItem(key agentRenderKey, item *agentRenderItem) error {
	if item.pending == "" {
		return nil
	}
	if err := renderer.writeAgentSegment(key, item, item.pending); err != nil {
		return err
	}
	item.pending = ""
	return nil
}

func (renderer *chatRenderer) finishCommandOutput(activity model.AgentActivity) error {
	if activity.Status == model.AgentActivityStarted {
		return nil
	}
	key := keyForAgentActivity(activity)
	key.kind = model.AgentActivityCommandOutput
	if item := renderer.agentStream.items[key]; item != nil {
		if err := renderer.flushAgentItem(key, item); err != nil {
			return err
		}
		delete(renderer.agentStream.items, key)
	}
	return nil
}

func keyForAgentActivity(activity model.AgentActivity) agentRenderKey {
	return agentRenderKey{source: string(activity.Source), task: activity.TaskID, check: activity.CheckID,
		thread: activity.ThreadID, turn: activity.TurnID, item: activity.ItemID, kind: activity.Kind}
}

func shortAgentExcerpt(text string) string {
	characters := []rune(strings.TrimSpace(strings.SplitN(text, "\n", 2)[0]))
	const limit = 120
	if len(characters) > limit {
		return string(characters[:limit-1]) + "…"
	}
	return string(characters)
}

func liveAgentExcerpt(text string) string {
	characters := []rune(strings.TrimSpace(text))
	const limit = 120
	if len(characters) > limit {
		return "…" + string(characters[len(characters)-limit+1:])
	}
	return string(characters)
}
