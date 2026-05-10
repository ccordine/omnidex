package main

import (
	"strings"
	"testing"
)

func TestDeriveHostCapabilities(t *testing.T) {
	snapshot := hostEnvironmentSnapshot{
		PackageManagers: []string{"dnf", "rpm"},
		AvailableTools:  []string{"bash", "touch", "mv", "git", "go", "docker", "vlc", "playerctl", "ip", "dig", "nmcli"},
	}

	got := deriveHostCapabilities(snapshot)
	joined := strings.Join(got, "\n")
	expectContains := []string{
		"local_shell.run_command",
		"local_shell.file_create_rename",
		"repo.inspect_and_diff",
		"repo.go_build_and_test",
		"container.build_and_compose_control",
		"media.playback_control_and_next_episode",
		"network.local_ip_and_open_ports_inspection",
		"network.dns_route_whois_scan_diagnostics",
		"network.vpn_detection_and_status",
		"system.package_install_via_dnf|rpm",
	}
	for _, expected := range expectContains {
		if !strings.Contains(joined, expected) {
			t.Fatalf("capabilities missing %q\nfull=%s", expected, joined)
		}
	}
}

func TestBuildHostCapabilityMemoryContent(t *testing.T) {
	snapshot := hostEnvironmentSnapshot{
		OS:              "linux",
		Arch:            "amd64",
		Distro:          "Fedora 41",
		PackageManager:  "dnf",
		PackageManagers: []string{"dnf", "rpm"},
		AvailableTools:  []string{"bash", "git"},
	}
	content := buildHostCapabilityMemoryContent(snapshot)
	if !strings.Contains(content, "package_managers=dnf,rpm") {
		t.Fatalf("expected package managers in content, got: %s", content)
	}
	if !strings.Contains(content, "actions:") {
		t.Fatalf("expected actions section, got: %s", content)
	}
}

func TestHostCapabilityTags(t *testing.T) {
	snapshot := hostEnvironmentSnapshot{
		OS:              "linux",
		PackageManagers: []string{"apt-get", "dpkg"},
	}
	tags := hostCapabilityTags(snapshot)
	joined := strings.Join(tags, ",")
	if !strings.Contains(joined, "os-linux") {
		t.Fatalf("expected os tag in %q", joined)
	}
	if !strings.Contains(joined, "pm-apt-get") {
		t.Fatalf("expected apt-get package manager tag in %q", joined)
	}
	if !strings.Contains(joined, "pm-dpkg") {
		t.Fatalf("expected dpkg package manager tag in %q", joined)
	}
}
