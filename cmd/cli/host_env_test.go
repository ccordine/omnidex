package main

import "testing"

func TestPackageProbeCandidatesForTool(t *testing.T) {
	cases := []struct {
		manager string
		tool    string
		expect  string
	}{
		{manager: "apt-get", tool: "go", expect: "golang-go"},
		{manager: "dnf", tool: "go", expect: "golang"},
		{manager: "apt-get", tool: "docker", expect: "docker.io"},
		{manager: "apt-get", tool: "dig", expect: "dnsutils"},
		{manager: "dnf", tool: "dig", expect: "bind-utils"},
		{manager: "apk", tool: "dig", expect: "bind-tools"},
		{manager: "apt-get", tool: "nmcli", expect: "network-manager"},
	}

	for _, tc := range cases {
		candidates := packageProbeCandidatesForTool(tc.manager, tc.tool)
		found := false
		for _, candidate := range candidates {
			if candidate == tc.expect {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("manager=%s tool=%s expected package candidate %q not found (got=%v)", tc.manager, tc.tool, tc.expect, candidates)
		}
	}
}

func TestBuildInstalledPackageProbeListIncludesToolDrivenPackages(t *testing.T) {
	candidates := buildInstalledPackageProbeList("apt-get", []string{"go", "docker", "dig"})
	expect := []string{"golang-go", "docker.io", "dnsutils"}
	for _, target := range expect {
		found := false
		for _, candidate := range candidates {
			if candidate == target {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q in candidate list %v", target, candidates)
		}
	}
}
