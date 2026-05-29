package ollama

import "testing"

func TestCandidateBaseURLsUsesSharedResolver(t *testing.T) {
	candidates := CandidateBaseURLs("http://172.20.0.1:11434")
	if len(candidates) == 0 || candidates[0] != "http://172.20.0.1:11434" {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestNormalizeBaseURLDelegates(t *testing.T) {
	got := NormalizeBaseURL("http://172.20.0.1.:11434")
	if got != "http://172.20.0.1:11434" {
		t.Fatalf("NormalizeBaseURL()=%q", got)
	}
}
