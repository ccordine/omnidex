package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const localAudioNotesCommandTimeout = 30 * time.Second
const localAudioNotesStopTimeout = 2 * time.Hour

type localAudioNotesIntent struct {
	Action string
	Query  string
}

func tryHandleLocalAudioNotesCommand(input string) (bool, string) {
	intent, ok := parseLocalAudioNotesIntent(input)
	if !ok {
		return false, ""
	}

	output, err := runLocalAudioNotesCommand(intent)
	if err != nil {
		message := strings.TrimSpace(output)
		if message == "" {
			message = err.Error()
		}
		return true, "Local audio notes action failed: " + message
	}
	return true, strings.TrimSpace(output)
}

func parseLocalAudioNotesIntent(input string) (localAudioNotesIntent, bool) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return localAudioNotesIntent{}, false
	}
	lower := strings.ToLower(clean)

	stopPhrases := []string{
		"stop taking notes",
		"stop notes",
		"stop audio notes",
		"end notes",
		"stop recording",
		"stop call notes",
	}
	if containsAnyPhrase(lower, stopPhrases) {
		return localAudioNotesIntent{Action: "stop"}, true
	}

	statusPhrases := []string{
		"notes status",
		"audio notes status",
		"call notes status",
		"are you taking notes",
		"are you recording",
	}
	if containsAnyPhrase(lower, statusPhrases) {
		return localAudioNotesIntent{Action: "status"}, true
	}

	searchMarkers := []string{
		"search notes for ",
		"search call notes for ",
		"find in notes ",
		"find quote in notes ",
	}
	for _, marker := range searchMarkers {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		query := strings.TrimSpace(clean[idx+len(marker):])
		query = strings.Trim(query, " \t\r\n\"'`.,!?;:()[]{}")
		if query != "" {
			return localAudioNotesIntent{Action: "search", Query: query}, true
		}
	}

	startPhrases := []string{
		"take notes during this call",
		"take notes during this meeting",
		"take notes",
		"start taking notes",
		"begin taking notes",
		"start audio notes",
		"start call notes",
		"record this call",
		"capture this call",
	}
	if containsAnyPhrase(lower, startPhrases) {
		return localAudioNotesIntent{Action: "start"}, true
	}

	return localAudioNotesIntent{}, false
}

func runLocalAudioNotesCommand(intent localAudioNotesIntent) (string, error) {
	binary := strings.TrimSpace(os.Args[0])
	if binary == "" {
		return "", errors.New("unable to determine current omni binary path")
	}

	args := []string{"audio-notes"}
	timeout := localAudioNotesCommandTimeout
	switch intent.Action {
	case "start":
		args = append(args, "start")
	case "stop":
		args = append(args, "stop")
		timeout = localAudioNotesStopTimeout
	case "status":
		args = append(args, "status")
	case "search":
		query := strings.TrimSpace(intent.Query)
		if query == "" {
			return "", errors.New("search query is required")
		}
		args = append(args, "search", query)
	default:
		return "", fmt.Errorf("unsupported local audio notes action: %s", intent.Action)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "OMNI_FRONTEND_TRACE=1")
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("audio-notes %s timed out after %s", intent.Action, timeout)
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, errors.New(text)
	}
	if text == "" {
		text = "audio notes action completed"
	}
	return text, nil
}
