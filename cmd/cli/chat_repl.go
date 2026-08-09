package main

import (
	"fmt"
	"strings"
	"time"
)

func handleChatReplCommand(line string, sessionID *string, lastJobID *int64, baseMetadata map[string]any, agentOverrides *cliAgentRuntimeConfig, ui *chatUI) (bool, bool) {
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
	case "agent":
		if strings.TrimSpace(body) == "" {
			emitSystem(ui, agentOverrides.Summary())
			return true, false
		}
		if isResetRuntimeValue(body) || strings.EqualFold(strings.TrimSpace(body), "core") || strings.EqualFold(strings.TrimSpace(body), "default") {
			agentOverrides.Clear()
			agentOverrides.ApplyToMetadata(baseMetadata)
			emitSystem(ui, "agent override cleared; core default will be used")
			return true, false
		}
		if err := agentOverrides.Set("agent_system", body); err != nil {
			emitAssistantError(ui, err.Error())
			return true, false
		}
		agentOverrides.ApplyToMetadata(baseMetadata)
		emitSystem(ui, agentOverrides.Summary())
		return true, false
	case "model":
		if strings.TrimSpace(body) == "" {
			emitSystem(ui, "usage: /model <model-name>")
			return true, false
		}
		message, err := setActiveChatModel(baseMetadata, agentOverrides, body)
		if err != nil {
			emitAssistantError(ui, err.Error())
			return true, false
		}
		agentOverrides.ApplyToMetadata(baseMetadata)
		emitSystem(ui, message)
		return true, false
	case "set":
		key, value, ok := parseRuntimeSetBody(body)
		if !ok {
			emitSystem(ui, "usage: /set <setting> <value>")
			return true, false
		}
		if normalizeRuntimeConfigKey(key) == "agent_model" || normalizeRuntimeConfigKey(key) == "model" {
			message, err := setActiveChatModel(baseMetadata, agentOverrides, value)
			if err != nil {
				emitAssistantError(ui, err.Error())
				return true, false
			}
			agentOverrides.ApplyToMetadata(baseMetadata)
			emitSystem(ui, message)
			return true, false
		}
		if err := agentOverrides.Set(key, value); err == nil {
			agentOverrides.ApplyToMetadata(baseMetadata)
			emitSystem(ui, agentOverrides.Summary())
			return true, false
		} else if !strings.Contains(err.Error(), "unknown agent setting") {
			emitAssistantError(ui, err.Error())
			return true, false
		}
		handled, err := setChatMetadataOverride(baseMetadata, key, value)
		if err != nil {
			emitAssistantError(ui, err.Error())
			return true, false
		}
		if !handled {
			emitAssistantError(ui, fmt.Sprintf("unknown setting %q", key))
			return true, false
		}
		emitSystem(ui, chatRuntimeSettingsSummary(baseMetadata, agentOverrides))
		return true, false
	case "settings":
		emitSystem(ui, chatRuntimeSettingsSummary(baseMetadata, agentOverrides))
		return true, false
	default:
		emitSystem(ui, "unknown command. type /help")
		return true, false
	}
}

func parseRuntimeSetBody(body string) (string, string, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", "", false
	}
	parts := strings.Fields(body)
	if len(parts) < 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(strings.TrimPrefix(body, parts[0]))
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
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
	fmt.Println("  /settings          show active chat/model/agent overrides")
	fmt.Println("  /agent             show active execution-agent override")
	fmt.Println("  /agent <name>      switch agent override: omnidex|cursor|codex")
	fmt.Println("  /agent reset       clear agent override and use core defaults")
	fmt.Println("  /model <name>      set model for the active agent override")
	fmt.Println("  /set <key> <value> set an active agent or model key such as codex_reasoning_effort, codex_sandbox, or model_plan")
	fmt.Println("  /exit              quit interactive mode")
	fmt.Println("  progress note      live stage/event updates are shown by default (disable with --progress=false)")
	fmt.Println("  phase note         stage lines include phase=planning|execution|review")
	fmt.Println("  queue note         while assistant is running, type TAB + message + Enter to queue a follow-up for the next turn")
	fmt.Println("  routing note       every non-slash message is sent unchanged to the core semantic pipeline")
	fmt.Println("  host operations    use explicit top-level commands: browser-scan, screen-read, audio-notes, media-index, media-search")
}

func printInteractiveInputHelp() {
	fmt.Println("feedback mode commands:")
	fmt.Println("  /interrupt <text>  inject context into the active job")
	fmt.Println("  /replan <text>     restart the job from plan with new context")
	fmt.Println("  /cancel <reason>   stop the active job")
	fmt.Println("  note               during a running turn, type these slash commands directly; TAB + text queues a follow-up")
	fmt.Println("  /exit              quit interactive mode")
}

func cloneMetadata(metadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}
