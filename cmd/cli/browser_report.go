package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func reportToText(report map[string]any) string {
	lines := []string{
		"Local browser scan:",
		"generated_at=" + safeValue(fmt.Sprintf("%v", report["generated_at"]), "unknown"),
		fmt.Sprintf("process_count=%v", report["process_count"]),
		fmt.Sprintf("endpoint_count=%v", report["endpoint_count"]),
	}

	if processes, ok := report["processes"].([]browserProcess); ok && len(processes) > 0 {
		lines = append(lines, "processes:")
		for _, proc := range processes {
			desc := fmt.Sprintf("- pid=%d name=%s", proc.PID, safeValue(proc.Name, proc.ExecName))
			if proc.DebugPort > 0 {
				desc += fmt.Sprintf(" debug_port=%d", proc.DebugPort)
			}
			if strings.TrimSpace(proc.Cmdline) != "" {
				desc += " cmd=" + truncateText(proc.Cmdline, 220)
			}
			lines = append(lines, desc)
		}
	}

	if endpoints, ok := report["endpoints"].([]browserEndpoint); ok && len(endpoints) > 0 {
		lines = append(lines, "tabs:")
		for _, endpoint := range endpoints {
			header := fmt.Sprintf("- port=%d browser=%s protocol=%s", endpoint.Port, safeValue(endpoint.Version.Browser, "unknown"), safeValue(endpoint.Version.ProtocolVersion, "unknown"))
			lines = append(lines, header)
			for _, target := range endpoint.Targets {
				if strings.ToLower(strings.TrimSpace(target.Type)) != "page" {
					continue
				}
				lines = append(lines, fmt.Sprintf("  • %s | %s", safeValue(target.Title, "(untitled)"), safeValue(target.URL, "(no url)")))
			}
		}
	}

	if events, ok := report["console_events"].([]browserConsoleEntry); ok && len(events) > 0 {
		lines = append(lines, "console_events:")
		for _, event := range events {
			line := fmt.Sprintf("- [%s] %s %s", safeValue(event.Time, "unknown"), strings.ToUpper(safeValue(event.Level, "log")), safeValue(event.Text, ""))
			if strings.TrimSpace(event.TabTitle) != "" {
				line += " tab=" + event.TabTitle
			}
			if strings.TrimSpace(event.URL) != "" {
				line += " url=" + event.URL
			}
			lines = append(lines, line)
		}
	} else if _, ok := report["console_events"]; ok {
		lines = append(lines, "console_events: none captured")
	}
	if warningValues, ok := report["warnings"].([]string); ok && len(warningValues) > 0 {
		lines = append(lines, "warnings:")
		for _, warning := range warningValues {
			lines = append(lines, "- "+warning)
		}
	}
	if endpointCount, ok := report["endpoint_count"].(int); ok && endpointCount == 0 {
		lines = append(lines, "Note: tab and console access usually requires launching a browser with --remote-debugging-port=9222.")
	}

	if len(lines) == 4 {
		lines = append(lines, "No active browser process or debuggable endpoint found.")
	}
	return strings.Join(lines, "\n")
}

func discoverBrowserProcesses() []browserProcess {
	rootEntries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	processes := make([]browserProcess, 0, 32)
	for _, entry := range rootEntries {
		if !entry.IsDir() {
			continue
		}
		pidText := strings.TrimSpace(entry.Name())
		if !numericDirPattern.MatchString(pidText) {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid <= 0 {
			continue
		}
		proc, ok := readBrowserProcess(pid)
		if !ok {
			continue
		}
		processes = append(processes, proc)
	}

	sort.Slice(processes, func(i, j int) bool {
		if processes[i].Name == processes[j].Name {
			return processes[i].PID < processes[j].PID
		}
		return processes[i].Name < processes[j].Name
	})
	return processes
}

func readBrowserProcess(pid int) (browserProcess, bool) {
	commPath := filepath.Join("/proc", strconv.Itoa(pid), "comm")
	cmdPath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	exePath := filepath.Join("/proc", strconv.Itoa(pid), "exe")

	commBytes, err := os.ReadFile(commPath)
	if err != nil {
		return browserProcess{}, false
	}
	comm := strings.TrimSpace(string(commBytes))
	exeName := ""
	if target, err := os.Readlink(exePath); err == nil {
		exeName = strings.TrimSpace(filepath.Base(target))
	}

	cmdline := ""
	firstArg := ""
	if raw, err := os.ReadFile(cmdPath); err == nil && len(raw) > 0 {
		parts := strings.Split(string(raw), "\x00")
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			filtered = append(filtered, part)
		}
		if len(filtered) > 0 {
			firstArg = filtered[0]
		}
		cmdline = strings.Join(filtered, " ")
	}

	name := classifyBrowserName(comm, exeName, firstArg, cmdline)
	if name == "" {
		return browserProcess{}, false
	}
	if isLikelyBrowserHelperProcess(cmdline) {
		return browserProcess{}, false
	}

	return browserProcess{
		PID:       pid,
		Name:      name,
		ExecName:  exeName,
		Cmdline:   cmdline,
		DebugPort: parseDebugPortFromCmdline(cmdline),
	}, true
}

func classifyBrowserName(comm, exeName, firstArg, cmdline string) string {
	joined := strings.ToLower(strings.TrimSpace(strings.Join([]string{exeName, filepath.Base(firstArg), firstArg}, " ")))
	commLower := strings.ToLower(strings.TrimSpace(comm))
	if strings.Contains(joined, "firefox") || commLower == "firefox" {
		return "firefox"
	}
	switch {
	case strings.Contains(joined, "brave"):
		return "brave"
	case strings.Contains(joined, "chromium"):
		return "chromium"
	case strings.Contains(joined, "google-chrome"), strings.Contains(joined, "chrome"):
		return "chrome"
	case strings.Contains(joined, "microsoft-edge"), strings.Contains(joined, "msedge"), strings.Contains(joined, "edge"):
		return "edge"
	case strings.Contains(joined, "vivaldi"):
		return "vivaldi"
	case strings.Contains(joined, "opera"):
		return "opera"
	case commLower == "chrome":
		// "chrome" is ambiguous for Electron helpers; only accept when first argv
		// references a browser-like binary.
		if strings.Contains(strings.ToLower(filepath.Base(firstArg)), "chrome") {
			return "chrome"
		}
		return ""
	default:
		return ""
	}
}

func isLikelyBrowserHelperProcess(cmdline string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmdline))
	if lower == "" {
		return false
	}
	helperMarkers := []string{
		"steamwebhelper",
		"chrome_crashpad_handler",
		"crashpad-handler",
		"crashhelper",
		"electron",
		"slack",
		"discord",
		"teams-for-linux",
		"--type=",
		"-contentproc",
		" --utility-sub-type=",
	}
	return containsAnyPhrase(lower, helperMarkers)
}

func parseDebugPortFromCmdline(cmdline string) int {
	match := browserDebugPortPattern.FindStringSubmatch(strings.TrimSpace(cmdline))
	if len(match) != 2 {
		return 0
	}
	port, err := strconv.Atoi(strings.TrimSpace(match[1]))
	if err != nil || port <= 0 || port > 65535 {
		return 0
	}
	return port
}

func extractDebugPorts(processes []browserProcess) []int {
	ports := make([]int, 0, len(processes))
	for _, proc := range processes {
		if proc.DebugPort > 0 {
			ports = append(ports, proc.DebugPort)
		}
	}
	return ports
}

func parsePortList(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		port, err := strconv.Atoi(value)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		out = append(out, port)
	}
	return out
}

func mergePorts(groups ...[]int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, 16)
	for _, group := range groups {
		for _, port := range group {
			if port <= 0 || port > 65535 {
				continue
			}
			if _, ok := seen[port]; ok {
				continue
			}
			seen[port] = struct{}{}
			out = append(out, port)
		}
	}
	sort.Ints(out)
	return out
}
