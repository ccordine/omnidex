package main

import (
	"strings"
)

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
