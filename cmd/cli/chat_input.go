package main

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

type chatInputEvent struct {
	line string
	err  error
	eof  bool
}

type chatInputReader struct {
	events chan chatInputEvent
}

func newChatInputReader(scanner *bufio.Scanner) *chatInputReader {
	reader := &chatInputReader{
		events: make(chan chatInputEvent, 64),
	}
	go func() {
		if scanner == nil {
			reader.events <- chatInputEvent{eof: true}
			close(reader.events)
			return
		}
		for scanner.Scan() {
			reader.events <- chatInputEvent{line: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			reader.events <- chatInputEvent{err: err}
		} else {
			reader.events <- chatInputEvent{eof: true}
		}
		close(reader.events)
	}()
	return reader
}

func (r *chatInputReader) readBlocking() (string, bool, error) {
	if r == nil {
		return "", true, nil
	}
	event, ok := <-r.events
	if !ok {
		return "", true, nil
	}
	if event.err != nil {
		return "", false, event.err
	}
	if event.eof {
		return "", true, nil
	}
	return event.line, false, nil
}

func (r *chatInputReader) readNonBlocking() (chatInputEvent, bool) {
	if r == nil {
		return chatInputEvent{}, false
	}
	select {
	case event, ok := <-r.events:
		if !ok {
			return chatInputEvent{eof: true}, true
		}
		return event, true
	default:
		return chatInputEvent{}, false
	}
}

func executeConfirmedChatAction(
	c *client.Client,
	input *chatInputReader,
	session string,
	baseMetadata map[string]any,
	lastJobID *int64,
	pendingInputs *[]string,
	candidate *chatActionCandidate,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	localShell bool,
	shellState *localShellState,
	ui *chatUI,
) bool {
	if candidate == nil {
		return false
	}
	if trace := formatLocalAutomationTrace(candidate); trace != "" && strings.TrimSpace(candidate.Kind) != "core_job" {
		emitSystem(ui, trace)
	}

	restoreTrace := func() {}
	if strings.TrimSpace(candidate.Kind) != "core_job" {
		restoreTrace = installLocalExecutionTraceSink(func(line string) {
			emitSystem(ui, line)
		})
		defer restoreTrace()
	}

	switch candidate.Kind {
	case "local_media":
		handled, response := tryHandleLocalMediaCommand(candidate.Input)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local host media inspection/control output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	case "local_browser":
		handled, response := tryHandleLocalBrowserCommand(candidate.Input)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local browser process/tab/console inspection output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	case "local_screen":
		handled, response := tryHandleLocalScreenCommand(candidate.Input)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local screenshot/OCR/vision output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	case "local_shell":
		handled, response := tryHandleLocalShellCommand(candidate.Input, shellState)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local command execution output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	case "local_audio":
		handled, response := tryHandleLocalAudioNotesCommand(candidate.Input)
		if handled {
			emitAssistant(ui, strings.TrimSpace(formatLocalAutomationResponse(localAutomationSourceLine(candidate.Kind, "local audio notes command output"), response)))
			return runDeterministicLocalActionReview(
				c,
				input,
				session,
				baseMetadata,
				lastJobID,
				pendingInputs,
				candidate,
				response,
				interval,
				progress,
				verbose,
				maxChars,
				localShell,
				shellState,
				ui,
			)
		}
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	default:
		return executeChatCoreTurn(c, input, session, baseMetadata, lastJobID, pendingInputs, candidate.Input, specialistRoleIDForCandidateTurn(candidate), interval, progress, verbose, maxChars, localShell, shellState, ui)
	}
}

func promptChatPermissionDecision(input *chatInputReader, ui *chatUI, key, reason, storePath, description string) (bool, error) {
	reason = strings.TrimSpace(reason)
	description = strings.TrimSpace(description)
	if ui != nil {
		emitSystem(ui, "permission required:")
		emitSystem(ui, "  key: "+key)
		if description != "" {
			emitSystem(ui, "  description: "+description)
		}
		if reason != "" {
			emitSystem(ui, "  reason: "+reason)
		}
		if strings.TrimSpace(storePath) != "" {
			emitSystem(ui, "  store: "+storePath)
		}
	} else {
		fmt.Println("permission required:")
		fmt.Println("  key: " + key)
		if description != "" {
			fmt.Println("  description: " + description)
		}
		if reason != "" {
			fmt.Println("  reason: " + reason)
		}
		if strings.TrimSpace(storePath) != "" {
			fmt.Println("  store: " + storePath)
		}
	}
	for {
		fmt.Print("allow and save this permission? [y/n]: ")
		line, eof, err := input.readBlocking()
		if err != nil {
			return false, err
		}
		if eof {
			return false, fmt.Errorf("permission prompt closed before answer for %s", key)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			if ui != nil {
				emitSystem(ui, "please answer y or n")
			} else {
				fmt.Println("please answer y or n")
			}
		}
	}
}
