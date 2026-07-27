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
	case "reasoning":
		if strings.TrimSpace(body) == "" {
			emitSystem(ui, "reasoning_level="+strings.TrimSpace(fmt.Sprint(baseMetadata["reasoning_level"])))
			return true, false
		}
		if _, err := setChatMetadataOverride(baseMetadata, "reasoning_level", body); err != nil {
			emitAssistantError(ui, err.Error())
			return true, false
		}
		emitSystem(ui, "reasoning_level="+strings.TrimSpace(fmt.Sprint(baseMetadata["reasoning_level"])))
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
	fmt.Println("  /reasoning <mode>  set thinking level: auto|fast|deep")
	fmt.Println("  /set <key> <value> set an agent/runtime key such as codex_reasoning_effort, codex_sandbox, model_plan, web, verify")
	fmt.Println("  /exit              quit interactive mode")
	fmt.Println("  progress note      live stage/event updates are shown by default (disable with --progress=false)")
	fmt.Println("  phase note         stage lines include phase=planning|execution|review")
	fmt.Println("  specialist note    each routed action is assigned to a specialist role (e.g., browser/media/shell)")
	fmt.Println("  confirm note       chat runs an AI interpretation pass for local automation actions and waits for `yes` before execution (disable with --confirm-actions=false)")
	fmt.Println("  queue note         while assistant is running, type TAB + message + Enter to queue a follow-up for the next turn")
	fmt.Println("  note               invasive local actions ask one-time permission (managed by `omni permissions ...`)")
	fmt.Println("  natural command    routing is capability-based (examples below are not exact trigger phrases)")
	fmt.Println("  natural command    'play the next episode ...' controls local VLC when --local-media is on")
	fmt.Println("  natural command    'show my browser tabs' scans local browser processes/tabs when --local-browser is on")
	fmt.Println("  natural command    'what's on my screen' captures and reads the local screen when --local-screen is on")
	fmt.Println("  natural command    'create a file named notes.txt' executes locally when --local-shell is on")
	fmt.Println("  natural command    'what is my ip' or 'what ports are open' runs local network inspection when --local-shell is on")
	fmt.Println("  local-shell note   file-edit commands include git change summaries/diff snippets when inside a git repo")
	fmt.Println("  natural command    'determine my location' / 'am I on VPN' / 'show network tools catalog' runs advanced network intelligence when --local-shell is on")
	fmt.Println("  natural command    'take notes during this call' starts local audio notes when --local-audio is on")
	fmt.Println("  profile note       start with --profile architect for stricter plan/verify/watch defaults")
}

func printInteractiveInputHelp() {
	fmt.Println("feedback mode commands:")
	fmt.Println("  /interrupt <text>  inject context into the active job")
	fmt.Println("  /replan <text>     restart the job from plan with new context")
	fmt.Println("  /cancel [reason]   stop the active job")
	fmt.Println("  note               during a running turn, type these slash commands directly; TAB + text queues a follow-up")
	fmt.Println("  /exit              quit interactive mode")
}

func formatLocalAutomationResponse(sourceLine, response string) string {
	text := strings.TrimSpace(response)
	if text == "" {
		return text
	}
	if hasSourceSection(text) {
		return text
	}
	sourceLine = strings.TrimSpace(sourceLine)
	if sourceLine == "" {
		return text
	}
	return text + "\n\nSources:\n- " + sourceLine
}

func hasSourceSection(text string) bool {
	for _, line := range strings.Split(strings.ToLower(text), "\n") {
		clean := strings.TrimSpace(line)
		if clean == "source:" || clean == "sources:" || strings.HasPrefix(clean, "source:") || strings.HasPrefix(clean, "sources:") {
			return true
		}
	}
	return false
}

func cloneMetadata(metadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}
