package dockerhost

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestNormalizeBaseURLStripsTrailingDotHost(t *testing.T) {
	got := NormalizeBaseURL("http://172.20.0.1.:11434")
	want := "http://172.20.0.1:11434"
	if got != want {
		t.Fatalf("NormalizeBaseURL()=%q want %q", got, want)
	}
}

func TestCandidateHostURLsDedupesAndIncludesFallbacks(t *testing.T) {
	candidates := CandidateHostURLs("http://172.20.0.1:11434", "", "11434")
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	if candidates[0] != "http://172.20.0.1:11434" {
		t.Fatalf("primary first=%q", candidates[0])
	}
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			t.Fatalf("duplicate candidate %q", candidate)
		}
		seen[candidate] = struct{}{}
	}
	foundHostGateway := false
	for _, candidate := range candidates {
		if candidate == "http://host.docker.internal:11434" {
			foundHostGateway = true
		}
	}
	if !foundHostGateway {
		t.Fatalf("expected host.docker.internal fallback in %#v", candidates)
	}
}

func TestParseRouteHexIP(t *testing.T) {
	ip := parseRouteHexIP("010011AC")
	if ip == nil || ip.String() != "172.17.0.1" {
		t.Fatalf("parseRouteHexIP()=%v want 172.17.0.1", ip)
	}
}

func TestServicePortFromURL(t *testing.T) {
	if got := servicePort("http://127.0.0.1:22434", "11434"); got != "22434" {
		t.Fatalf("servicePort()=%q want 22434", got)
	}
}

func TestDefaultGatewayIPOptional(t *testing.T) {
	_ = DefaultGatewayIP()
	_ = net.ParseIP("172.17.0.1")
}

func TestFirewallHintTimeout(t *testing.T) {
	hint := FirewallHint(fmt.Errorf("Get \"http://172.20.0.1:11434/api/tags\": context deadline exceeded"), []int{11434})
	if hint == "" {
		t.Fatal("expected firewall hint for timeout error")
	}
	if !strings.Contains(hint, "ufw-docker-host") {
		t.Fatalf("expected script mention in hint: %q", hint)
	}
}
