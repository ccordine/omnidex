package agentstream

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

const MaxEventBytes = 1 << 20

type EventType string

const (
	EventStarted     EventType = "started"
	EventStatus      EventType = "status"
	EventMessage     EventType = "message"
	EventThinking    EventType = "thinking"
	EventCommand     EventType = "command"
	EventTool        EventType = "tool"
	EventFileChange  EventType = "file_change"
	EventCompleted   EventType = "completed"
	EventError       EventType = "error"
	EventInterrupted EventType = "interrupted"
)

func (eventType EventType) Valid() bool {
	switch eventType {
	case EventStarted, EventStatus, EventMessage, EventThinking, EventCommand,
		EventTool, EventFileChange, EventCompleted, EventError, EventInterrupted:
		return true
	default:
		return false
	}
}

type Event struct {
	SessionID string          `json:"session_id,omitempty"`
	Agent     string          `json:"agent"`
	Type      EventType       `json:"type"`
	Message   string          `json:"message"`
	Command   string          `json:"command,omitempty"`
	Files     []string        `json:"files,omitempty"`
	Evidence  []string        `json:"evidence,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

func DecodeLine(line string) (Event, error) {
	return decodeLine(line, true)
}

func DecodeBoundaryLine(line string) (Event, error) {
	return decodeLine(line, false)
}

func EncodeLine(event Event) (string, error) {
	return encodeLine(event, true)
}

func EncodeBoundaryLine(event Event) (string, error) {
	return encodeLine(event, false)
}

func Validate(event Event) error {
	return validateEvent(event, true)
}

func ValidateBoundary(event Event) error {
	return validateEvent(event, false)
}

func decodeLine(line string, requireSession bool) (Event, error) {
	if len(line) > MaxEventBytes {
		return Event{}, fmt.Errorf("external agent event has %d bytes; maximum is %d", len(line), MaxEventBytes)
	}
	if !utf8.ValidString(line) {
		return Event{}, fmt.Errorf("external agent event must be valid UTF-8")
	}
	var event Event
	if err := exactjson.ValidateObject([]byte(line), Event{}, "external agent event"); err != nil {
		return Event{}, fmt.Errorf("decode external agent event: %w", err)
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return Event{}, fmt.Errorf("decode external agent event fields: %w", err)
	}
	if err := validateEvent(event, requireSession); err != nil {
		return Event{}, err
	}
	return event, nil
}

func encodeLine(event Event, requireSession bool) (string, error) {
	if err := validateEvent(event, requireSession); err != nil {
		return "", err
	}
	blob, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("encode external agent event: %w", err)
	}
	if len(blob) > MaxEventBytes {
		return "", fmt.Errorf("external agent event has %d bytes; maximum is %d", len(blob), MaxEventBytes)
	}
	return string(blob), nil
}

func validateEvent(event Event, requireSession bool) error {
	if !event.Type.Valid() {
		return fmt.Errorf("external agent event has unsupported type %q", event.Type)
	}
	if err := validateIdentity("agent", event.Agent, true); err != nil {
		return err
	}
	if err := validateIdentity("session_id", event.SessionID, requireSession); err != nil {
		return err
	}
	if event.Message == "" {
		return fmt.Errorf("external agent event message is required")
	}
	for name, value := range map[string]string{
		"message": event.Message,
		"command": event.Command,
	} {
		if err := validateText(name, value); err != nil {
			return err
		}
	}
	for index, value := range event.Files {
		if err := validateText(fmt.Sprintf("files[%d]", index), value); err != nil {
			return err
		}
	}
	for index, value := range event.Evidence {
		if err := validateText(fmt.Sprintf("evidence[%d]", index), value); err != nil {
			return err
		}
	}
	if len(event.Raw) > 0 && !json.Valid(event.Raw) {
		return fmt.Errorf("external agent event raw payload must be valid JSON")
	}
	return nil
}

func validateIdentity(name, value string, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("external agent event %s is required", name)
		}
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("external agent event %s must be canonical without surrounding whitespace", name)
	}
	return validateText(name, value)
}

func validateText(name, value string) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("external agent event %s must be PostgreSQL-compatible UTF-8", name)
	}
	return nil
}
