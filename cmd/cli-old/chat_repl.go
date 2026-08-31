package main

import (
	"fmt"
	"strings"
	"time"
)

func handleChatReplCommand(line string, sessionID *string, lastJobID *int64, ui *chatUI) (bool, bool) {
	command, body := parseSlashCommand(line)
	switch command {
	case "exit", "quit":
		return true, true
	case "help":
		printInteractiveChatHelp()
		return true, false
	case "session":
		if strings.TrimSpace(body) == "" {
			emitSystem(ui, "session="+strings.TrimSpace(*sessionID))
			return true, false
		}
		next := strings.TrimSpace(body)
		if next == "" {
			emitSystem(ui, "usage: /session <id>")
			return true, false
		}
		*sessionID = next
		*lastJobID = 0
		emitSystem(ui, "switched to session="+*sessionID)
		return true, false
	case "new":
		*sessionID = fmt.Sprintf("chat-%d", time.Now().Unix())
		*lastJobID = 0
		emitSystem(ui, "started new session="+*sessionID)
		return true, false
	case "last":
		if *lastJobID > 0 {
			emitSystem(ui, fmt.Sprintf("last-job=%d", *lastJobID))
		} else {
			emitSystem(ui, "no prior turns in this chat session")
		}
		return true, false
	default:
		emitSystem(ui, "unknown command. type /help")
		return true, false
	}
}

func parseSlashCommand(line string) (string, string) {
	raw := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	if raw == "" {
		return "", ""
	}
	parts := strings.SplitN(raw, " ", 2)
	command := strings.ToLower(strings.TrimSpace(parts[0]))
	body := ""
	if len(parts) > 1 {
		body = strings.TrimSpace(parts[1])
	}
	return command, body
}

func printInteractiveChatHelp() {
	fmt.Println("interactive commands:")
	fmt.Println("  /help              show this help")
	fmt.Println("  /session           show current session id")
	fmt.Println("  /session <id>      switch to a specific session id")
	fmt.Println("  /new               start a fresh session id")
	fmt.Println("  /last              show most recent job id")
	fmt.Println("  /exit              quit interactive mode")
	fmt.Println("  progress note      live stage/event updates are shown by default (disable with --progress=false)")
	fmt.Println("  phase note         stage lines include phase=coding|objective|execution")
	fmt.Println("  queue note         while assistant is running, type TAB + message + Enter to queue a follow-up for the next turn")
	fmt.Println("  routing note       every non-slash message is sent unchanged to the core semantic pipeline")
	fmt.Println("  host operations    use explicit top-level commands: browser-scan, screen-read, audio-notes, media-index, media-search")
}

func printInteractiveInputHelp() {
	fmt.Println("feedback mode commands:")
	fmt.Println("  /interrupt <text>  inject context into the active job")
	fmt.Println("  /replan <text>     create a new generation on the same job with new context")
	fmt.Println("  /cancel <reason>   stop the active job")
	fmt.Println("  note               during a running turn, type these slash commands directly; TAB + text queues a follow-up")
	fmt.Println("  /exit              quit interactive mode")
}
