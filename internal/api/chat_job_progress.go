package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	maxChatProgressRawBytes     = 8 << 10
	maxChatProgressSummaryRunes = 600
)

type chatProgressKind string

const (
	chatProgressActivity     chatProgressKind = "activity"
	chatProgressStation      chatProgressKind = "station"
	chatProgressRetrieval    chatProgressKind = "retrieval"
	chatProgressReview       chatProgressKind = "review"
	chatProgressFile         chatProgressKind = "file"
	chatProgressVerification chatProgressKind = "verification"
	chatProgressDiagnostic   chatProgressKind = "diagnostic"
)

type chatProgressEvent struct {
	ContextID  int64
	StepID     int64
	Generation int64
	OccurredAt time.Time
	Kind       chatProgressKind
	Summary    string
}

type parsedChatStepEvent struct {
	OccurredAt time.Time
	Type       string
	Message    string
}

func projectChatProgress(page queue.JobProgressPage) ([]chatProgressEvent, error) {
	if page.JobID <= 0 || page.Generation <= 0 || len(page.Items) > 24 {
		return nil, fmt.Errorf("chat progress requires bounded positive job-generation authority")
	}
	events := make([]chatProgressEvent, 0, len(page.Items))
	var priorID int64
	for _, item := range page.Items {
		if item.Context.ID <= priorID || item.Generation != page.Generation {
			return nil, fmt.Errorf("chat progress contexts are not strictly ordered in generation %d", page.Generation)
		}
		priorID = item.Context.ID
		event, err := projectChatProgressContext(item)
		if err != nil {
			return nil, fmt.Errorf("project progress context %d: %w", item.Context.ID, err)
		}
		events = append(events, event)
	}
	if (len(events) == 0 && page.LatestContextID != 0) ||
		(len(events) > 0 && page.LatestContextID != events[len(events)-1].ContextID) {
		return nil, fmt.Errorf("chat progress latest context identity is inconsistent")
	}
	return events, nil
}

func projectChatProgressContext(item queue.JobProgressContext) (chatProgressEvent, error) {
	contextValue := item.Context
	if contextValue.ID <= 0 || contextValue.StepID <= 0 || item.Generation <= 0 ||
		contextValue.CreatedAt.IsZero() || strings.TrimSpace(item.StepAction) == "" {
		return chatProgressEvent{}, fmt.Errorf("progress context has incomplete authority")
	}
	if len(contextValue.Value) > maxChatProgressRawBytes || !utf8.ValidString(contextValue.Value) ||
		strings.ContainsRune(contextValue.Value, '\x00') || contextValue.Value != strings.TrimSpace(contextValue.Value) {
		return chatProgressEvent{}, fmt.Errorf("progress value is not bounded canonical UTF-8")
	}
	event := chatProgressEvent{
		ContextID: contextValue.ID, StepID: contextValue.StepID, Generation: item.Generation,
		OccurredAt: contextValue.CreatedAt,
	}
	if contextValue.Key != "event" {
		return chatProgressEvent{}, fmt.Errorf("progress key %q is not registered", contextValue.Key)
	}
	parsed, err := parseChatStepEvent(contextValue.Value)
	if err != nil {
		return chatProgressEvent{}, err
	}
	event.OccurredAt = parsed.OccurredAt
	event.Kind, event.Summary, err = summarizeChatStepEvent(parsed, item.StepAction)
	if err != nil {
		return chatProgressEvent{}, err
	}
	if strings.TrimSpace(event.Summary) == "" {
		return chatProgressEvent{}, fmt.Errorf("progress summary is empty")
	}
	return event, nil
}

func parseChatStepEvent(raw string) (parsedChatStepEvent, error) {
	first, rest, ok := cutChatProgressToken(raw)
	if !ok || !strings.HasPrefix(first, "time=") {
		return parsedChatStepEvent{}, fmt.Errorf("event requires a leading time field")
	}
	second, message, ok := cutChatProgressToken(rest)
	if !ok || !strings.HasPrefix(second, "event=") {
		return parsedChatStepEvent{}, fmt.Errorf("event requires a second event field")
	}
	timestamp := strings.TrimPrefix(first, "time=")
	occurredAt, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || occurredAt.Location() != time.UTC || occurredAt.Format(time.RFC3339) != timestamp {
		return parsedChatStepEvent{}, fmt.Errorf("event time must be canonical UTC RFC3339")
	}
	eventType := strings.TrimPrefix(second, "event=")
	if !validChatProgressToken(eventType, 96) {
		return parsedChatStepEvent{}, fmt.Errorf("event type %q is not a canonical token", eventType)
	}
	return parsedChatStepEvent{OccurredAt: occurredAt, Type: eventType, Message: message}, nil
}

func cutChatProgressToken(value string) (string, string, bool) {
	value = strings.TrimLeft(value, " \t\r\n")
	if value == "" {
		return "", "", false
	}
	index := strings.IndexAny(value, " \t\r\n")
	if index < 0 {
		return value, "", true
	}
	return value[:index], strings.TrimLeft(value[index:], " \t\r\n"), true
}

func exactChatEventFields(message string, keys ...string) (map[string]string, error) {
	if len(keys) == 0 {
		if message != "" {
			return nil, fmt.Errorf("event does not permit a message")
		}
		return map[string]string{}, nil
	}
	values := make(map[string]string, len(keys))
	rest := message
	for index, key := range keys {
		prefix := key + "="
		if !strings.HasPrefix(rest, prefix) {
			return nil, fmt.Errorf("event requires ordered field %q", key)
		}
		rest = strings.TrimPrefix(rest, prefix)
		value := rest
		if index < len(keys)-1 {
			next := " " + keys[index+1] + "="
			boundary := strings.Index(rest, next)
			if boundary < 0 {
				return nil, fmt.Errorf("event requires ordered field %q", keys[index+1])
			}
			value = rest[:boundary]
			rest = rest[boundary+1:]
		} else {
			rest = ""
		}
		if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("event field %q is not canonical text", key)
		}
		values[key] = value
	}
	if rest != "" {
		return nil, fmt.Errorf("event contains unparsed fields")
	}
	return values, nil
}

func requireChatEventToken(fields map[string]string, key string, maxBytes int) (string, error) {
	value := fields[key]
	if !validChatProgressToken(value, maxBytes) {
		return "", fmt.Errorf("event field %q must be one token of at most %d bytes", key, maxBytes)
	}
	return value, nil
}

func requireChatEventInteger(fields map[string]string, key string, allowZero bool) (int, error) {
	value := fields[key]
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 || (!allowZero && parsed == 0) || strconv.Itoa(parsed) != value {
		return 0, fmt.Errorf("event field %q must be one canonical integer", key)
	}
	return parsed, nil
}

func requireChatEventAttempt(fields map[string]string) (string, error) {
	value := fields["attempt"]
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("event attempt must contain current and maximum")
	}
	current, firstErr := strconv.Atoi(parts[0])
	maximum, secondErr := strconv.Atoi(parts[1])
	if firstErr != nil || secondErr != nil || current <= 0 || maximum <= 0 || current > maximum ||
		fmt.Sprintf("%d/%d", current, maximum) != value {
		return "", fmt.Errorf("event attempt is not canonical")
	}
	return value, nil
}

func validChatProgressToken(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			(index > 0 && (character == '_' || character == '-' || character == '.' || character == ':' || character == '/')) {
			continue
		}
		return false
	}
	return true
}

func boundedChatProgressText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= maxChatProgressSummaryRunes {
		return value
	}
	return string(runes[:maxChatProgressSummaryRunes-14]) + " … [truncated]"
}
