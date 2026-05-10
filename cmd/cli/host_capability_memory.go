package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func persistHostCapabilityMemory(c *client.Client, snapshot hostEnvironmentSnapshot) error {
	if c == nil {
		return nil
	}
	content := strings.TrimSpace(buildHostCapabilityMemoryContent(snapshot))
	if content == "" {
		return nil
	}

	source := "host-capability"
	if user := strings.TrimSpace(snapshot.User); user != "" && user != "unknown" {
		source = source + ":" + user
	}
	tags := hostCapabilityTags(snapshot)
	_, err := c.AddMemory(context.Background(), source, model.MemoryKindProcedural, content, tags)
	return err
}

func buildHostCapabilityMemoryContent(snapshot hostEnvironmentSnapshot) string {
	lines := []string{
		"Host capability discovery snapshot:",
		"os=" + safeValue(snapshot.OS, "unknown"),
		"arch=" + safeValue(snapshot.Arch, "unknown"),
		"distro=" + safeValue(snapshot.Distro, "unknown"),
		"primary_package_manager=" + safeValue(snapshot.PackageManager, "(none)"),
		"package_managers=" + strings.Join(snapshot.PackageManagers, ","),
	}

	if len(snapshot.AvailableTools) > 0 {
		tools := append([]string(nil), snapshot.AvailableTools...)
		sort.Strings(tools)
		lines = append(lines, "available_tools="+strings.Join(tools, ","))
	}
	if len(snapshot.InstalledPackage) > 0 {
		pkgs := append([]string(nil), snapshot.InstalledPackage...)
		sort.Strings(pkgs)
		lines = append(lines, "installed_packages="+strings.Join(pkgs, ","))
	}

	capabilities := deriveHostCapabilities(snapshot)
	if len(capabilities) > 0 {
		lines = append(lines, "actions:")
		for _, capability := range capabilities {
			lines = append(lines, "- "+capability)
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func deriveHostCapabilities(snapshot hostEnvironmentSnapshot) []string {
	toolSet := make(map[string]struct{}, len(snapshot.AvailableTools))
	for _, tool := range snapshot.AvailableTools {
		name := strings.ToLower(strings.TrimSpace(tool))
		if name == "" {
			continue
		}
		toolSet[name] = struct{}{}
	}
	has := func(tools ...string) bool {
		for _, tool := range tools {
			name := strings.ToLower(strings.TrimSpace(tool))
			if name == "" {
				continue
			}
			if _, ok := toolSet[name]; ok {
				return true
			}
		}
		return false
	}

	capabilities := make([]string, 0, 24)
	add := func(action string) {
		action = strings.TrimSpace(action)
		if action == "" {
			return
		}
		capabilities = append(capabilities, action)
	}

	if has("sh", "bash", "zsh") {
		add("local_shell.run_command")
	}
	if has("touch", "mv", "cp") {
		add("local_shell.file_create_rename")
	}
	if has("git") {
		add("repo.inspect_and_diff")
	}
	if has("go") {
		add("repo.go_build_and_test")
	}
	if has("npm", "pnpm", "yarn", "node") {
		add("repo.node_dependency_and_test")
	}
	if has("python3", "python", "pip", "pip3", "pytest") {
		add("repo.python_dependency_and_test")
	}
	if has("docker", "docker-compose", "podman") {
		add("container.build_and_compose_control")
	}
	if has("vlc", "playerctl") {
		add("media.playback_control_and_next_episode")
	}
	if has("ffmpeg") {
		add("media.subtitle_audio_video_processing")
	}
	if has("ip", "ifconfig", "ss", "netstat", "lsof") {
		add("network.local_ip_and_open_ports_inspection")
	}
	if has("dig", "nslookup", "host", "traceroute", "mtr", "whois", "nmap") {
		add("network.dns_route_whois_scan_diagnostics")
	}
	if has("nmcli", "wg", "openvpn") {
		add("network.vpn_detection_and_status")
	}
	if len(snapshot.PackageManagers) > 0 {
		add(fmt.Sprintf("system.package_install_via_%s", strings.Join(snapshot.PackageManagers, "|")))
	}

	sort.Strings(capabilities)
	out := make([]string, 0, len(capabilities))
	seen := map[string]struct{}{}
	for _, capability := range capabilities {
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		out = append(out, capability)
	}
	return out
}

func hostCapabilityTags(snapshot hostEnvironmentSnapshot) []string {
	tags := []string{"host-capability", "auto-discovery", "tools", "actions"}
	if osTag := normalizeCapabilityTag(snapshot.OS); osTag != "" {
		tags = append(tags, "os-"+osTag)
	}
	for _, manager := range snapshot.PackageManagers {
		if pm := normalizeCapabilityTag(manager); pm != "" {
			tags = append(tags, "pm-"+pm)
		}
	}
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		clean := normalizeCapabilityTag(tag)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

func normalizeCapabilityTag(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return ""
	}
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		if r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, lower)
	clean = strings.Trim(clean, "-")
	return clean
}
