package workingset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/gryph/omnidex/internal/taskstate"
)

type EventKind string

const (
	EventAcquired         EventKind = "acquired"
	EventRetained         EventKind = "retained"
	EventReleased         EventKind = "released"
	EventTouched          EventKind = "touched"
	EventInvalidatedStale EventKind = "invalidated_stale"
	EventScopeClosed      EventKind = "scope_closed"
)

type Event struct {
	SetID         SetID               `json:"working_set_id"`
	Version       uint64              `json:"working_set_version"`
	CommandID     CommandID           `json:"command_id"`
	CommandSHA256 string              `json:"command_sha256"`
	CommandKind   CommandKind         `json:"command_kind"`
	Kind          EventKind           `json:"event_kind"`
	Actor         taskstate.Authority `json:"actor"`
	Command       json.RawMessage     `json:"command"`
}

func (set *Set) Apply(command Command) (Event, error) {
	if err := set.validateCommandAggregate(); err != nil {
		return Event{}, err
	}
	descriptor, err := DescribeCommand(command)
	if err != nil {
		return Event{}, err
	}
	if existing, exists := set.commandEvents[descriptor.ID]; exists {
		if existing.CommandSHA256 != descriptor.SHA256 {
			return Event{}, fmt.Errorf("%w: command ID %q was reused with different content", ErrCommandIDConflict, descriptor.ID)
		}
		return cloneEvent(existing), nil
	}
	if descriptor.ExpectedVersion != set.version {
		return Event{}, VersionConflictError{Expected: descriptor.ExpectedVersion, Actual: set.version}
	}
	before := set.version
	if err := command.decide(set); err != nil {
		return Event{}, err
	}
	if set.version != before+1 {
		return Event{}, fmt.Errorf("%w: command %q did not advance exactly one version", ErrInvalidSet, descriptor.ID)
	}
	event := Event{
		SetID: set.id, Version: set.version, CommandID: descriptor.ID,
		CommandSHA256: descriptor.SHA256, CommandKind: descriptor.Kind,
		Kind: eventKindForCommand(descriptor.Kind), Actor: descriptor.Actor,
		Command: append(json.RawMessage(nil), descriptor.Raw...),
	}
	if err := ValidateEvent(event); err != nil {
		return Event{}, fmt.Errorf("%w: generated command event is invalid: %v", ErrInvalidSet, err)
	}
	set.commandEvents[event.CommandID] = cloneEvent(event)
	return cloneEvent(event), nil
}

func ValidateEvent(event Event) error {
	if !setIDPattern.MatchString(string(event.SetID)) || event.Version == 0 ||
		!commandIDPattern.MatchString(string(event.CommandID)) {
		return fmt.Errorf("%w: event identity is invalid", ErrInvalidSet)
	}
	command, err := DecodeCommand(event.CommandKind, event.Command)
	if err != nil {
		return err
	}
	descriptor, err := DescribeCommand(command)
	if err != nil {
		return err
	}
	if event.CommandID != descriptor.ID || event.CommandSHA256 != descriptor.SHA256 ||
		event.Actor != descriptor.Actor || event.Version != descriptor.ExpectedVersion+1 ||
		event.Kind != eventKindForCommand(event.CommandKind) {
		return fmt.Errorf("%w: event columns disagree with the exact command", ErrInvalidSet)
	}
	return nil
}

func DecodeCommand(kind CommandKind, raw []byte) (Command, error) {
	var command Command
	switch kind {
	case CommandAcquire:
		command = &AcquireCommand{}
	case CommandRetain:
		command = &RetainCommand{}
	case CommandRelease:
		command = &ReleaseCommand{}
	case CommandTouch:
		command = &TouchCommand{}
	case CommandInvalidateStale:
		command = &InvalidateStaleCommand{}
	case CommandCloseScope:
		command = &CloseScopeCommand{}
	default:
		return nil, fmt.Errorf("%w: command kind %q is not registered", ErrInvalidCommand, kind)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(command); err != nil {
		return nil, fmt.Errorf("%w: decode %s command: %v", ErrInvalidCommand, kind, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: command contains trailing JSON", ErrInvalidCommand)
	}
	if command.kind() != kind {
		return nil, fmt.Errorf("%w: decoded command kind disagrees with its envelope", ErrInvalidCommand)
	}
	return command, nil
}

func Reconstruct(owner Owner, budget Budget, events []Event) (*Set, error) {
	set, err := New(owner, budget)
	if err != nil {
		return nil, err
	}
	for index, supplied := range events {
		if supplied.SetID != set.id || supplied.Version != set.version+1 {
			return nil, fmt.Errorf("%w: event %d is outside the expected stream", ErrInvalidSet, index)
		}
		command, err := DecodeCommand(supplied.CommandKind, supplied.Command)
		if err != nil {
			return nil, fmt.Errorf("event %d: %w", index, err)
		}
		actual, err := set.Apply(command)
		if err != nil {
			return nil, fmt.Errorf("event %d cannot replay: %w", index, err)
		}
		if !reflect.DeepEqual(actual, supplied) {
			return nil, fmt.Errorf("%w: event %d replay disagrees with persisted event", ErrInvalidSet, index)
		}
	}
	return set, nil
}

func (set *Set) validateCommandAggregate() error {
	if set == nil || set.commandEvents == nil {
		return fmt.Errorf("%w: command aggregate is uninitialized", ErrInvalidSet)
	}
	return nil
}

func eventKindForCommand(kind CommandKind) EventKind {
	switch kind {
	case CommandAcquire:
		return EventAcquired
	case CommandRetain:
		return EventRetained
	case CommandRelease:
		return EventReleased
	case CommandTouch:
		return EventTouched
	case CommandInvalidateStale:
		return EventInvalidatedStale
	case CommandCloseScope:
		return EventScopeClosed
	default:
		return ""
	}
}

func cloneEvent(event Event) Event {
	event.Command = append(json.RawMessage(nil), event.Command...)
	return event
}

type VersionConflictError struct {
	Expected uint64
	Actual   uint64
}

func (conflict VersionConflictError) Error() string {
	return fmt.Sprintf("%s: expected %d, actual %d", ErrVersionConflict, conflict.Expected, conflict.Actual)
}

func (VersionConflictError) Unwrap() error { return ErrVersionConflict }
