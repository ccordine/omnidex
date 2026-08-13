package omni

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentstream"
)

const externalAgentCleanupTimeout = 5 * time.Second

// StreamExternalAgentSession runs an external agent session, invoking onEvent for each
// streamed event before returning the aggregated result.
func StreamExternalAgentSession(ctx context.Context, session ExternalAgentSession, job ExternalAgentJob, onEvent func(agentstream.Event) error) (result ExternalCodingResult, returnErr error) {
	if session == nil {
		return result, fmt.Errorf("external agent session is nil")
	}
	if strings.TrimSpace(job.Agent) == "" || strings.TrimSpace(job.SessionID) == "" {
		return result, fmt.Errorf("external agent job requires typed agent and session_id")
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
	collected := make([]agentstream.Event, 0, 32)
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
				return result, nil
			}
			if event.SessionID == "" {
				event.SessionID = job.SessionID
			}
			if event.Agent == "" {
				event.Agent = job.Agent
			}
			if event.SessionID != job.SessionID {
				return result, fmt.Errorf("external agent stream session_id %q differs from job session_id %q", event.SessionID, job.SessionID)
			}
			if event.Agent != job.Agent {
				return result, fmt.Errorf("external agent stream agent %q differs from job agent %q", event.Agent, job.Agent)
			}
			if err := agentstream.Validate(event); err != nil {
				return result, fmt.Errorf("validate external agent stream event %d: %w", len(collected)+1, err)
			}
			if err := validateNextExternalAgentEvent(collected, event); err != nil {
				return result, err
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

func validateNextExternalAgentEvent(events []agentstream.Event, event agentstream.Event) error {
	started := false
	for _, previous := range events {
		switch previous.Type {
		case agentstream.EventStarted:
			started = true
		case agentstream.EventCompleted:
			if event.Type == agentstream.EventCompleted {
				return fmt.Errorf("external agent stream contains a duplicate completed event")
			}
			return fmt.Errorf("external agent stream emitted %q after its completed event", event.Type)
		case agentstream.EventError, agentstream.EventInterrupted:
			return fmt.Errorf("external agent stream emitted %q after terminal %q event", event.Type, previous.Type)
		}
	}
	if event.Type == agentstream.EventStarted {
		if started || len(events) > 0 {
			return fmt.Errorf("external agent stream contains a duplicate or late started event")
		}
		return nil
	}
	if event.Type == agentstream.EventError || event.Type == agentstream.EventInterrupted {
		return nil
	}
	if !started {
		return fmt.Errorf("external agent stream emitted %q before a started event", event.Type)
	}
	return nil
}

func resultFromExternalAgentEventSlice(events []agentstream.Event) ExternalCodingResult {
	replay := make(chan agentstream.Event, len(events))
	for _, event := range events {
		replay <- event
	}
	close(replay)
	return resultFromExternalAgentEvents(replay)
}

func validateExternalAgentEventSequence(events []agentstream.Event) error {
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
		case agentstream.EventError:
			message := strings.TrimSpace(event.Message)
			if message == "" {
				message = "external agent reported an error"
			}
			return fmt.Errorf("external agent failed: %s", message)
		case agentstream.EventInterrupted:
			message := strings.TrimSpace(event.Message)
			if message == "" {
				message = "external agent session was interrupted"
			}
			return fmt.Errorf("external agent interrupted: %s", message)
		}
		if completedAt >= 0 {
			if event.Type == agentstream.EventCompleted {
				return fmt.Errorf("external agent stream contains a duplicate completed event")
			}
			return fmt.Errorf("external agent stream emitted %q after its completed event", event.Type)
		}
		switch event.Type {
		case agentstream.EventStarted:
			if started {
				return fmt.Errorf("external agent stream contains a duplicate started event")
			}
			started = true
		case agentstream.EventCompleted:
			if !started {
				return fmt.Errorf("external agent stream completed before a started event")
			}
			completedAt = index
		}
	}
	if completedAt < 0 {
		return fmt.Errorf("external agent stream ended without a completed event")
	}
	return nil
}

func wrapExternalAgentCancelError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("cancel external agent session: %w", err)
}
