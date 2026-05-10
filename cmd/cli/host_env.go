package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"sort"
	"strings"
	"time"
)

const hostProbeTimeout = 2 * time.Second

type hostEnvironmentSnapshot struct {
	OS               string
	Arch             string
	Kernel           string
	Distro           string
	Shell            string
	User             string
	Identity         string
	CWD              string
	PackageManager   string
	PackageManagers  []string
	AvailableTools   []string
	InstalledPackage []string
	NowUTC           string
	NowLocal         string
	Timezone         string
	Weekday          string
	EpochUnix        int64
}

func discoverHostEnvironmentSnapshot(cwd string) hostEnvironmentSnapshot {
	cleanCWD := strings.TrimSpace(cwd)
	if cleanCWD == "" {
		if dir, err := os.Getwd(); err == nil {
			cleanCWD = strings.TrimSpace(dir)
		}
	}

	packageManagers := detectHostPackageManagers()
	packageManager := ""
	if len(packageManagers) > 0 {
		packageManager = packageManagers[0]
	}
	availableTools := discoverHostTools()
	installed := discoverInstalledPackages(packageManager, availableTools)
	now := time.Now()
	userName, identity := discoverCurrentUserIdentity()

	return hostEnvironmentSnapshot{
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		Kernel:           safeCommandOutput("uname", "-sr"),
		Distro:           discoverOSPrettyName(),
		Shell:            safeValue(os.Getenv("SHELL"), "unknown"),
		User:             safeValue(userName, "unknown"),
		Identity:         safeValue(identity, "unknown"),
		CWD:              safeValue(cleanCWD, "unknown"),
		PackageManager:   safeValue(packageManager, "(none)"),
		PackageManagers:  packageManagers,
		AvailableTools:   availableTools,
		InstalledPackage: installed,
		NowUTC:           now.UTC().Format(time.RFC3339),
		NowLocal:         now.Format(time.RFC3339),
		Timezone:         safeValue(now.Location().String(), "unknown"),
		Weekday:          now.Weekday().String(),
		EpochUnix:        now.Unix(),
	}
}

func applyHostEnvironmentMetadata(metadata map[string]any, snapshot hostEnvironmentSnapshot) {
	if metadata == nil {
		return
	}

	metadata["host_env_os"] = snapshot.OS
	metadata["host_env_arch"] = snapshot.Arch
	metadata["host_env_kernel"] = snapshot.Kernel
	metadata["host_env_distro"] = snapshot.Distro
	metadata["host_env_shell"] = snapshot.Shell
	metadata["host_env_user"] = snapshot.User
	metadata["host_env_identity"] = snapshot.Identity
	metadata["host_env_cwd"] = snapshot.CWD
	metadata["host_env_package_manager"] = snapshot.PackageManager
	metadata["host_env_package_managers"] = strings.Join(snapshot.PackageManagers, ",")
	metadata["host_tools_available"] = strings.Join(snapshot.AvailableTools, ",")
	metadata["host_packages_installed"] = strings.Join(snapshot.InstalledPackage, ",")
	metadata["host_discovery_time"] = time.Now().UTC().Format(time.RFC3339)
	metadata["host_clock_utc"] = snapshot.NowUTC
	metadata["host_clock_local"] = snapshot.NowLocal
	metadata["host_clock_tz"] = snapshot.Timezone
	metadata["host_clock_weekday"] = snapshot.Weekday
	metadata["host_clock_epoch"] = snapshot.EpochUnix
}

func applyHostTemporalMetadata(metadata map[string]any, now time.Time) {
	if metadata == nil {
		return
	}
	now = now.Local()
	metadata["host_clock_utc"] = now.UTC().Format(time.RFC3339)
	metadata["host_clock_local"] = now.Format(time.RFC3339)
	metadata["host_clock_tz"] = safeValue(now.Location().String(), "unknown")
	metadata["host_clock_weekday"] = now.Weekday().String()
	metadata["host_clock_epoch"] = now.Unix()
}

func discoverCurrentUserIdentity() (string, string) {
	envUser := strings.TrimSpace(os.Getenv("USER"))
	if current, err := user.Current(); err == nil && current != nil {
		name := strings.TrimSpace(current.Username)
		if name == "" {
			name = envUser
		}
		identity := name
		if uid := strings.TrimSpace(current.Uid); uid != "" {
			identity = strings.TrimSpace(identity + " uid=" + uid)
		}
		if gid := strings.TrimSpace(current.Gid); gid != "" {
			identity = strings.TrimSpace(identity + " gid=" + gid)
		}
		return name, identity
	}
	return envUser, envUser
}

func detectHostPackageManager() string {
	managers := detectHostPackageManagers()
	if len(managers) > 0 {
		return managers[0]
	}
	return ""
}

func detectHostPackageManagers() []string {
	type managerProbe struct {
		Canonical string
		Binaries  []string
	}

	probes := []managerProbe{
		{Canonical: "dnf", Binaries: []string{"dnf"}},
		{Canonical: "yum", Binaries: []string{"yum"}},
		{Canonical: "apt-get", Binaries: []string{"apt-get", "apt"}},
		{Canonical: "pacman", Binaries: []string{"pacman"}},
		{Canonical: "apk", Binaries: []string{"apk"}},
		{Canonical: "brew", Binaries: []string{"brew"}},
		{Canonical: "zypper", Binaries: []string{"zypper"}},
		{Canonical: "rpm", Binaries: []string{"rpm"}},
		{Canonical: "dpkg", Binaries: []string{"dpkg-query", "dpkg"}},
	}

	found := make([]string, 0, len(probes))
	seen := map[string]struct{}{}
	for _, probe := range probes {
		for _, binary := range probe.Binaries {
			if _, err := exec.LookPath(binary); err != nil {
				continue
			}
			if _, ok := seen[probe.Canonical]; !ok {
				seen[probe.Canonical] = struct{}{}
				found = append(found, probe.Canonical)
			}
			break
		}
	}
	return found
}

func discoverHostTools() []string {
	candidates := []string{
		"sh", "bash", "zsh",
		"git", "make",
		"go", "python3", "python", "node", "npm", "pnpm", "yarn",
		"docker", "podman", "kubectl",
		"ffmpeg", "vlc", "playerctl",
		"ip", "ifconfig", "ss", "netstat", "lsof", "dig", "nslookup", "host", "traceroute", "mtr", "whois", "nmap", "nmcli", "wg", "openvpn", "pgrep",
		"rg", "sed", "awk", "jq", "curl", "wget",
	}

	out := make([]string, 0, len(candidates))
	for _, tool := range candidates {
		if _, err := exec.LookPath(tool); err == nil {
			out = append(out, tool)
		}
	}
	sort.Strings(out)
	return out
}

func discoverInstalledPackages(packageManager string, availableTools []string) []string {
	candidates := buildInstalledPackageProbeList(packageManager, availableTools)
	if len(candidates) == 0 || strings.TrimSpace(packageManager) == "" {
		return nil
	}

	out := make([]string, 0, len(candidates))
	for _, pkg := range candidates {
		if version, ok := queryInstalledPackage(packageManager, pkg); ok {
			out = append(out, pkg+"="+version)
		}
	}
	sort.Strings(out)
	return out
}

func buildInstalledPackageProbeList(packageManager string, availableTools []string) []string {
	if strings.TrimSpace(packageManager) == "" {
		return nil
	}

	baselineByManager := map[string][]string{
		"dnf":     {"vlc", "playerctl", "docker", "golang", "nodejs", "npm", "python3", "git", "make", "ffmpeg", "iproute", "bind-utils", "traceroute", "whois", "nmap", "NetworkManager"},
		"yum":     {"vlc", "playerctl", "docker", "golang", "nodejs", "npm", "python3", "git", "make", "ffmpeg", "iproute", "bind-utils", "traceroute", "whois", "nmap", "NetworkManager"},
		"apt-get": {"vlc", "playerctl", "docker.io", "golang", "golang-go", "nodejs", "npm", "python3", "git", "make", "ffmpeg", "iproute2", "dnsutils", "traceroute", "whois", "nmap", "network-manager"},
		"pacman":  {"vlc", "playerctl", "docker", "go", "nodejs", "npm", "python", "git", "make", "ffmpeg", "iproute2", "bind", "traceroute", "whois", "nmap", "networkmanager"},
		"apk":     {"vlc", "playerctl", "docker", "go", "nodejs", "npm", "python3", "git", "make", "ffmpeg", "iproute2", "bind-tools", "traceroute", "whois", "nmap", "networkmanager"},
		"brew":    {"vlc", "playerctl", "docker", "go", "node", "python", "git", "ffmpeg", "bind", "whois", "nmap"},
		"rpm":     {"vlc", "playerctl", "docker", "golang", "nodejs", "npm", "python3", "git", "make", "ffmpeg", "iproute", "bind-utils", "traceroute", "whois", "nmap", "NetworkManager"},
		"dpkg":    {"vlc", "playerctl", "docker.io", "golang", "golang-go", "nodejs", "npm", "python3", "git", "make", "ffmpeg", "iproute2", "dnsutils", "traceroute", "whois", "nmap", "network-manager"},
		"zypper":  {"vlc", "playerctl", "docker", "go", "nodejs", "npm", "python3", "git", "make", "ffmpeg", "iproute2", "bind-utils", "traceroute", "whois", "nmap", "NetworkManager"},
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, 32)
	add := func(pkg string) {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			return
		}
		if _, ok := seen[pkg]; ok {
			return
		}
		seen[pkg] = struct{}{}
		out = append(out, pkg)
	}

	for _, pkg := range baselineByManager[packageManager] {
		add(pkg)
	}
	for _, tool := range availableTools {
		for _, pkg := range packageProbeCandidatesForTool(packageManager, tool) {
			add(pkg)
		}
	}

	const maxProbePackages = 40
	if len(out) > maxProbePackages {
		out = out[:maxProbePackages]
	}
	return out
}

func packageProbeCandidatesForTool(packageManager, tool string) []string {
	normalizedManager := strings.TrimSpace(packageManager)
	normalizedTool := strings.ToLower(strings.TrimSpace(tool))
	if normalizedManager == "" || normalizedTool == "" {
		return nil
	}

	candidates := []string{normalizedTool}
	add := func(values ...string) {
		candidates = append(candidates, values...)
	}

	switch normalizedTool {
	case "go":
		switch normalizedManager {
		case "apt-get", "dpkg":
			add("golang", "golang-go")
		default:
			add("golang", "go")
		}
	case "node":
		add("nodejs")
	case "python":
		add("python3")
	case "docker":
		if normalizedManager == "apt-get" || normalizedManager == "dpkg" {
			add("docker.io")
		}
	case "ip":
		switch normalizedManager {
		case "dnf", "yum", "rpm":
			add("iproute")
		default:
			add("iproute2")
		}
	case "dig", "host", "nslookup":
		switch normalizedManager {
		case "dnf", "yum", "rpm", "zypper":
			add("bind-utils")
		case "apt-get", "dpkg":
			add("dnsutils")
		case "apk":
			add("bind-tools")
		case "pacman", "brew":
			add("bind")
		}
	case "nmcli":
		switch normalizedManager {
		case "dnf", "yum", "rpm", "zypper":
			add("NetworkManager")
		case "apt-get", "dpkg":
			add("network-manager")
		default:
			add("networkmanager")
		}
	}

	seen := map[string]struct{}{}
	deduped := make([]string, 0, len(candidates))
	for _, value := range candidates {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		deduped = append(deduped, name)
	}
	return deduped
}

func queryInstalledPackage(packageManager, pkg string) (string, bool) {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return "", false
	}

	switch packageManager {
	case "dnf", "yum", "rpm", "zypper":
		out, err := runShortCommand("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}.%{ARCH}", pkg)
		if err != nil || strings.Contains(strings.ToLower(out), "not installed") {
			return "", false
		}
		return strings.TrimSpace(out), true
	case "apt-get", "dpkg":
		out, err := runShortCommand("dpkg-query", "-W", "-f", "${Status}\t${Version}", pkg)
		if err != nil {
			return "", false
		}
		lower := strings.ToLower(out)
		if !strings.Contains(lower, "install ok installed") {
			return "", false
		}
		parts := strings.Split(strings.TrimSpace(out), "\t")
		if len(parts) < 2 {
			return "installed", true
		}
		return strings.TrimSpace(parts[len(parts)-1]), true
	case "pacman":
		out, err := runShortCommand("pacman", "-Q", pkg)
		if err != nil {
			return "", false
		}
		fields := strings.Fields(out)
		if len(fields) >= 2 {
			return fields[1], true
		}
		return "installed", true
	case "apk":
		out, err := runShortCommand("apk", "info", "-e", pkg)
		if err != nil {
			return "", false
		}
		if strings.TrimSpace(out) == "" {
			return "", false
		}
		return "installed", true
	case "brew":
		out, err := runShortCommand("brew", "list", "--versions", pkg)
		if err != nil || strings.TrimSpace(out) == "" {
			return "", false
		}
		fields := strings.Fields(out)
		if len(fields) >= 2 {
			return fields[len(fields)-1], true
		}
		return "installed", true
	default:
		return "", false
	}
}

func discoverOSPrettyName() string {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return safeCommandOutput("uname", "-s")
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "PRETTY_NAME=") {
			continue
		}
		value := strings.TrimPrefix(line, "PRETTY_NAME=")
		value = strings.Trim(value, "\"")
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return safeCommandOutput("uname", "-s")
}

func safeCommandOutput(name string, args ...string) string {
	out, err := runShortCommand(name, args...)
	if err != nil {
		return "unknown"
	}
	clean := strings.TrimSpace(out)
	if clean == "" {
		return "unknown"
	}
	return clean
}

func runShortCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hostProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
