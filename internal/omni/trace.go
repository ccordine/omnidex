package omni

import (
	"fmt"
	"strings"
	"time"
)

type RunTrace struct {
	Workspace         string         `json:"workspace"`
	WorkspaceID       string         `json:"workspace_id"`
	GeneratedAt       string         `json:"generated_at"`
	TurnCount         int            `json:"turn_count"`
	TotalEvents       int            `json:"total_events"`
	EstimatedDuration string         `json:"estimated_duration,omitempty"`
	ModelCalls        int            `json:"model_calls"`
	ModelFailures     int            `json:"model_failures"`
	Commands          int            `json:"commands"`
	CommandFailures   int            `json:"command_failures"`
	RejectedCommands  int            `json:"rejected_commands"`
	DoneRejections    int            `json:"done_rejections"`
	LoopExhaustions   int            `json:"loop_exhaustions"`
	ObjectiveEvents   int            `json:"objective_events"`
	CompletionChecks  int            `json:"completion_checks"`
	EventCounts       map[string]int `json:"event_counts"`
	Turns             []RunTraceTurn `json:"turns,omitempty"`
}

type RunTraceTurn struct {
	ID                string         `json:"id"`
	CreatedAt         string         `json:"created_at"`
	EventCount        int            `json:"event_count"`
	EstimatedDuration string         `json:"estimated_duration,omitempty"`
	ModelCalls        int            `json:"model_calls"`
	Commands          int            `json:"commands"`
	RejectedCommands  int            `json:"rejected_commands"`
	DoneRejections    int            `json:"done_rejections"`
	LoopExhaustions   int            `json:"loop_exhaustions"`
	EventCounts       map[string]int `json:"event_counts"`
}

func BuildRunTrace(session *Session) RunTrace {
	trace := RunTrace{GeneratedAt: nowUTC(), EventCounts: map[string]int{}}
	if session == nil {
		return trace
	}
	trace.Workspace = session.WorkspacePath
	trace.WorkspaceID = session.WorkspaceHash
	trace.TurnCount = len(session.Turns)
	var first, last time.Time
	for _, turn := range session.Turns {
		turnTrace := RunTraceTurn{
			ID:          turn.ID,
			CreatedAt:   turn.CreatedAt,
			EventCount:  len(turn.Events),
			EventCounts: map[string]int{},
		}
		var turnFirst, turnLast time.Time
		for _, event := range turn.Events {
			trace.TotalEvents++
			trace.EventCounts[event.Type]++
			turnTrace.EventCounts[event.Type]++
			updateTraceCounts(event.Type, &trace.ModelCalls, &trace.ModelFailures, &trace.Commands, &trace.CommandFailures, &trace.RejectedCommands, &trace.DoneRejections, &trace.LoopExhaustions, &trace.ObjectiveEvents, &trace.CompletionChecks)
			updateTraceCounts(event.Type, &turnTrace.ModelCalls, nil, &turnTrace.Commands, nil, &turnTrace.RejectedCommands, &turnTrace.DoneRejections, &turnTrace.LoopExhaustions, nil, nil)
			if ts, ok := parseEventTime(event.CreatedAt); ok {
				if first.IsZero() || ts.Before(first) {
					first = ts
				}
				if last.IsZero() || ts.After(last) {
					last = ts
				}
				if turnFirst.IsZero() || ts.Before(turnFirst) {
					turnFirst = ts
				}
				if turnLast.IsZero() || ts.After(turnLast) {
					turnLast = ts
				}
			}
		}
		if !turnFirst.IsZero() && !turnLast.IsZero() && turnLast.After(turnFirst) {
			turnTrace.EstimatedDuration = turnLast.Sub(turnFirst).Round(time.Millisecond).String()
		}
		trace.Turns = append(trace.Turns, turnTrace)
	}
	if !first.IsZero() && !last.IsZero() && last.After(first) {
		trace.EstimatedDuration = last.Sub(first).Round(time.Millisecond).String()
	}
	return trace
}

func updateTraceCounts(eventType string, modelCalls, modelFailures, commands, commandFailures, rejectedCommands, doneRejections, loopExhaustions, objectiveEvents, completionChecks *int) {
	switch eventType {
	case "structured_llm_request_started", "prompt_interpreter_completed", "minimal_context_updated", "completion_check_completed":
		increment(modelCalls)
	case "structured_llm_request_failed", "prompt_interpreter_failed", "minimal_context_failed", "completion_check_failed":
		increment(modelFailures)
	case "structured_command_finished", "structured_command_completed":
		increment(commands)
	case "structured_command_failed":
		increment(commandFailures)
	case "structured_command_rejected":
		increment(rejectedCommands)
	case "structured_done_rejected":
		increment(doneRejections)
	case "structured_loop_exhausted":
		increment(loopExhaustions)
	}
	switch eventType {
	case "prompt_interpreter_completed", "objective_ledger_reconciled", "recipe_selected":
		increment(objectiveEvents)
	case "completion_check_completed", "completion_check_accepted_from_context", "completion_check_accepted_from_observations":
		increment(completionChecks)
	}
}

func increment(value *int) {
	if value != nil {
		(*value)++
	}
}

func parseEventTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, raw)
	}
	return ts, err == nil
}

func formatRunTraceText(trace RunTrace) string {
	lines := []string{
		fmt.Sprintf("workspace=%s", trace.Workspace),
		fmt.Sprintf("turns=%d events=%d duration=%s", trace.TurnCount, trace.TotalEvents, emptyAs(trace.EstimatedDuration, "unknown")),
		fmt.Sprintf("model_calls=%d model_failures=%d", trace.ModelCalls, trace.ModelFailures),
		fmt.Sprintf("commands=%d command_failures=%d rejected_commands=%d done_rejections=%d loop_exhaustions=%d", trace.Commands, trace.CommandFailures, trace.RejectedCommands, trace.DoneRejections, trace.LoopExhaustions),
		fmt.Sprintf("objective_events=%d completion_checks=%d", trace.ObjectiveEvents, trace.CompletionChecks),
	}
	if len(trace.Turns) > 0 {
		last := trace.Turns[len(trace.Turns)-1]
		lines = append(lines, fmt.Sprintf("latest_turn=%s events=%d duration=%s model_calls=%d commands=%d rejected=%d", last.ID, last.EventCount, emptyAs(last.EstimatedDuration, "unknown"), last.ModelCalls, last.Commands, last.RejectedCommands))
	}
	return strings.Join(lines, "\n")
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
