package main

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestParseStatusCSV(t *testing.T) {
	got := parseStatusCSV(" google,reddit,google, ,yahoo ")
	if len(got) != 3 {
		t.Fatalf("expected 3 unique providers, got %v", got)
	}
	if got[0] != "google" || got[1] != "reddit" || got[2] != "yahoo" {
		t.Fatalf("unexpected provider ordering/content: %v", got)
	}
}

func TestStatusProviderProbeURL(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{provider: "google", want: "https://www.google.com"},
		{provider: "yahoo", want: "https://search.yahoo.com"},
		{provider: "reddit", want: "https://www.reddit.com"},
		{provider: "https://example.com/", want: "https://example.com"},
		{provider: "example.org", want: "https://example.org"},
		{provider: "unknown-provider", want: ""},
	}

	for _, tc := range tests {
		if got := statusProviderProbeURL(tc.provider); got != tc.want {
			t.Fatalf("statusProviderProbeURL(%q)=%q, want %q", tc.provider, got, tc.want)
		}
	}
}

func TestWebStatusHasFailures(t *testing.T) {
	report := webStatusReport{
		Enabled: true,
		Probe:   true,
		Probes: []webProbeReport{
			{Provider: "google", StatusCode: 200},
			{Provider: "reddit", Error: "dial tcp timeout"},
		},
	}
	if !webStatusHasFailures(report) {
		t.Fatalf("expected failed probe to mark web status unhealthy")
	}

	report.Probes[1].Error = ""
	report.Probes[1].StatusCode = 302
	if webStatusHasFailures(report) {
		t.Fatalf("did not expect all successful probes to mark web status unhealthy")
	}

	report.Enabled = false
	if webStatusHasFailures(report) {
		t.Fatalf("disabled web status should not be treated as failure")
	}
}

func TestIsActiveJobStatus(t *testing.T) {
	if !isActiveJobStatus(model.JobStatusPending) {
		t.Fatalf("pending should be active")
	}
	if !isActiveJobStatus(model.JobStatusRunning) {
		t.Fatalf("running should be active")
	}
	if !isActiveJobStatus(model.JobStatusWaiting) {
		t.Fatalf("waiting_input should be active")
	}
	if isActiveJobStatus(model.JobStatusCompleted) {
		t.Fatalf("completed should not be active")
	}
}

func TestCompactStatusList(t *testing.T) {
	if got := compactStatusList([]string{"a", "b", "c"}, 2); got != "a|b|..." {
		t.Fatalf("compactStatusList=%q, want %q", got, "a|b|...")
	}
	if got := compactStatusList([]string{"a", "b"}, 5); got != "a|b" {
		t.Fatalf("compactStatusList=%q, want %q", got, "a|b")
	}
}
