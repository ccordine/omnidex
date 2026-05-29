package ollama

import (
	"net"
	"testing"
)

func TestNormalizeBaseURLStripsTrailingDotHost(t *testing.T) {
	got := NormalizeBaseURL("http://172.20.0.1.:11434")
	want := "http://172.20.0.1:11434"
	if got != want {
		t.Fatalf("NormalizeBaseURL()=%q want %q", got, want)
	}
}

func TestCandidateBaseURLsDedupesAndIncludesFallbacks(t *testing.T) {
	candidates := CandidateBaseURLs("http://172.20.0.1:11434")
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

func TestOllamaPortFromURL(t *testing.T) {
	if got := ollamaPort("http://127.0.0.1:22434"); got != "22434" {
		t.Fatalf("ollamaPort()=%q want 22434", got)
	}
}

func TestDockerDefaultGatewayIPOptional(t *testing.T) {
	_ = dockerDefaultGatewayIP()
	_ = net.ParseIP("172.17.0.1")
}
