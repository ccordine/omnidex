package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

func parseLocalShellIntent(input string, state *localShellState) (localShellIntent, bool) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return localShellIntent{}, false
	}
	lower := strings.ToLower(clean)

	if command, ok := parseDoItIntent(lower, state); ok {
		return localShellIntent{Action: "run_command", Command: command}, true
	}
	if source, target, ok := parseRenameIntent(clean, lower); ok {
		return localShellIntent{
			Action: "rename_file",
			Source: source,
			Target: target,
		}, true
	}
	if target, ok := parseCreateFileIntent(clean, lower); ok {
		return localShellIntent{
			Action: "create_file",
			Target: target,
		}, true
	}
	if intent, ok := parseSystemNetworkIntent(clean, lower); ok {
		return intent, true
	}
	if parseRepoWalkthroughIntent(lower) {
		return localShellIntent{Action: "show_repo_walkthrough"}, true
	}
	if command, ok := parseRepositoryWorkflowIntent(lower); ok {
		return localShellIntent{
			Action:  "run_command",
			Command: command,
		}, true
	}
	if command, ok := parseExplicitRunCommand(clean, lower); ok {
		return localShellIntent{
			Action:  "run_command",
			Command: command,
		}, true
	}
	if intent, ok := inferLocalShellIntentByCapabilities(clean, lower); ok {
		return intent, true
	}

	return localShellIntent{}, false
}

func inferLocalShellIntentByCapabilities(clean, lower string) (localShellIntent, bool) {
	tokens := tokenizeForCapabilityMatch(lower)
	if len(tokens) == 0 {
		return localShellIntent{}, false
	}

	if hasAnyCapabilityToken(tokens, "ip", "address", "wan", "lan", "public", "external") {
		return localShellIntent{Action: "show_ip"}, true
	}
	if hasAnyCapabilityToken(tokens, "port", "socket", "listen", "listening") {
		if hasAnyCapabilityToken(tokens, "process", "pid", "program", "service", "detail", "detailed", "full") {
			return localShellIntent{Action: "show_open_ports_detailed"}, true
		}
		return localShellIntent{Action: "show_open_ports"}, true
	}
	if hasAnyCapabilityToken(tokens, "process", "pid", "program", "task", "service") &&
		hasAnyCapabilityToken(tokens, "run", "runn", "current", "currently", "active", "top", "show", "list", "inspect", "check", "what", "which") {
		return localShellIntent{Action: "show_running_processes"}, true
	}
	if hasAnyCapabilityToken(tokens, "vpn", "wireguard", "openvpn", "tunnel") {
		return localShellIntent{Action: "show_vpn_status"}, true
	}
	if hasAnyCapabilityToken(tokens, "location", "geolocate", "geolocation", "region", "city", "country") {
		return localShellIntent{Action: "show_network_location"}, true
	}
	if hasAnyCapabilityToken(tokens, "network", "connection") && hasAnyCapabilityToken(tokens, "profile", "status", "inspect") {
		return localShellIntent{Action: "show_network_profile"}, true
	}
	if hasAnyCapabilityToken(tokens, "tool", "tools", "catalog", "discover", "website", "app") &&
		hasAnyCapabilityToken(tokens, "network", "net", "tool", "tools") {
		return localShellIntent{Action: "show_network_tools_catalog"}, true
	}
	if hasAnyCapabilityToken(tokens, "install", "setup", "set", "add") &&
		hasAnyCapabilityToken(tokens, "network", "net", "tool", "tools") {
		return localShellIntent{Action: "install_network_tools"}, true
	}
	if hasAnyCapabilityToken(tokens, "username", "user", "name", "identity", "whoami") {
		if hasAnyCapabilityToken(tokens, "who", "id", "uid") {
			return localShellIntent{Action: "run_command", Command: "id"}, true
		}
		return localShellIntent{Action: "run_command", Command: "id -un"}, true
	}
	if hasAnyCapabilityToken(tokens, "time", "date", "clock", "timezone") {
		return localShellIntent{Action: "run_command", Command: "date"}, true
	}
	if hasAnyCapabilityToken(tokens, "kernel", "os", "operating", "system") &&
		hasAnyCapabilityToken(tokens, "what", "which", "show", "check", "inspect") {
		return localShellIntent{Action: "run_command", Command: "uname -a"}, true
	}
	if hasAnyCapabilityToken(tokens, "system", "machine", "host", "environment") &&
		hasAnyCapabilityToken(tokens, "summary", "info", "details", "about", "inspect") {
		return localShellIntent{Action: "show_system_summary"}, true
	}
	if hasAnyCapabilityToken(tokens, "create", "make", "touch") &&
		hasAnyCapabilityToken(tokens, "file", "document", "doc", "note", "notes") {
		target := "test"
		if parsed, ok := parseCreateFileIntent(clean, lower); ok && strings.TrimSpace(parsed) != "" {
			target = parsed
		}
		return localShellIntent{Action: "create_file", Target: target}, true
	}
	if hasAnyCapabilityToken(tokens, "git", "repo", "repository", "project") &&
		hasAnyCapabilityToken(tokens, "change", "changes", "changed", "diff", "status", "resume", "left", "chronological", "recent") {
		return localShellIntent{Action: "show_repo_walkthrough"}, true
	}
	if hasAnyCapabilityToken(tokens, "rename", "move") &&
		hasAnyCapabilityToken(tokens, "file", "document", "doc", "note", "notes") {
		if source, target, ok := parseRenameIntent(clean, lower); ok {
			return localShellIntent{Action: "rename_file", Source: source, Target: target}, true
		}
	}
	if hasAnyCapabilityToken(tokens, "directory", "folder", "path", "cwd", "workspace", "working") {
		return localShellIntent{Action: "run_command", Command: "pwd"}, true
	}

	return localShellIntent{}, false
}

func hasAnyCapabilityToken(tokens map[string]struct{}, terms ...string) bool {
	for _, term := range terms {
		value := normalizeCapabilityToken(term)
		if value == "" {
			continue
		}
		if _, ok := tokens[value]; ok {
			return true
		}
	}
	return false
}

func parseSystemNetworkIntent(_ string, lower string) (localShellIntent, bool) {
	ipPhrases := []string{
		"what is my ip",
		"what's my ip",
		"whats my ip",
		"my ip address",
		"show my ip",
		"public ip",
		"external ip",
		"wan ip",
		"local ip",
		"lan ip",
	}
	if containsAnyPhrase(lower, ipPhrases) {
		return localShellIntent{Action: "show_ip"}, true
	}

	portPhrases := []string{
		"open ports",
		"ports are open",
		"open port",
		"listening ports",
		"ports listening",
		"ports in use",
		"show open ports",
		"list open ports",
		"what ports are open",
		"which ports are open",
	}
	if containsAnyPhrase(lower, portPhrases) {
		if containsAnyPhrase(lower, []string{"process", "pid", "program", "service", "detailed", "full"}) {
			return localShellIntent{Action: "show_open_ports_detailed"}, true
		}
		return localShellIntent{Action: "show_open_ports"}, true
	}

	if containsAnyPhrase(lower, []string{
		"system info",
		"system details",
		"host info",
		"host details",
		"environment info",
		"machine info",
		"about my system",
	}) {
		return localShellIntent{Action: "show_system_summary"}, true
	}
	if containsAnyPhrase(lower, []string{
		"what is running",
		"what's running",
		"whats running",
		"currently running",
		"running right now",
		"what processes are running",
		"which processes are running",
		"list running processes",
		"running processes",
		"active processes",
		"top processes",
	}) {
		return localShellIntent{Action: "show_running_processes"}, true
	}

	if containsAnyPhrase(lower, []string{"who am i", "current user"}) {
		return localShellIntent{Action: "run_command", Command: "id"}, true
	}
	if containsAnyPhrase(lower, []string{
		"what is my name",
		"what's my name",
		"whats my name",
		"my username",
		"what is my username",
		"what's my username",
	}) {
		return localShellIntent{Action: "run_command", Command: "id -un"}, true
	}
	if containsAnyPhrase(lower, []string{"what time is it", "current time", "what date is it", "current date"}) {
		return localShellIntent{Action: "run_command", Command: "date"}, true
	}
	if containsAnyPhrase(lower, []string{
		"what directory are we in",
		"which directory are we in",
		"what folder are we in",
		"which folder are we in",
		"current directory",
		"working directory",
	}) {
		return localShellIntent{Action: "run_command", Command: "pwd"}, true
	}
	if containsAnyPhrase(lower, []string{"what os", "which os", "operating system", "kernel version"}) {
		return localShellIntent{Action: "run_command", Command: "uname -a"}, true
	}
	if containsAnyPhrase(lower, []string{
		"network profile",
		"connection profile",
		"network status",
		"inspect network",
	}) {
		return localShellIntent{Action: "show_network_profile"}, true
	}
	if containsAnyPhrase(lower, []string{
		"where am i",
		"what is my location",
		"what's my location",
		"determine my location",
		"location based on my connection",
		"geo locate me",
		"geolocate me",
	}) {
		return localShellIntent{Action: "show_network_location"}, true
	}
	if containsAnyPhrase(lower, []string{
		"am i on vpn",
		"is vpn active",
		"is a vpn running",
		"check vpn",
		"vpn status",
		"running vpn",
		"running vpns",
	}) {
		return localShellIntent{Action: "show_vpn_status"}, true
	}
	if containsAnyPhrase(lower, []string{
		"install network tools",
		"install net tools",
		"add network tools",
		"setup network tools",
		"set up network tools",
		"install networking tools",
	}) {
		return localShellIntent{Action: "install_network_tools"}, true
	}
	if containsAnyPhrase(lower, []string{
		"network tools",
		"net tools",
		"tool catalog",
		"discover tools",
		"what tools can you use",
		"what websites can you use",
		"what apps can you use",
		"what tools are available",
		"discover network tools",
	}) {
		return localShellIntent{Action: "show_network_tools_catalog"}, true
	}
	return localShellIntent{}, false
}

func parseRepoWalkthroughIntent(lower string) bool {
	phrases := []string{
		"walk me through current changes",
		"walk through current changes",
		"walk me through changes",
		"walk through changes",
		"where did i leave off",
		"where i left off",
		"pick back up",
		"resume this project",
		"show repo changes",
		"show repository changes",
		"show git changes",
		"summarize repo changes",
		"summarize repository changes",
		"what changed in this repo",
		"what changed in this repository",
		"what files changed recently",
		"chronological order of changed files",
		"changed files chronological",
	}
	if containsAnyPhrase(lower, phrases) {
		return true
	}
	if containsAnyPhrase(lower, []string{"git status", "repo status", "repository status"}) &&
		containsAnyPhrase(lower, []string{"walk", "explain", "summarize", "chronological", "recent"}) {
		return true
	}
	return false
}

func parseDoItIntent(lower string, state *localShellState) (string, bool) {
	if state == nil {
		return "", false
	}
	if strings.TrimSpace(state.LastSuggestedCommand) == "" {
		return "", false
	}
	if !state.LastSuggestedAt.IsZero() && time.Since(state.LastSuggestedAt) > localShellSuggestionTTL {
		return "", false
	}
	doItPhrases := []string{
		"do it for me",
		"please do it",
		"do that for me",
		"go ahead and do it",
		"go ahead",
		"run it",
	}
	if !containsAnyPhrase(lower, doItPhrases) {
		return "", false
	}
	return state.LastSuggestedCommand, true
}

func parseCreateFileIntent(clean, lower string) (string, bool) {
	commonTestFilePhrases := []string{
		"make me a test file",
		"make a test file",
		"create a test file",
		"create me a test file",
		"touch test file",
		"create test file",
	}
	if containsAnyPhrase(lower, commonTestFilePhrases) {
		return "test", true
	}

	if matches := shellCreateFileNestedPattern.FindStringSubmatch(clean); len(matches) == 3 {
		container := cleanShellToken(matches[1])
		name := cleanShellToken(matches[2])
		if container != "" && name != "" {
			if strings.ContainsAny(container, `\/`) || filepath.Ext(container) != "" {
				return name, true
			}
			return filepath.Join(container, name), true
		}
	}

	if matches := shellCreateFilePattern.FindStringSubmatch(clean); len(matches) == 2 {
		name := cleanShellToken(matches[1])
		if name != "" {
			return name, true
		}
	}
	if matches := shellTypedFilePattern.FindStringSubmatch(clean); len(matches) == 3 {
		name := cleanShellToken(matches[1])
		ext := normalizeTypedFileExtension(matches[2])
		if name != "" && ext != "" {
			if strings.Contains(filepath.Base(name), ".") {
				return filepath.Clean(name), true
			}
			return filepath.Clean(name + "." + ext), true
		}
	}
	if matches := shellCreateFileAltPattern.FindStringSubmatch(clean); len(matches) == 2 {
		name := cleanShellToken(matches[1])
		if name != "" {
			return name, true
		}
	}
	if containsAnyPhrase(lower, []string{"create", "make", "touch"}) {
		matches := shellFilenameTokenPattern.FindAllString(clean, -1)
		for i := len(matches) - 1; i >= 0; i-- {
			name := cleanShellToken(matches[i])
			if name == "" {
				continue
			}
			return name, true
		}
	}

	if strings.Contains(lower, "create file") || strings.Contains(lower, "make file") {
		return "test", true
	}
	return "", false
}

func normalizeTypedFileExtension(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "html":
		return "html"
	case "css":
		return "css"
	case "js", "javascript":
		return "js"
	case "json":
		return "json"
	case "md", "markdown":
		return "md"
	case "txt", "text":
		return "txt"
	default:
		return ""
	}
}

func parseRenameIntent(clean, lower string) (string, string, bool) {
	if matches := shellRenamePattern.FindStringSubmatch(clean); len(matches) == 3 {
		source := cleanShellToken(matches[1])
		target := cleanShellToken(matches[2])
		if source != "" && target != "" {
			return source, target, true
		}
	}
	if matches := shellRenameTestPattern.FindStringSubmatch(clean); len(matches) == 2 {
		target := cleanShellToken(matches[1])
		if target != "" {
			return "test", target, true
		}
	}
	if strings.Contains(lower, "rename that file") || strings.Contains(lower, "rename file") {
		return "", "", false
	}
	return "", "", false
}

func parseExplicitRunCommand(clean, lower string) (string, bool) {
	if matches := shellBacktickPattern.FindStringSubmatch(clean); len(matches) == 2 {
		if containsAnyPhrase(lower, []string{"run", "execute", "do it", "command"}) {
			return strings.TrimSpace(matches[1]), true
		}
	}

	markers := []string{
		"please run ",
		"can you run ",
		"run ",
		"please execute ",
		"can you execute ",
		"execute ",
	}
	for _, marker := range markers {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		start := idx + len(marker)
		command := strings.TrimSpace(clean[start:])
		command = strings.Trim(command, " \t\r\n\"'`.,!?;:()[]{}")
		if command != "" {
			return command, true
		}
	}

	tokens := strings.Fields(clean)
	if len(tokens) > 0 {
		name := strings.ToLower(strings.TrimSpace(tokens[0]))
		if _, ok := allowedLocalShellCommands[name]; ok {
			return strings.Join(tokens, " "), true
		}
	}

	return "", false
}

func parseRepositoryWorkflowIntent(lower string) (string, bool) {
	if containsAnyPhrase(lower, []string{
		"install requirements",
		"check requirements",
		"install dependencies",
		"install deps",
		"set up dependencies",
		"setup dependencies",
	}) {
		if command := inferDependencyInstallCommand(); command != "" {
			return command, true
		}
	}

	if containsAnyPhrase(lower, []string{
		"run tests",
		"run test",
		"test it",
		"test this",
		"execute tests",
		"verify it works",
		"make sure it works",
	}) {
		if command := inferRepositoryTestCommand(); command != "" {
			return command, true
		}
	}

	if containsAnyPhrase(lower, []string{
		"spin up docker",
		"start docker environment",
		"start docker test environment",
		"bring up docker",
		"docker test environment",
		"start test environment",
		"run docker compose",
	}) {
		if command := inferDockerUpCommand(); command != "" {
			return command, true
		}
	}

	if containsAnyPhrase(lower, []string{
		"stop docker environment",
		"stop test environment",
		"bring docker down",
		"shutdown docker environment",
	}) {
		if fileExists("docker-compose.yml") || fileExists("docker-compose.yaml") {
			return "docker compose down", true
		}
	}

	return "", false
}

func inferDependencyInstallCommand() string {
	if fileExists("./scripts/setup-host-deps.sh") {
		return "./scripts/setup-host-deps.sh --profile all -y"
	}
	if fileExists("scripts/setup-host-deps.sh") {
		return "scripts/setup-host-deps.sh --profile all -y"
	}
	if fileExists("package.json") {
		return "npm install"
	}
	if fileExists("go.mod") {
		return "go mod tidy"
	}
	if fileExists("requirements.txt") {
		return "pip install -r requirements.txt"
	}
	if fileExists("pyproject.toml") || fileExists("setup.py") {
		return "pip install -e ."
	}
	return ""
}

func inferRepositoryTestCommand() string {
	if fileExists("go.mod") {
		return "go test ./..."
	}
	if fileExists("package.json") {
		return "npm test"
	}
	if fileExists("Makefile") || fileExists("makefile") {
		return "make test"
	}
	if fileExists("pyproject.toml") || fileExists("requirements.txt") || fileExists("setup.py") {
		return "pytest -q"
	}
	return ""
}

func inferDockerUpCommand() string {
	if fileExists("docker-compose.yml") || fileExists("docker-compose.yaml") {
		return "docker compose up --build -d"
	}
	if fileExists("Dockerfile") {
		return "docker build -t local-test ."
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func cleanShellToken(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, " \t\r\n\"'`.,!?;:()[]{}")
	return value
}

func createLocalFile(target string) (string, error) {
	target = cleanShellToken(target)
	if target == "" {
		target = "test"
	}
	if err := validateRelativePath(target); err != nil {
		return "", err
	}
	parentDir := filepath.Clean(filepath.Dir(target))
	createdParent := parentDir != "." && parentDir != ""
	existedBefore := false
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		existedBefore = true
	}
	before := captureRepoWorkingTreeSnapshot()
	lines := make([]string, 0, 4)
	if createdParent {
		if _, err := runLocalCommand([]string{"mkdir", "-p", parentDir}, localShellCommandTimeout); err != nil {
			return "", err
		}
		lines = append(lines, "Executed: mkdir -p "+parentDir)
	}
	if _, err := runLocalCommand([]string{"touch", target}, localShellCommandTimeout); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(target)
	lines = append(lines, "Executed: touch "+target)
	if existedBefore {
		lines = append(lines, "File already exists: "+abs)
	} else {
		lines = append(lines, "Created file: "+abs)
	}
	if verifyOutput, err := runLocalCommand([]string{"ls", "-l", target}, localShellCommandTimeout); err == nil {
		lines = append(lines, "Executed: ls -l "+target)
		if strings.TrimSpace(verifyOutput) != "" {
			lines = append(lines, "Verification Output:")
			lines = append(lines, strings.TrimSpace(verifyOutput))
		}
	}
	result := strings.Join(lines, "\n")
	if changeSummary := buildRepoChangeSummary(before); strings.TrimSpace(changeSummary) != "" {
		result = strings.TrimSpace(result + "\n\n" + changeSummary)
	}
	return result, nil
}

func renameLocalFile(source, target string) (string, error) {
	source = cleanShellToken(source)
	target = cleanShellToken(target)
	if source == "" {
		return "", errors.New("source file name is required")
	}
	if target == "" {
		return "", errors.New("target file name is required")
	}
	if err := validateRelativePath(source); err != nil {
		return "", err
	}
	if err := validateRelativePath(target); err != nil {
		return "", err
	}
	if _, err := os.Stat(source); err != nil {
		return "", fmt.Errorf("source file not found: %s", source)
	}
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("target already exists: %s", target)
	}
	before := captureRepoWorkingTreeSnapshot()
	if _, err := runLocalCommand([]string{"mv", source, target}, localShellCommandTimeout); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(target)
	result := fmt.Sprintf("Executed: mv %s %s\nRenamed file to: %s", source, target, abs)
	if changeSummary := buildRepoChangeSummary(before); strings.TrimSpace(changeSummary) != "" {
		result = strings.TrimSpace(result + "\n\n" + changeSummary)
	}
	return result, nil
}

func runLocalSafeCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command is required")
	}
	if strings.ContainsAny(command, "|&;<>") {
		return "", errors.New("shell operators are not allowed in local chat command mode")
	}
	if shellUnsafePattern.MatchString(command) {
		return "", errors.New("command blocked by safety policy")
	}
	args := strings.Fields(command)
	if len(args) == 0 {
		return "", errors.New("command is required")
	}
	useSudo := false
	if strings.EqualFold(strings.TrimSpace(args[0]), "sudo") {
		useSudo = true
		args = args[1:]
		if len(args) == 0 {
			return "", errors.New("missing command after sudo")
		}
		if strings.HasPrefix(args[0], "-") {
			return "", errors.New("sudo flags are not allowed in local chat mode")
		}
	}
	name := strings.ToLower(strings.TrimSpace(args[0]))
	if _, ok := allowedLocalShellCommands[name]; !ok {
		return "", fmt.Errorf("command not allowed in local chat mode: %s", name)
	}
	before := captureRepoWorkingTreeSnapshot()

	var (
		output       string
		err          error
		executedText string
	)
	if useSudo {
		if err := ensureLocalPermission(permissionKeyShellSudo, "Allow running local shell commands with sudo when elevated access is required."); err != nil {
			return "", err
		}
		output, err = runLocalSudoCommand(args, localShellCommandTimeout)
		executedText = "sudo " + strings.Join(args, " ")
	} else {
		output, err = runLocalCommand(args, localShellCommandTimeout)
		executedText = strings.Join(args, " ")
		if err != nil && shouldRetryWithSudo(args, err) {
			reason := buildSudoRetryReason(args, err)
			if permErr := ensureLocalPermission(permissionKeyShellSudo, reason); permErr != nil {
				return "", fmt.Errorf("command failed without sudo (%s); sudo retry blocked: %w", sanitizeSudoReasonText(err.Error()), permErr)
			}
			output, err = runLocalSudoCommand(args, localShellCommandTimeout)
			executedText = "sudo " + strings.Join(args, " ")
		}
	}
	if err != nil {
		return "", err
	}
	lines := []string{"Executed: " + executedText}
	if strings.TrimSpace(output) != "" {
		lines = append(lines, "Output:")
		lines = append(lines, output)
	}
	if changeSummary := buildRepoChangeSummary(before); strings.TrimSpace(changeSummary) != "" {
		lines = append(lines, "")
		lines = append(lines, changeSummary)
	}
	return strings.Join(lines, "\n"), nil
}

func shouldRetryWithSudo(args []string, err error) bool {
	if len(args) == 0 || err == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(args[0]))
	if name == "" || name == "sudo" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if lower == "" {
		return false
	}
	if !containsAnyPhrase(lower, []string{
		"permission denied",
		"operation not permitted",
		"eacces",
		"requires root",
		"must be root",
		"access denied",
	}) {
		return false
	}
	if containsAnyPhrase(lower, []string{
		"not allowed in local chat mode",
		"command not found",
		"no such file or directory",
		"timed out",
	}) {
		return false
	}
	return true
}

func buildSudoRetryReason(args []string, runErr error) string {
	cmdText := strings.TrimSpace(strings.Join(args, " "))
	if cmdText == "" {
		cmdText = "the requested command"
	}
	errText := "permission-related failure"
	if runErr != nil {
		errText = sanitizeSudoReasonText(runErr.Error())
	}
	return fmt.Sprintf("Allow retrying `%s` with sudo because it failed with: %s", cmdText, errText)
}

func sanitizeSudoReasonText(text string) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if clean == "" {
		return "permission-related failure"
	}
	const maxChars = 220
	if len(clean) > maxChars {
		return clean[:maxChars] + "...(truncated)"
	}
	return clean
}

func captureRepoWorkingTreeSnapshot() repoWorkingTreeSnapshot {
	snapshot := repoWorkingTreeSnapshot{
		Available: false,
		Files:     map[string]repoWorkingFileState{},
	}
	if !commandExists("git") {
		return snapshot
	}
	if _, err := runLocalCommand([]string{"git", "rev-parse", "--is-inside-work-tree"}, 4*time.Second); err != nil {
		return snapshot
	}

	raw, err := runLocalCommand([]string{"git", "status", "--porcelain=1", "--untracked-files=all"}, localShellCommandTimeout)
	if err != nil {
		return snapshot
	}
	snapshot.Available = true
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r\n")
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		path := parseGitPorcelainPath(line[3:])
		if path == "" {
			continue
		}
		snapshot.Files[path] = repoWorkingFileState{
			Status: status,
			Hash:   hashLocalFile(path),
		}
	}
	return snapshot
}

func parseGitPorcelainPath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, " -> "); idx >= 0 {
		path = strings.TrimSpace(path[idx+4:])
	}
	path = strings.Trim(path, "\"")
	return strings.TrimSpace(path)
}

func hashLocalFile(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func buildRepoChangeSummary(before repoWorkingTreeSnapshot) string {
	if !before.Available {
		return ""
	}
	after := captureRepoWorkingTreeSnapshot()
	if !after.Available {
		return ""
	}

	changed := collectRepoChangedPaths(before, after)
	if len(changed) == 0 {
		return ""
	}

	stats := collectRepoDiffStats()
	lines := []string{"Code changes:"}
	const maxFiles = 6
	for i, path := range changed {
		if i >= maxFiles {
			lines = append(lines, fmt.Sprintf("• ... %d more file(s) changed", len(changed)-maxFiles))
			break
		}
		_, hadBefore := before.Files[path]
		afterState, hasAfter := after.Files[path]

		kind := describeRepoChangeKind(hadBefore, hasAfter, afterState)
		statText := ""
		if stat, ok := stats[path]; ok && stat.Known {
			statText = fmt.Sprintf(" (+%d -%d)", stat.Added, stat.Removed)
		} else if hasAfter && strings.TrimSpace(afterState.Status) == "??" {
			statText = " (untracked)"
		}
		lines = append(lines, fmt.Sprintf("• %s %s%s", kind, path, statText))

		if hasAfter {
			if snippet := readRepoDiffSnippet(path, afterState.Status); snippet != "" {
				lines = append(lines, "```diff")
				lines = append(lines, snippet)
				lines = append(lines, "```")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func collectRepoChangedPaths(before, after repoWorkingTreeSnapshot) []string {
	all := map[string]struct{}{}
	for path := range before.Files {
		all[path] = struct{}{}
	}
	for path := range after.Files {
		all[path] = struct{}{}
	}

	changed := make([]string, 0, len(all))
	for path := range all {
		b, bok := before.Files[path]
		a, aok := after.Files[path]
		if !bok || !aok {
			changed = append(changed, path)
			continue
		}
		if strings.TrimSpace(b.Status) != strings.TrimSpace(a.Status) || strings.TrimSpace(b.Hash) != strings.TrimSpace(a.Hash) {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func describeRepoChangeKind(hadBefore bool, hasAfter bool, after repoWorkingFileState) string {
	if !hadBefore && hasAfter {
		if statusContainsRune(after.Status, 'A') || strings.TrimSpace(after.Status) == "??" {
			return "Added"
		}
		if statusContainsRune(after.Status, 'D') {
			return "Deleted"
		}
		if statusContainsRune(after.Status, 'R') {
			return "Renamed"
		}
		return "Edited"
	}
	if hadBefore && !hasAfter {
		return "Reverted"
	}
	if hasAfter {
		if statusContainsRune(after.Status, 'D') {
			return "Deleted"
		}
		if statusContainsRune(after.Status, 'R') {
			return "Renamed"
		}
		if statusContainsRune(after.Status, 'A') || strings.TrimSpace(after.Status) == "??" {
			return "Added"
		}
	}
	return "Edited"
}

func statusContainsRune(status string, target rune) bool {
	for _, ch := range strings.TrimSpace(status) {
		if ch == target {
			return true
		}
	}
	return false
}

func collectRepoDiffStats() map[string]repoDiffStat {
	stats := map[string]repoDiffStat{}
	for _, args := range [][]string{
		{"git", "diff", "--numstat", "--"},
		{"git", "diff", "--cached", "--numstat", "--"},
	} {
		raw, err := runLocalCommand(args, localShellCommandTimeout)
		if err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		for _, line := range strings.Split(raw, "\n") {
			path, stat, ok := parseRepoNumstatLine(line)
			if !ok {
				continue
			}
			prev, exists := stats[path]
			if !exists {
				stats[path] = stat
				continue
			}
			if prev.Known && stat.Known {
				prev.Added += stat.Added
				prev.Removed += stat.Removed
				prev.Known = true
			} else {
				prev.Known = false
			}
			stats[path] = prev
		}
	}
	return stats
}

func parseRepoNumstatLine(line string) (string, repoDiffStat, bool) {
	parts := strings.SplitN(strings.TrimSpace(line), "\t", 3)
	if len(parts) < 3 {
		return "", repoDiffStat{}, false
	}
	path := strings.TrimSpace(parts[2])
	if path == "" {
		return "", repoDiffStat{}, false
	}
	added, errAdded := strconv.Atoi(strings.TrimSpace(parts[0]))
	removed, errRemoved := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errAdded != nil || errRemoved != nil {
		return path, repoDiffStat{Known: false}, true
	}
	return path, repoDiffStat{
		Added:   added,
		Removed: removed,
		Known:   true,
	}, true
}

func readRepoDiffSnippet(path, status string) string {
	path = strings.TrimSpace(path)
	if path == "" || strings.TrimSpace(status) == "??" {
		return ""
	}
	for _, args := range [][]string{
		{"git", "--no-pager", "diff", "--", path},
		{"git", "--no-pager", "diff", "--cached", "--", path},
	} {
		raw, err := runLocalCommand(args, localShellCommandTimeout)
		if err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		return trimDiffSnippet(raw, 32, 900)
	}
	return ""
}

func trimDiffSnippet(value string, maxLines, maxChars int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = append(lines[:maxLines], "...(truncated)")
	}
	out := strings.Join(lines, "\n")
	if maxChars > 0 && len(out) > maxChars {
		out = out[:maxChars] + "...(truncated)"
	}
	return strings.TrimSpace(out)
}

type repoLookupResult struct {
	Root         string
	Reason       string
	Alternatives []string
}

type repoTimelineEntry struct {
	Path          string
	Status        string
	Kind          string
	Stats         repoDiffStat
	WorkingMTime  time.Time
	CommitTime    time.Time
	CommitHash    string
	CommitSubject string
	EffectiveTime time.Time
	TimeSource    string
}

type repoCommitEntry struct {
	Time    time.Time
	Hash    string
	Subject string
}

func showRepositoryWalkthrough() (string, error) {
	repo, err := locateNearbyRepository()
	if err != nil {
		return "", err
	}

	branch := "(unknown)"
	if value, err := gitCurrentBranch(repo.Root); err == nil && strings.TrimSpace(value) != "" {
		branch = strings.TrimSpace(value)
	}

	entries, err := collectRepoTimeline(repo.Root)
	if err != nil {
		return "", err
	}
	recentCommits := collectRecentRepoCommits(repo.Root, 6)

	lines := []string{
		"Repository walkthrough:",
		"selected_repo=" + repo.Root,
		"selection_reason=" + safeValue(repo.Reason, "unknown"),
		"branch=" + branch,
		fmt.Sprintf("changed_files=%d", len(entries)),
	}
	if len(repo.Alternatives) > 0 {
		lines = append(lines, "other_nearby_repos="+strings.Join(repo.Alternatives, ","))
	}

	if len(entries) == 0 {
		lines = append(lines, "working_tree=clean (no uncommitted file changes detected)")
	} else {
		lines = append(lines, "chronological_changes (most recent first):")
		const maxItems = 20
		for i, entry := range entries {
			if i >= maxItems {
				lines = append(lines, fmt.Sprintf("- ... %d more changed file(s)", len(entries)-maxItems))
				break
			}
			statText := ""
			if entry.Stats.Known {
				statText = fmt.Sprintf(" (+%d -%d)", entry.Stats.Added, entry.Stats.Removed)
			} else if strings.TrimSpace(entry.Status) == "??" {
				statText = " (untracked)"
			}
			lines = append(lines, fmt.Sprintf("- %s %s%s", safeValue(entry.Kind, "Edited"), entry.Path, statText))
			if !entry.EffectiveTime.IsZero() {
				lines = append(lines, "  last_changed="+entry.EffectiveTime.Format(time.RFC3339)+" source="+safeValue(entry.TimeSource, "unknown"))
			}
			if !entry.CommitTime.IsZero() && strings.TrimSpace(entry.CommitHash) != "" {
				lines = append(lines, fmt.Sprintf("  last_commit=%s %s %s", entry.CommitTime.Format(time.RFC3339), entry.CommitHash, strings.TrimSpace(entry.CommitSubject)))
			}
		}
	}

	if len(recentCommits) > 0 {
		lines = append(lines, "recent_commits:")
		for _, commit := range recentCommits {
			lines = append(lines, fmt.Sprintf("- %s %s %s", commit.Time.Format(time.RFC3339), commit.Hash, commit.Subject))
		}
	}

	return strings.Join(lines, "\n"), nil
}

func locateNearbyRepository() (repoLookupResult, error) {
	if !commandExists("git") {
		return repoLookupResult{}, errors.New("`git` is not available on this system")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return repoLookupResult{}, fmt.Errorf("unable to determine current directory: %w", err)
	}
	cwd, _ = filepath.Abs(cwd)

	if root, err := discoverGitRootFromDir(cwd); err == nil && strings.TrimSpace(root) != "" {
		return repoLookupResult{
			Root:   root,
			Reason: "inside current git work tree",
		}, nil
	}

	bases := []string{cwd}
	if parent := filepath.Dir(cwd); parent != "" && parent != cwd {
		bases = append(bases, parent)
	}
	if grandParent := filepath.Dir(filepath.Dir(cwd)); grandParent != "" && grandParent != cwd {
		bases = append(bases, grandParent)
	}

	candidates := make([]string, 0, 8)
	seen := map[string]struct{}{}
	for _, base := range bases {
		repos := discoverChildGitRepos(base, 2, 80)
		for _, repo := range repos {
			validRoot, err := discoverGitRootFromDir(repo)
			if err != nil {
				continue
			}
			absRepo, _ := filepath.Abs(validRoot)
			absRepo = strings.TrimSpace(absRepo)
			if absRepo == "" {
				continue
			}
			if _, ok := seen[absRepo]; ok {
				continue
			}
			seen[absRepo] = struct{}{}
			candidates = append(candidates, absRepo)
		}
	}
	if len(candidates) == 0 {
		return repoLookupResult{}, errors.New("no git repository found in current or nearby directories")
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		di := pathDistance(cwd, candidates[i])
		dj := pathDistance(cwd, candidates[j])
		if di != dj {
			return di < dj
		}
		return candidates[i] < candidates[j]
	})

	selected := candidates[0]
	alternatives := []string{}
	for _, candidate := range candidates[1:] {
		alternatives = append(alternatives, candidate)
		if len(alternatives) >= 4 {
			break
		}
	}
	return repoLookupResult{
		Root:         selected,
		Reason:       "nearby repository discovered",
		Alternatives: alternatives,
	}, nil
}

func discoverGitRootFromDir(dir string) (string, error) {
	out, err := runLocalCommandMax([]string{"git", "-C", dir, "rev-parse", "--show-toplevel"}, 4*time.Second, 2000)
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", errors.New("empty git root")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return root, nil
	}
	return absRoot, nil
}

func discoverChildGitRepos(base string, maxDepth, maxVisited int) []string {
	base = strings.TrimSpace(base)
	if base == "" {
		return nil
	}
	info, err := os.Stat(base)
	if err != nil || !info.IsDir() {
		return nil
	}

	type queueItem struct {
		Path  string
		Depth int
	}

	repos := make([]string, 0, 8)
	seenDirs := map[string]struct{}{}
	queue := []queueItem{{Path: base, Depth: 0}}
	visited := 0

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if _, ok := seenDirs[item.Path]; ok {
			continue
		}
		seenDirs[item.Path] = struct{}{}
		visited++
		if maxVisited > 0 && visited > maxVisited {
			break
		}

		if hasGitMetadata(item.Path) {
			repos = append(repos, item.Path)
			continue
		}
		if item.Depth >= maxDepth {
			continue
		}

		entries, err := os.ReadDir(item.Path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := strings.TrimSpace(entry.Name())
			if name == "" || name == ".git" {
				continue
			}
			if strings.HasPrefix(name, ".") {
				continue
			}
			nextPath := filepath.Join(item.Path, name)
			queue = append(queue, queueItem{Path: nextPath, Depth: item.Depth + 1})
		}
	}

	sort.Strings(repos)
	return repos
}

func hasGitMetadata(path string) bool {
	gitPath := filepath.Join(strings.TrimSpace(path), ".git")
	if gitPath == "" {
		return false
	}
	_, err := os.Stat(gitPath)
	return err == nil
}

func pathDistance(fromPath, toPath string) int {
	from := splitPathSegments(fromPath)
	to := splitPathSegments(toPath)
	common := 0
	for common < len(from) && common < len(to) && from[common] == to[common] {
		common++
	}
	return (len(from) - common) + (len(to) - common)
}

func splitPathSegments(path string) []string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return nil
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func collectRepoTimeline(repoRoot string) ([]repoTimelineEntry, error) {
	raw, err := runLocalCommandMax([]string{"git", "-C", repoRoot, "status", "--porcelain=1", "--untracked-files=all"}, localShellCommandTimeout, 12000)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []repoTimelineEntry{}, nil
	}

	stats := collectRepoDiffStatsForRepo(repoRoot)
	entries := make([]repoTimelineEntry, 0, 16)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		path := parseGitPorcelainPath(line[3:])
		if path == "" {
			continue
		}
		entry := repoTimelineEntry{
			Path:   path,
			Status: status,
			Kind:   describeRepoStatusKind(status),
		}
		if stat, ok := stats[path]; ok {
			entry.Stats = stat
		}

		absPath := filepath.Join(repoRoot, path)
		if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
			entry.WorkingMTime = info.ModTime()
		}
		commitTime, commitHash, commitSubject := queryRepoFileLastCommit(repoRoot, path)
		entry.CommitTime = commitTime
		entry.CommitHash = commitHash
		entry.CommitSubject = commitSubject

		entry.EffectiveTime = entry.CommitTime
		entry.TimeSource = "last commit"
		if !entry.WorkingMTime.IsZero() && (entry.EffectiveTime.IsZero() || entry.WorkingMTime.After(entry.EffectiveTime)) {
			entry.EffectiveTime = entry.WorkingMTime
			entry.TimeSource = "working tree mtime"
		}
		if entry.EffectiveTime.IsZero() {
			entry.TimeSource = "unknown"
		}

		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i].EffectiveTime
		right := entries[j].EffectiveTime
		if !left.Equal(right) {
			return left.After(right)
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func describeRepoStatusKind(status string) string {
	clean := strings.TrimSpace(status)
	if clean == "" {
		return "Edited"
	}
	if clean == "??" {
		return "Added"
	}
	if statusContainsRune(clean, 'R') {
		return "Renamed"
	}
	if statusContainsRune(clean, 'D') {
		return "Deleted"
	}
	if statusContainsRune(clean, 'A') {
		return "Added"
	}
	if statusContainsRune(clean, 'M') {
		return "Edited"
	}
	return "Edited"
}

func collectRepoDiffStatsForRepo(repoRoot string) map[string]repoDiffStat {
	stats := map[string]repoDiffStat{}
	for _, args := range [][]string{
		{"git", "-C", repoRoot, "diff", "--numstat", "--"},
		{"git", "-C", repoRoot, "diff", "--cached", "--numstat", "--"},
	} {
		raw, err := runLocalCommandMax(args, localShellCommandTimeout, 12000)
		if err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		for _, line := range strings.Split(raw, "\n") {
			path, stat, ok := parseRepoNumstatLine(line)
			if !ok {
				continue
			}
			prev, exists := stats[path]
			if !exists {
				stats[path] = stat
				continue
			}
			if prev.Known && stat.Known {
				prev.Added += stat.Added
				prev.Removed += stat.Removed
				prev.Known = true
			} else {
				prev.Known = false
			}
			stats[path] = prev
		}
	}
	return stats
}

func queryRepoFileLastCommit(repoRoot, path string) (time.Time, string, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return time.Time{}, "", ""
	}
	raw, err := runLocalCommandMax([]string{"git", "-C", repoRoot, "log", "-1", "--format=%ct\t%h\t%s", "--", path}, 5*time.Second, 3000)
	if err != nil || strings.TrimSpace(raw) == "" {
		return time.Time{}, "", ""
	}
	parts := strings.SplitN(strings.TrimSpace(raw), "\t", 3)
	if len(parts) < 3 {
		return time.Time{}, "", ""
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return time.Time{}, "", ""
	}
	return time.Unix(epoch, 0).Local(), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
}

func collectRecentRepoCommits(repoRoot string, limit int) []repoCommitEntry {
	if limit <= 0 {
		limit = 6
	}
	raw, err := runLocalCommandMax(
		[]string{"git", "-C", repoRoot, "log", fmt.Sprintf("-%d", limit), "--format=%ct\t%h\t%s"},
		6*time.Second,
		6000,
	)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	out := make([]repoCommitEntry, 0, limit)
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 3)
		if len(parts) < 3 {
			continue
		}
		epoch, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			continue
		}
		out = append(out, repoCommitEntry{
			Time:    time.Unix(epoch, 0).Local(),
			Hash:    strings.TrimSpace(parts[1]),
			Subject: strings.TrimSpace(parts[2]),
		})
	}
	return out
}

func gitCurrentBranch(repoRoot string) (string, error) {
	return runLocalCommandMax([]string{"git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD"}, 4*time.Second, 800)
}

func showSystemSummary() string {
	cwd := ""
	if dir, err := os.Getwd(); err == nil {
		cwd = strings.TrimSpace(dir)
	}
	snapshot := discoverHostEnvironmentSnapshot(cwd)
	lines := []string{
		"System summary:",
		"user=" + safeValue(snapshot.User, "unknown"),
		"identity=" + safeValue(snapshot.Identity, "unknown"),
		"os=" + safeValue(snapshot.Distro, snapshot.OS),
		"kernel=" + safeValue(snapshot.Kernel, "unknown"),
		"arch=" + safeValue(snapshot.Arch, "unknown"),
		"shell=" + safeValue(snapshot.Shell, "unknown"),
		"cwd=" + safeValue(snapshot.CWD, "unknown"),
		"package_manager=" + safeValue(snapshot.PackageManager, "(none)"),
		"local_time=" + safeValue(snapshot.NowLocal, "unknown"),
		"timezone=" + safeValue(snapshot.Timezone, "unknown"),
	}
	return strings.Join(lines, "\n")
}

func showRunningProcesses() (string, error) {
	lines := []string{"Running processes snapshot:"}
	strategies := make([]string, 0, 3)
	sections := make([]string, 0, 3)
	notes := make([]string, 0, 3)

	if output, executed, err := collectTopProcessSnapshot(); err == nil {
		strategies = append(strategies, "top")
		sections = append(sections, fmt.Sprintf("Strategy: top\nExecuted: %s\nOutput:\n%s", executed, output))
	} else {
		notes = append(notes, "top strategy unavailable: "+sanitizeSudoReasonText(err.Error()))
	}

	if output, executed, err := collectPSProcessSnapshot(); err == nil {
		strategies = append(strategies, "ps")
		sections = append(sections, fmt.Sprintf("Strategy: ps\nExecuted: %s\nOutput:\n%s", executed, output))
	} else {
		notes = append(notes, "ps strategy unavailable: "+sanitizeSudoReasonText(err.Error()))
	}

	if output, executed, err := collectRunningServiceSnapshot(); err == nil {
		strategies = append(strategies, "services")
		sections = append(sections, fmt.Sprintf("Strategy: service inventory\nExecuted: %s\nOutput:\n%s", executed, output))
	}

	if len(strategies) == 0 {
		return "", errors.New("unable to inspect running processes with available local tools (tried top/ps)")
	}

	lines = append(lines, "strategies="+strings.Join(strategies, ", "))
	lines = append(lines, sections...)
	if len(notes) > 0 {
		lines = append(lines, "notes:")
		for _, note := range notes {
			lines = append(lines, "- "+note)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func collectTopProcessSnapshot() (string, string, error) {
	if !commandExists("top") {
		return "", "", errors.New("`top` is not available")
	}
	attempts := [][]string{
		{"top", "-b", "-n", "1", "-w", "120"},
		{"top", "-b", "-n", "1"},
		{"top", "-l", "1", "-n", "25"},
		{"top", "-l", "1"},
	}
	var lastErr error
	for _, args := range attempts {
		raw, err := runLocalCommandMax(args, 10*time.Second, 12000)
		if err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(raw) == "" {
			lastErr = errors.New("empty output")
			continue
		}
		return trimCommandOutputLines(raw, 45), strings.Join(args, " "), nil
	}
	if lastErr == nil {
		lastErr = errors.New("no usable output")
	}
	return "", "", lastErr
}

func collectPSProcessSnapshot() (string, string, error) {
	if !commandExists("ps") {
		return "", "", errors.New("`ps` is not available")
	}
	attempts := [][]string{
		{"ps", "-eo", "pid,ppid,user,comm,%cpu,%mem,etime,state", "--sort=-%cpu"},
		{"ps", "-Ao", "pid,ppid,user,comm,%cpu,%mem,etime,state", "-r"},
		{"ps", "aux"},
	}
	var lastErr error
	for _, args := range attempts {
		raw, err := runLocalCommandMax(args, 8*time.Second, 10000)
		if err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(raw) == "" {
			lastErr = errors.New("empty output")
			continue
		}
		return trimCommandOutputLines(raw, 35), strings.Join(args, " "), nil
	}
	if lastErr == nil {
		lastErr = errors.New("no usable output")
	}
	return "", "", lastErr
}

func collectRunningServiceSnapshot() (string, string, error) {
	if commandExists("systemctl") {
		args := []string{"systemctl", "--type=service", "--state=running", "--no-pager", "--no-legend"}
		raw, err := runLocalCommandMax(args, 8*time.Second, 8000)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(raw) == "" {
			return "", "", errors.New("empty output")
		}
		return trimCommandOutputLines(raw, 30), strings.Join(args, " "), nil
	}
	return "", "", errors.New("no supported service manager command found")
}

func trimCommandOutputLines(raw string, maxLines int) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		remaining := len(lines) - maxLines
		lines = append(lines[:maxLines], fmt.Sprintf("...(%d more lines truncated)", remaining))
	}
	return strings.Join(lines, "\n")
}

func showNetworkIP() (string, error) {
	local := discoverLocalIPv4()
	public := discoverPublicIPv4()

	lines := []string{"Network IP snapshot:"}
	if len(local) == 0 {
		lines = append(lines, "local_ipv4=(unavailable)")
	} else {
		lines = append(lines, "local_ipv4="+strings.Join(local, ","))
	}
	if strings.TrimSpace(public) == "" {
		lines = append(lines, "public_ipv4=(unavailable)")
	} else {
		lines = append(lines, "public_ipv4="+public)
	}
	return strings.Join(lines, "\n"), nil
}

type geoLookupResult struct {
	Provider      string
	IP            string
	City          string
	Region        string
	Country       string
	CountryCode   string
	Latitude      float64
	Longitude     float64
	Timezone      string
	Org           string
	ISP           string
	VPN           bool
	Proxy         bool
	Tor           bool
	SecurityKnown bool
}

type vpnInspection struct {
	DefaultInterface string
	VPNInterfaces    []string
	VPNProcesses     []string
	ActiveVPNConns   []string
	GeoSignal        geoLookupResult
	LikelyActive     bool
}

type localNetworkTool struct {
	Name        string
	Commands    []string
	Description string
}

type webNetworkTool struct {
	Name        string
	URL         string
	Description string
}

func showNetworkProfile() (string, error) {
	location, _ := showNetworkLocation()
	vpn, _ := showVPNStatus()
	tools, _ := showNetworkToolsCatalog()
	ip, _ := showNetworkIP()

	lines := []string{
		"Network profile summary:",
	}
	if strings.TrimSpace(ip) != "" {
		lines = append(lines, ip)
	}
	if strings.TrimSpace(location) != "" {
		lines = append(lines, "")
		lines = append(lines, location)
	}
	if strings.TrimSpace(vpn) != "" {
		lines = append(lines, "")
		lines = append(lines, vpn)
	}
	if strings.TrimSpace(tools) != "" {
		lines = append(lines, "")
		lines = append(lines, tools)
	}
	return strings.Join(lines, "\n"), nil
}

func showNetworkLocation() (string, error) {
	publicIP := strings.TrimSpace(discoverPublicIPv4())
	if publicIP == "" {
		return "Network location snapshot:\npublic_ip=(unavailable)\nlocation=(unavailable)", nil
	}

	geo, err := lookupPublicIPLocation(publicIP)
	if err != nil {
		lines := []string{
			"Network location snapshot:",
			"public_ip=" + publicIP,
			"location=(lookup_failed)",
			"lookup_error=" + safeValue(strings.TrimSpace(err.Error()), "unknown error"),
		}
		return strings.Join(lines, "\n"), nil
	}

	locationParts := []string{}
	if geo.City != "" {
		locationParts = append(locationParts, geo.City)
	}
	if geo.Region != "" && geo.Region != geo.City {
		locationParts = append(locationParts, geo.Region)
	}
	if geo.Country != "" {
		locationParts = append(locationParts, geo.Country)
	}
	locationText := "(unknown)"
	if len(locationParts) > 0 {
		locationText = strings.Join(locationParts, ", ")
	}

	lines := []string{
		"Network location snapshot:",
		"provider=" + safeValue(geo.Provider, "unknown"),
		"public_ip=" + safeValue(geo.IP, publicIP),
		"location=" + locationText,
		"coordinates=" + fmt.Sprintf("%.4f,%.4f", geo.Latitude, geo.Longitude),
		"timezone=" + safeValue(geo.Timezone, "unknown"),
	}
	if geo.Org != "" {
		lines = append(lines, "network_org="+geo.Org)
	}
	if geo.ISP != "" {
		lines = append(lines, "isp="+geo.ISP)
	}
	if geo.SecurityKnown {
		lines = append(lines, fmt.Sprintf("security_signal=vpn:%t proxy:%t tor:%t", geo.VPN, geo.Proxy, geo.Tor))
	} else {
		lines = append(lines, "security_signal=(provider did not return vpn/proxy/tor flags)")
	}
	return strings.Join(lines, "\n"), nil
}

func showVPNStatus() (string, error) {
	inspection := inspectVPNState()

	lines := []string{
		"VPN inspection:",
		"default_route_interface=" + safeValue(inspection.DefaultInterface, "unknown"),
	}
	if len(inspection.VPNInterfaces) > 0 {
		lines = append(lines, "vpn_interfaces="+strings.Join(inspection.VPNInterfaces, ","))
	} else {
		lines = append(lines, "vpn_interfaces=(none detected)")
	}
	if len(inspection.ActiveVPNConns) > 0 {
		lines = append(lines, "active_vpn_connections="+strings.Join(inspection.ActiveVPNConns, ","))
	} else {
		lines = append(lines, "active_vpn_connections=(none detected)")
	}
	if len(inspection.VPNProcesses) > 0 {
		lines = append(lines, "vpn_processes="+strings.Join(inspection.VPNProcesses, " | "))
	} else {
		lines = append(lines, "vpn_processes=(none detected)")
	}
	if inspection.GeoSignal.SecurityKnown {
		lines = append(lines, fmt.Sprintf("geo_security_signal=vpn:%t proxy:%t tor:%t", inspection.GeoSignal.VPN, inspection.GeoSignal.Proxy, inspection.GeoSignal.Tor))
	}
	lines = append(lines, fmt.Sprintf("likely_vpn_active=%t", inspection.LikelyActive))
	return strings.Join(lines, "\n"), nil
}

func showNetworkToolsCatalog() (string, error) {
	available := []string{}
	missing := []string{}

	for _, tool := range knownLocalNetworkTools() {
		found := ""
		for _, cmd := range tool.Commands {
			if commandExists(cmd) {
				found = cmd
				break
			}
		}
		if found != "" {
			available = append(available, fmt.Sprintf("%s (via `%s`): %s", tool.Name, found, tool.Description))
		} else {
			missing = append(missing, fmt.Sprintf("%s (expects `%s`): %s", tool.Name, strings.Join(tool.Commands, "` or `"), tool.Description))
		}
	}

	lines := []string{
		"Network tools catalog:",
	}
	if len(available) > 0 {
		lines = append(lines, "local_tools_available:")
		for _, line := range available {
			lines = append(lines, "- "+line)
		}
	} else {
		lines = append(lines, "local_tools_available=(none detected)")
	}
	if len(missing) > 0 {
		lines = append(lines, "local_tools_missing:")
		for _, line := range missing {
			lines = append(lines, "- "+line)
		}
	}

	installCmd := inferNetworkToolsInstallCommand()
	if installCmd != "" {
		lines = append(lines, "recommended_install_command="+installCmd)
	}

	lines = append(lines, "web_tools_catalog:")
	for _, tool := range knownWebNetworkTools() {
		lines = append(lines, fmt.Sprintf("- %s: %s (%s)", tool.Name, tool.URL, tool.Description))
	}
	return strings.Join(lines, "\n"), nil
}

func knownLocalNetworkTools() []localNetworkTool {
	return []localNetworkTool{
		{Name: "Interface + route inspector", Commands: []string{"ip", "ifconfig"}, Description: "Inspect network interfaces, addresses, and default routes."},
		{Name: "Open ports inspector", Commands: []string{"ss", "netstat", "lsof"}, Description: "List listening TCP/UDP sockets and active port bindings."},
		{Name: "DNS lookup", Commands: []string{"dig", "nslookup", "host"}, Description: "Resolve domains, inspect DNS records, and troubleshoot DNS."},
		{Name: "Traceroute", Commands: []string{"traceroute", "mtr"}, Description: "Trace network path and identify latency hops."},
		{Name: "Network scanner", Commands: []string{"nmap"}, Description: "Probe hosts/ports for diagnostics and service discovery."},
		{Name: "VPN status", Commands: []string{"nmcli", "wg", "openvpn"}, Description: "Inspect VPN clients/connections and tunnel state."},
		{Name: "WHOIS", Commands: []string{"whois"}, Description: "Inspect domain/IP registration metadata."},
	}
}

func knownWebNetworkTools() []webNetworkTool {
	return []webNetworkTool{
		{Name: "IPify", URL: "https://api.ipify.org", Description: "Returns your current public IP address."},
		{Name: "IPWho.is", URL: "https://ipwho.is", Description: "Public IP geolocation + ISP/org + optional VPN/proxy/Tor flags."},
		{Name: "IPAPI", URL: "https://ipapi.co/json", Description: "Public IP geolocation and connection metadata fallback."},
		{Name: "BGP.he.net", URL: "https://bgp.he.net", Description: "ASN, prefixes, and routing context for IP/network operators."},
		{Name: "Shodan", URL: "https://www.shodan.io", Description: "Internet-exposed service discovery and host intelligence."},
		{Name: "Censys", URL: "https://search.censys.io", Description: "Internet-wide host/certificate search for security research."},
		{Name: "VirusTotal URL/IP", URL: "https://www.virustotal.com", Description: "Reputation and threat checks for URLs/domains/IPs."},
		{Name: "WhatIsMyDNS", URL: "https://www.whatsmydns.net", Description: "DNS propagation checks across global resolvers."},
	}
}

func inferNetworkToolsInstallCommand() string {
	if fileExists("./scripts/setup-host-deps.sh") {
		return "./scripts/setup-host-deps.sh --profile local -y"
	}
	if fileExists("scripts/setup-host-deps.sh") {
		return "scripts/setup-host-deps.sh --profile local -y"
	}
	return ""
}

func inspectVPNState() vpnInspection {
	inspection := vpnInspection{
		DefaultInterface: discoverDefaultRouteInterface(),
		VPNInterfaces:    discoverVPNInterfaces(),
		VPNProcesses:     discoverVPNProcesses(),
		ActiveVPNConns:   discoverActiveVPNConnectionsNmcli(),
	}

	publicIP := discoverPublicIPv4()
	if publicIP != "" {
		if geo, err := lookupPublicIPLocation(publicIP); err == nil {
			inspection.GeoSignal = geo
		}
	}

	inspection.LikelyActive = len(inspection.VPNInterfaces) > 0 ||
		len(inspection.VPNProcesses) > 0 ||
		len(inspection.ActiveVPNConns) > 0 ||
		(inspection.GeoSignal.SecurityKnown && (inspection.GeoSignal.VPN || inspection.GeoSignal.Proxy || inspection.GeoSignal.Tor))
	return inspection
}

func discoverDefaultRouteInterface() string {
	if !commandExists("ip") {
		return ""
	}
	raw, err := runLocalCommand([]string{"ip", "route", "show", "default"}, localShellCommandTimeout)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		for i := 0; i < len(fields)-1; i++ {
			if fields[i] == "dev" {
				return strings.TrimSpace(fields[i+1])
			}
		}
	}
	return ""
}

func discoverVPNInterfaces() []string {
	found := []string{}
	seen := map[string]struct{}{}

	if commandExists("ip") {
		if raw, err := runLocalCommand([]string{"ip", "-o", "link", "show"}, localShellCommandTimeout); err == nil {
			for _, line := range strings.Split(raw, "\n") {
				parts := strings.SplitN(line, ":", 3)
				if len(parts) < 2 {
					continue
				}
				iface := strings.TrimSpace(parts[1])
				if idx := strings.Index(iface, "@"); idx > 0 {
					iface = iface[:idx]
				}
				lower := strings.ToLower(strings.TrimSpace(iface))
				if lower == "" || !networkInterfaceNamePattern.MatchString(lower) {
					continue
				}
				if _, ok := seen[lower]; ok {
					continue
				}
				seen[lower] = struct{}{}
				found = append(found, lower)
			}
		}
	}

	if len(found) == 0 && commandExists("ifconfig") {
		if raw, err := runLocalCommand([]string{"ifconfig", "-a"}, localShellCommandTimeout); err == nil {
			for _, line := range strings.Split(raw, "\n") {
				line = strings.TrimSpace(line)
				matches := ifconfigInterfacePattern.FindStringSubmatch(line)
				if len(matches) != 2 {
					continue
				}
				lower := strings.ToLower(strings.TrimSpace(matches[1]))
				if lower == "" || !networkInterfaceNamePattern.MatchString(lower) {
					continue
				}
				if _, ok := seen[lower]; ok {
					continue
				}
				seen[lower] = struct{}{}
				found = append(found, lower)
			}
		}
	}

	return found
}

func discoverVPNProcesses() []string {
	if !commandExists("pgrep") {
		return nil
	}
	patterns := []string{"openvpn", "wg-quick", "wireguard", "tailscaled", "protonvpn", "nordvpn", "mullvad", "openconnect", "pia", "expressvpn", "ivpn"}
	found := []string{}
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		raw, err := runLocalCommand([]string{"pgrep", "-af", pattern}, 4*time.Second)
		if err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		for _, line := range strings.Split(raw, "\n") {
			entry := trimLocalText(strings.TrimSpace(line), 180)
			if entry == "" {
				continue
			}
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			found = append(found, entry)
		}
	}
	return found
}

func discoverActiveVPNConnectionsNmcli() []string {
	if !commandExists("nmcli") {
		return nil
	}
	raw, err := runLocalCommand([]string{"nmcli", "-t", "-f", "NAME,TYPE,DEVICE", "connection", "show", "--active"}, localShellCommandTimeout)
	if err != nil {
		return nil
	}
	out := []string{}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		if strings.ToLower(strings.TrimSpace(parts[1])) != "vpn" {
			continue
		}
		name := strings.TrimSpace(parts[0])
		device := ""
		if len(parts) > 2 {
			device = strings.TrimSpace(parts[2])
		}
		entry := name
		if device != "" {
			entry = name + " (device=" + device + ")"
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func lookupPublicIPLocation(ip string) (geoLookupResult, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return geoLookupResult{}, errors.New("public ip is required for location lookup")
	}

	if geo, err := lookupGeoViaIPWhoIs(ip); err == nil {
		return geo, nil
	}
	return lookupGeoViaIPAPI()
}

func lookupGeoViaIPWhoIs(ip string) (geoLookupResult, error) {
	raw, err := fetchWebText("https://ipwho.is/" + ip)
	if err != nil {
		return geoLookupResult{}, err
	}

	var payload struct {
		Success     bool    `json:"success"`
		Message     string  `json:"message"`
		IP          string  `json:"ip"`
		City        string  `json:"city"`
		Region      string  `json:"region"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Timezone    struct {
			ID string `json:"id"`
		} `json:"timezone"`
		Connection struct {
			ISP string `json:"isp"`
			Org string `json:"org"`
		} `json:"connection"`
		Security struct {
			VPN   bool `json:"vpn"`
			Proxy bool `json:"proxy"`
			Tor   bool `json:"tor"`
		} `json:"security"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return geoLookupResult{}, err
	}
	if !payload.Success {
		return geoLookupResult{}, errors.New(safeValue(strings.TrimSpace(payload.Message), "ipwho.is returned unsuccessful status"))
	}

	return geoLookupResult{
		Provider:      "ipwho.is",
		IP:            strings.TrimSpace(payload.IP),
		City:          strings.TrimSpace(payload.City),
		Region:        strings.TrimSpace(payload.Region),
		Country:       strings.TrimSpace(payload.Country),
		CountryCode:   strings.TrimSpace(payload.CountryCode),
		Latitude:      payload.Latitude,
		Longitude:     payload.Longitude,
		Timezone:      strings.TrimSpace(payload.Timezone.ID),
		Org:           strings.TrimSpace(payload.Connection.Org),
		ISP:           strings.TrimSpace(payload.Connection.ISP),
		VPN:           payload.Security.VPN,
		Proxy:         payload.Security.Proxy,
		Tor:           payload.Security.Tor,
		SecurityKnown: true,
	}, nil
}

func lookupGeoViaIPAPI() (geoLookupResult, error) {
	raw, err := fetchWebText("https://ipapi.co/json/")
	if err != nil {
		return geoLookupResult{}, err
	}

	var payload struct {
		IP          string  `json:"ip"`
		City        string  `json:"city"`
		Region      string  `json:"region"`
		CountryName string  `json:"country_name"`
		CountryCode string  `json:"country_code"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Timezone    string  `json:"timezone"`
		Org         string  `json:"org"`
		Error       bool    `json:"error"`
		Reason      string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return geoLookupResult{}, err
	}
	if payload.Error {
		return geoLookupResult{}, errors.New(safeValue(strings.TrimSpace(payload.Reason), "ipapi returned error"))
	}

	return geoLookupResult{
		Provider:      "ipapi.co",
		IP:            strings.TrimSpace(payload.IP),
		City:          strings.TrimSpace(payload.City),
		Region:        strings.TrimSpace(payload.Region),
		Country:       strings.TrimSpace(payload.CountryName),
		CountryCode:   strings.TrimSpace(payload.CountryCode),
		Latitude:      payload.Latitude,
		Longitude:     payload.Longitude,
		Timezone:      strings.TrimSpace(payload.Timezone),
		Org:           strings.TrimSpace(payload.Org),
		SecurityKnown: false,
	}, nil
}

func fetchWebText(url string) (string, error) {
	if commandExists("curl") {
		raw, err := runLocalCommand([]string{"curl", "-fsS", "--max-time", "8", url}, localShellCommandTimeout)
		if err == nil && strings.TrimSpace(raw) != "" {
			return raw, nil
		}
	}
	if commandExists("wget") {
		raw, err := runLocalCommand([]string{"wget", "-qO-", url}, localShellCommandTimeout)
		if err == nil && strings.TrimSpace(raw) != "" {
			return raw, nil
		}
	}
	return "", errors.New("no available fetch tool (`curl` or `wget`) could retrieve web content")
}

func trimLocalText(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if maxChars > 0 && len(value) > maxChars {
		return value[:maxChars]
	}
	return value
}

func showOpenPorts(detailed bool) (string, error) {
	command := []string{}
	useSudo := false

	switch {
	case commandExists("ss"):
		if detailed {
			command = []string{"ss", "-lntup"}
			useSudo = true
		} else {
			command = []string{"ss", "-lntu"}
		}
	case commandExists("netstat"):
		if detailed {
			command = []string{"netstat", "-lntup"}
			useSudo = true
		} else {
			command = []string{"netstat", "-lntu"}
		}
	case commandExists("lsof"):
		command = []string{"lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP"}
		if detailed {
			useSudo = true
		}
	default:
		return "", errors.New("no supported port-inspection tool found (need one of: ss, netstat, lsof)")
	}

	var (
		output string
		err    error
	)
	if useSudo {
		if err := ensureLocalPermission(permissionKeyShellSudo, "Allow running privileged network port inspection commands."); err != nil {
			return "", err
		}
		output, err = runLocalSudoCommand(command, localShellCommandTimeout)
	} else {
		output, err = runLocalCommand(command, localShellCommandTimeout)
	}
	if err != nil {
		return "", err
	}

	executed := strings.Join(command, " ")
	if useSudo {
		executed = "sudo " + executed
	}
	mode := "standard"
	if detailed {
		mode = "detailed"
	}
	lines := []string{
		"Open ports snapshot (" + mode + "):",
		"Executed: " + executed,
	}
	if strings.TrimSpace(output) != "" {
		lines = append(lines, "Output:")
		lines = append(lines, output)
	}
	return strings.Join(lines, "\n"), nil
}

func discoverLocalIPv4() []string {
	ips := []string{}
	seen := map[string]struct{}{}

	if commandExists("ip") {
		if raw, err := runLocalCommand([]string{"ip", "-4", "-o", "addr", "show", "scope", "global"}, localShellCommandTimeout); err == nil {
			for _, line := range strings.Split(raw, "\n") {
				fields := strings.Fields(line)
				for i := 0; i < len(fields)-1; i++ {
					if fields[i] != "inet" {
						continue
					}
					value := strings.TrimSpace(fields[i+1])
					if idx := strings.Index(value, "/"); idx > 0 {
						value = value[:idx]
					}
					if value == "" {
						continue
					}
					if _, ok := seen[value]; ok {
						continue
					}
					seen[value] = struct{}{}
					ips = append(ips, value)
				}
			}
		}
	}

	if len(ips) == 0 && commandExists("hostname") {
		if raw, err := runLocalCommand([]string{"hostname", "-I"}, localShellCommandTimeout); err == nil {
			for _, value := range strings.Fields(raw) {
				value = strings.TrimSpace(value)
				if value == "" || strings.Contains(value, ":") {
					continue
				}
				if _, ok := seen[value]; ok {
					continue
				}
				seen[value] = struct{}{}
				ips = append(ips, value)
			}
		}
	}

	return ips
}

func discoverPublicIPv4() string {
	if commandExists("curl") {
		if raw, err := runLocalCommand([]string{"curl", "-fsS", "--max-time", "5", "https://api.ipify.org"}, localShellCommandTimeout); err == nil {
			return strings.TrimSpace(raw)
		}
	}
	if commandExists("wget") {
		if raw, err := runLocalCommand([]string{"wget", "-qO-", "https://api.ipify.org"}, localShellCommandTimeout); err == nil {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

func runLocalSudoCommand(args []string, timeout time.Duration) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty sudo command")
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return "", errors.New("`sudo` is not available on this system")
	}
	if err := ensureSudoCredential(); err != nil {
		return "", err
	}
	sudoArgs := append([]string{"-n"}, args...)
	return runLocalCommand(append([]string{"sudo"}, sudoArgs...), timeout)
}

func ensureSudoCredential() error {
	if _, err := runLocalCommand([]string{"sudo", "-n", "true"}, 4*time.Second); err == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), localShellSudoAuthTimeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, "sudo", "-v")
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		cmd.Stdin = tty
		cmd.Stdout = tty
		cmd.Stderr = tty
	} else {
		if !isCharDevice(os.Stdin) || !isCharDevice(os.Stdout) {
			return errors.New("sudo authentication required but no interactive terminal is available (run `sudo -v` manually and retry)")
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return errors.New("sudo authentication timed out")
		}
		return fmt.Errorf("sudo authentication failed: %w", err)
	}
	if _, err := runLocalCommand([]string{"sudo", "-n", "true"}, 4*time.Second); err != nil {
		return errors.New("sudo credentials are unavailable after authentication")
	}
	return nil
}

func commandExists(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func runLocalCommand(args []string, timeout time.Duration) (string, error) {
	return runLocalCommandMax(args, timeout, localShellOutputMaxChars)
}

func runLocalCommandMax(args []string, timeout time.Duration, maxChars int) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty command")
	}
	if maxChars <= 0 {
		maxChars = localShellOutputMaxChars
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if len(text) > maxChars {
		text = text[:maxChars] + "...(truncated)"
	}
	if ctx.Err() == context.DeadlineExceeded {
		if text == "" {
			text = args[0] + " timed out"
		}
		return "", errors.New(text)
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", errors.New(text)
	}
	return text, nil
}

func validateRelativePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("path is required")
	}
	if strings.Contains(path, "~") {
		return errors.New("home-expansion (~) paths are not allowed in local chat mode")
	}
	if filepath.IsAbs(path) {
		return errors.New("absolute paths are not allowed in local chat mode")
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return errors.New("path must reference a file")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("paths outside the current directory are not allowed in local chat mode")
	}
	return nil
}

func updateLocalShellStateFromAssistant(state *localShellState, assistantResult string) {
	if state == nil {
		return
	}
	command, ok := extractSuggestedSafeCommand(assistantResult)
	if !ok {
		return
	}
	state.LastSuggestedCommand = command
	state.LastSuggestedAt = time.Now()
}

func extractSuggestedSafeCommand(text string) (string, bool) {
	matches := shellBacktickPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		candidate := strings.TrimSpace(match[1])
		if candidate == "" {
			continue
		}
		if strings.ContainsAny(candidate, "|&;<>") {
			continue
		}
		args := strings.Fields(candidate)
		if len(args) == 0 {
			continue
		}
		if _, ok := allowedLocalShellCommands[strings.ToLower(args[0])]; !ok {
			continue
		}
		return strings.Join(args, " "), true
	}
	return "", false
}
