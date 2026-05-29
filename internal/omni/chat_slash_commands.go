package omni

import "strings"

type chatSlashKind int

const (
	chatSlashNone chatSlashKind = iota
	chatSlashResearch
	chatSlashSearch
	chatSlashThoughts
	chatSlashManage
	chatSlashMicro
	chatSlashTurn
	chatSlashUsageError
)

// turnRouteOptions steers handleTurn when the user invoked a slash command.
type turnRouteOptions struct {
	Objective       string
	ForceExecution  bool
	ThinkOnly       bool
	TaskMode        TaskMode
	ExecutionPrefix string
	PilotTrigger    string
	ReasonCode      string
}

type chatSlashCommand struct {
	Kind chatSlashKind
	Args string
	Turn turnRouteOptions
}

func parseChatSlashCommand(input string) (chatSlashCommand, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return chatSlashCommand{}, false
	}
	lower := strings.ToLower(trimmed)
	switch {
	case lower == "/exit", lower == "/quit", lower == "exit", lower == "quit":
		return chatSlashCommand{}, false
	case lower == "/help", lower == "/status", lower == "/history", lower == "/clear", lower == "/mode":
		return chatSlashCommand{}, false
	case strings.HasPrefix(lower, "/thoughts"):
		args := strings.TrimSpace(trimmed[len("/thoughts"):])
		return chatSlashCommand{Kind: chatSlashThoughts, Args: args}, true
	}

	name, args := splitChatSlashCommand(trimmed)
	if name == "" {
		return chatSlashCommand{}, false
	}
	switch name {
	case "/research":
		if args == "" {
			return chatSlashCommand{Kind: chatSlashUsageError, Args: "usage: /research <query>"}, true
		}
		return chatSlashCommand{Kind: chatSlashResearch, Args: args}, true
	case "/search":
		if args == "" {
			return chatSlashCommand{Kind: chatSlashUsageError, Args: "usage: /search <query>"}, true
		}
		return chatSlashCommand{Kind: chatSlashSearch, Args: args}, true
	case "/build":
		if args == "" {
			return chatSlashCommand{Kind: chatSlashUsageError, Args: "usage: /build <objective>"}, true
		}
		return chatSlashCommand{
			Kind: chatSlashTurn,
			Turn: turnRouteOptions{
				Objective:       args,
				ForceExecution:  true,
				TaskMode:        taskModeForBuildObjective(args),
				ExecutionPrefix: buildSlashExecutionPrefix,
				PilotTrigger:    "slash_build",
				ReasonCode:      "slash_build",
			},
		}, true
	case "/plan":
		if args == "" {
			return chatSlashCommand{Kind: chatSlashUsageError, Args: "usage: /plan <objective>"}, true
		}
		return chatSlashCommand{
			Kind: chatSlashTurn,
			Turn: turnRouteOptions{
				Objective:       args,
				ForceExecution:  true,
				TaskMode:        TaskModeInspectOnly,
				ExecutionPrefix: planSlashExecutionPrefix,
				PilotTrigger:    "slash_plan",
				ReasonCode:      "slash_plan",
			},
		}, true
	case "/think":
		if args == "" {
			return chatSlashCommand{Kind: chatSlashUsageError, Args: "usage: /think <question>"}, true
		}
		return chatSlashCommand{
			Kind: chatSlashTurn,
			Turn: turnRouteOptions{
				Objective:    args,
				ThinkOnly:    true,
				PilotTrigger: "slash_think",
				ReasonCode:   "slash_think",
			},
		}, true
	case "/manage", "/job":
		if args == "" {
			return chatSlashCommand{Kind: chatSlashUsageError, Args: "usage: /manage <objective>"}, true
		}
		return chatSlashCommand{Kind: chatSlashManage, Args: args}, true
	case "/micro", "/queue":
		if args == "" {
			return chatSlashCommand{Kind: chatSlashUsageError, Args: "usage: /micro <objective>"}, true
		}
		return chatSlashCommand{Kind: chatSlashMicro, Args: args}, true
	default:
		return chatSlashCommand{}, false
	}
}

func splitChatSlashCommand(input string) (name, args string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", ""
	}
	space := strings.Index(input, " ")
	if space < 0 {
		return strings.ToLower(input), ""
	}
	return strings.ToLower(input[:space]), strings.TrimSpace(input[space+1:])
}

const buildSlashExecutionPrefix = "Build mode: implement the following in the workspace. Execute file edits, installs, tests, and verification as needed.\n\n"

const planSlashExecutionPrefix = "Planning mode: produce architecture, milestones, and an implementation plan only. Use read-only inspection. Do not run mutating commands, file writes, installs, patches, or build repair.\n\n"

func taskModeForBuildObjective(objective string) TaskMode {
	survey := BuildWorksiteSurvey("")
	mode := inferTaskMode(objective, survey)
	switch mode {
	case TaskModeImplementFeature, TaskModeRepairProject, TaskModeBuildOrVerify, TaskModeCreateProject:
		return mode
	default:
		return TaskModeCreateProject
	}
}

func execPromptForTurnRoute(objective string, opts turnRouteOptions) string {
	core := strings.TrimSpace(objective)
	if core == "" {
		core = strings.TrimSpace(opts.Objective)
	}
	if prefix := strings.TrimSpace(opts.ExecutionPrefix); prefix != "" {
		return strings.TrimSpace(prefix + core)
	}
	return core
}
