package main

import (
	"regexp"
	"strings"
	"time"
)

const localShellCommandTimeout = 20 * time.Second
const localShellSuggestionTTL = 30 * time.Minute
const localShellOutputMaxChars = 1400
const localShellSudoAuthTimeout = 90 * time.Second

var shellBacktickPattern = regexp.MustCompile("`([^`]+)`")
var shellCreateFilePattern = regexp.MustCompile(`(?i)\b(?:create|make|touch)\s+(?:me\s+)?(?:a\s+)?(?:new\s+)?file(?:\s+named|\s+called)?\s+([^\s]+)`)
var shellCreateFileAltPattern = regexp.MustCompile(`(?i)\b(?:create|make)\s+(?:an?\s+|the\s+)?([^\s]+)\s+file\b`)
var shellCreateFileNestedPattern = regexp.MustCompile(`(?i)\b(?:create|make)\s+(?:me\s+)?(?:a\s+)?(?:new\s+)?file(?:\s+named|\s+called)?\s+([^\s]+)\s+(?:and\s+)?(?:name|call)\s+it\s+([^\s]+)`)
var shellTypedFilePattern = regexp.MustCompile(`(?i)\b(?:create|make|touch|write)\s+(?:me\s+)?(?:a\s+|an\s+)?(?:new\s+)?([a-z0-9][a-z0-9._/\-]*)\s+(html|css|js|javascript|json|md|markdown|txt|text)\s+file\b`)
var shellFilenameTokenPattern = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9._/\-]*\.[a-z0-9]{1,16}\b`)
var shellRenamePattern = regexp.MustCompile(`(?i)\brename\s+(?:the\s+|that\s+)?(?:file\s+)?([^\s]+)\s+(?:to|as)\s+([^\s]+)`)
var shellRenameTestPattern = regexp.MustCompile(`(?i)\brename\s+(?:the\s+|that\s+)?test\s+file\s+(?:to|as)\s+([^\s]+)`)
var shellUnsafePattern = regexp.MustCompile(`(?i)\b(rm|mkfs|dd|shutdown|reboot|halt|poweroff|init|killall|kill|git\s+reset\s+--hard|truncate|drop|format)\b`)

var allowedLocalShellCommands = map[string]struct{}{
	"awk":                          {},
	"bash":                         {},
	"cat":                          {},
	"cp":                           {},
	"curl":                         {},
	"date":                         {},
	"df":                           {},
	"du":                           {},
	"docker":                       {},
	"docker-compose":               {},
	"echo":                         {},
	"find":                         {},
	"git":                          {},
	"go":                           {},
	"head":                         {},
	"hostname":                     {},
	"id":                           {},
	"ifconfig":                     {},
	"ip":                           {},
	"ipconfig":                     {},
	"jq":                           {},
	"ls":                           {},
	"lsof":                         {},
	"make":                         {},
	"mkdir":                        {},
	"mv":                           {},
	"netstat":                      {},
	"node":                         {},
	"nslookup":                     {},
	"npm":                          {},
	"ping":                         {},
	"pnpm":                         {},
	"podman":                       {},
	"python":                       {},
	"python3":                      {},
	"pwd":                          {},
	"pip":                          {},
	"pip3":                         {},
	"ps":                           {},
	"pytest":                       {},
	"rg":                           {},
	"sed":                          {},
	"sh":                           {},
	"ss":                           {},
	"stat":                         {},
	"sudo":                         {},
	"tail":                         {},
	"top":                          {},
	"tee":                          {},
	"traceroute":                   {},
	"touch":                        {},
	"uname":                        {},
	"uptime":                       {},
	"wget":                         {},
	"wc":                           {},
	"whois":                        {},
	"whoami":                       {},
	"nmap":                         {},
	"dig":                          {},
	"host":                         {},
	"nmcli":                        {},
	"wg":                           {},
	"wg-quick":                     {},
	"openvpn":                      {},
	"pgrep":                        {},
	"resolvectl":                   {},
	"mtr":                          {},
	"yarn":                         {},
	"basename":                     {},
	"dirname":                      {},
	"realpath":                     {},
	"./scripts/setup-host-deps.sh": {},
	"scripts/setup-host-deps.sh":   {},
}

var networkInterfaceNamePattern = regexp.MustCompile(`(?i)\b(tun|tap|wg|ppp|utun|vpn)[a-z0-9._-]*\b`)
var ifconfigInterfacePattern = regexp.MustCompile(`^([a-zA-Z0-9._-]+):`)

type localShellState struct {
	LastSuggestedCommand string
	LastSuggestedAt      time.Time
}

type localShellIntent struct {
	Action  string
	Command string
	Source  string
	Target  string
}

type repoWorkingTreeSnapshot struct {
	Available bool
	Files     map[string]repoWorkingFileState
}

type repoWorkingFileState struct {
	Status string
	Hash   string
}

type repoDiffStat struct {
	Added   int
	Removed int
	Known   bool
}

func tryHandleLocalShellCommand(input string, state *localShellState) (bool, string) {
	intent, ok := parseLocalShellIntent(input, state)
	if !ok {
		return false, ""
	}
	if err := ensureLocalPermission(permissionKeyShellExec, "Allow executing local shell actions for file/command requests in the current directory."); err != nil {
		return true, "Local shell action blocked: " + err.Error()
	}

	switch intent.Action {
	case "create_file":
		outcome, err := createLocalFile(intent.Target)
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "rename_file":
		outcome, err := renameLocalFile(intent.Source, intent.Target)
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_system_summary":
		return true, showSystemSummary()
	case "show_running_processes":
		outcome, err := showRunningProcesses()
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_ip":
		outcome, err := showNetworkIP()
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_open_ports":
		outcome, err := showOpenPorts(false)
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_open_ports_detailed":
		outcome, err := showOpenPorts(true)
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_network_profile":
		outcome, err := showNetworkProfile()
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_network_location":
		outcome, err := showNetworkLocation()
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_vpn_status":
		outcome, err := showVPNStatus()
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_network_tools_catalog":
		outcome, err := showNetworkToolsCatalog()
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "show_repo_walkthrough":
		outcome, err := showRepositoryWalkthrough()
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "install_network_tools":
		command := inferNetworkToolsInstallCommand()
		if strings.TrimSpace(command) == "" {
			return true, "Local shell action failed: no network-tools installer was found for this workspace."
		}
		outcome, err := runLocalSafeCommand(command)
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	case "run_command":
		outcome, err := runLocalSafeCommand(intent.Command)
		if err != nil {
			return true, "Local shell action failed: " + err.Error()
		}
		return true, outcome
	default:
		return true, "Local shell action failed: unsupported action"
	}
}
