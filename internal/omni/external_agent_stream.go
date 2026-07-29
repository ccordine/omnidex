package omni

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const externalAgentCleanupTimeout = 5 * time.Second

// StreamExternalAgentSession runs an external agent session, invoking onEvent for each
// streamed event before returning the aggregated result.
func StreamExternalAgentSession(ctx context.Context, session ExternalAgentSession, job ExternalAgentJob, onEvent func(AgentEvent) error) (result ExternalCodingResult, returnErr error) {
	if session == nil {
		return result, fmt.Errorf("external agent session is nil")
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), externalAgentCleanupTimeout)
		defer cancel()
		if err := session.Cleanup(cleanupCtx); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("clean up external agent session: %w", err))
		}
	}()

	events, err := session.Start(ctx, job)
	if err != nil {
		return result, fmt.Errorf("start external agent session: %w", err)
	}
	if events == nil {
		return result, fmt.Errorf("external agent session returned a nil event stream")
	}
	collected := make([]AgentEvent, 0, 32)
	for {
		select {
		case <-ctx.Done():
			cancelErr := session.Cancel(context.WithoutCancel(ctx), "external agent context ended")
			return result, errors.Join(ctx.Err(), wrapExternalAgentCancelError(cancelErr))
		case event, ok := <-events:
			if !ok {
				if err := validateExternalAgentEventSequence(collected); err != nil {
					return resultFromExternalAgentEventSlice(collected), err
				}
				result = resultFromExternalAgentEventSlice(collected)
				if err := externalAgentResultError(result); err != nil {
					return result, err
				}
				return result, nil
			}
			event.Type = AgentEventType(strings.ToLower(strings.TrimSpace(string(event.Type))))
			if event.SessionID == "" {
				event.SessionID = job.SessionID
			}
			if event.Agent == "" {
				event.Agent = job.Agent
			}
			collected = append(collected, event)
			if onEvent != nil {
				if err := onEvent(event); err != nil {
					cancelErr := session.Cancel(context.WithoutCancel(ctx), "external agent event consumer failed")
					return result, errors.Join(err, wrapExternalAgentCancelError(cancelErr))
				}
			}
		}
	}
}

func resultFromExternalAgentEventSlice(events []AgentEvent) ExternalCodingResult {
	replay := make(chan AgentEvent, len(events))
	for _, event := range events {
		replay <- event
	}
	close(replay)
	return resultFromExternalAgentEvents(replay)
}

func validateExternalAgentEventSequence(events []AgentEvent) error {
	if len(events) == 0 {
		return fmt.Errorf("external agent stream ended without events")
	}
	started := false
	completedAt := -1
	for index, event := range events {
		if !event.Type.Valid() {
			return fmt.Errorf("external agent stream event %d has unsupported type %q", index+1, event.Type)
		}
		if strings.TrimSpace(event.Agent) == "" {
			return fmt.Errorf("external agent stream event %d is missing agent", index+1)
		}
		if strings.TrimSpace(event.SessionID) == "" {
			return fmt.Errorf("external agent stream event %d is missing session_id", index+1)
		}
		switch event.Type {
		case AgentEventError:
			message := strings.TrimSpace(event.Message)
			if message == "" {
				message = "external agent reported an error"
			}
			return fmt.Errorf("external agent failed: %s", message)
		case AgentEventInterrupted:
			message := strings.TrimSpace(event.Message)
			if message == "" {
				message = "external agent session was interrupted"
			}
			return fmt.Errorf("external agent interrupted: %s", message)
		case AgentEventStatus:
			if err := validateExternalAgentStatusEvent(event); err != nil {
				return err
			}
		}
		if completedAt >= 0 {
			if event.Type == AgentEventCompleted {
				return fmt.Errorf("external agent stream contains a duplicate completed event")
			}
			return fmt.Errorf("external agent stream emitted %q after its completed event", event.Type)
		}
		switch event.Type {
		case AgentEventStarted:
			if started {
				return fmt.Errorf("external agent stream contains a duplicate started event")
			}
			started = true
		case AgentEventCompleted:
			if !started {
				return fmt.Errorf("external agent stream completed before a started event")
			}
			if err := validateExternalAgentStatusEvent(event); err != nil {
				return err
			}
			completedAt = index
		}
	}
	if completedAt < 0 {
		return fmt.Errorf("external agent stream ended without a completed event")
	}
	return nil
}

func validateExternalAgentStatusEvent(event AgentEvent) error {
	if len(event.Raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Raw, &payload); err != nil {
		return fmt.Errorf("external agent %s event has invalid raw payload: %w", event.Type, err)
	}
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["status"])))
	switch status {
	case "error", "failed", "cancelled", "canceled":
		return fmt.Errorf("external agent failed with status %s: %s", status, externalAgentStatusFailureDetail(payload, event.Message))
	default:
		return nil
	}
}

func wrapExternalAgentCancelError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("cancel external agent session: %w", err)
}

func AgentEventJSONLine(event AgentEvent) (string, error) {
	blob, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("encode external agent event: %w", err)
	}
	return string(blob), nil
}
