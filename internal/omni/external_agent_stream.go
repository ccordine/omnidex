package omni

import (
	"context"
	"encoding/json"
)

// StreamExternalAgentSession runs an external agent session, invoking onEvent for each
// streamed event before returning the aggregated result.
func StreamExternalAgentSession(ctx context.Context, session ExternalAgentSession, job ExternalAgentJob, onEvent func(AgentEvent) error) (CursorArchitectAgentResult, error) {
	events, err := session.Start(ctx, job)
	if err != nil {
		return CursorArchitectAgentResult{}, err
	}
	collected := make([]AgentEvent, 0, 32)
	for event := range events {
		collected = append(collected, event)
		if onEvent != nil {
			if err := onEvent(event); err != nil {
				return CursorArchitectAgentResult{}, err
			}
		}
	}
	replay := make(chan AgentEvent, len(collected))
	for _, event := range collected {
		replay <- event
	}
	close(replay)
	result := resultFromExternalAgentEvents(replay)
	if err := externalAgentResultError(result); err != nil {
		return result, err
	}
	return result, nil
}

func AgentEventJSONLine(event AgentEvent) string {
	blob, err := json.Marshal(event)
	if err != nil {
		return ""
	}
	return string(blob)
}
