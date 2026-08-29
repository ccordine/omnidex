package websearch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fixtureTransport struct {
	responses map[string]fixtureResponse
	requests  []string
}

type fixtureResponse struct {
	status int
	body   string
}

type fixtureResolver struct {
	addresses map[string][]net.IPAddr
}

func (resolver fixtureResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if addresses, ok := resolver.addresses[host]; ok {
		return append([]net.IPAddr(nil), addresses...), nil
	}
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

func (transport *fixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	key := request.URL.String()
	transport.requests = append(transport.requests, key)
	fixture, ok := transport.responses[key]
	if !ok {
		return nil, fmt.Errorf("unexpected request %s", key)
	}
	return &http.Response{
		StatusCode: fixture.status,
		Body:       io.NopCloser(strings.NewReader(fixture.body)),
		Header:     make(http.Header),
		Request:    request,
	}, nil
}

func validConfig(transport http.RoundTripper, providers ...ProviderID) Config {
	return Config{
		Providers:                providers,
		Timeout:                  time.Second,
		PerDocumentBytes:         2_000,
		TotalDocumentBytes:       8_000,
		MaxCandidatesPerProvider: 4,
		MaxCandidates:            8,
		MaxDocuments:             4,
		MaxResponseBytes:         20_000,
		HTTPClient:               &http.Client{Transport: transport},
		Resolver:                 fixtureResolver{},
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "no providers", mutate: func(config *Config) { config.Providers = nil }, want: "at least one provider"},
		{name: "unknown provider", mutate: func(config *Config) { config.Providers = []ProviderID{"unknown"} }, want: "unsupported provider"},
		{name: "duplicate provider", mutate: func(config *Config) { config.Providers = []ProviderID{ProviderDuckDuckGo, ProviderDuckDuckGo} }, want: "duplicate provider"},
		{name: "zero timeout", mutate: func(config *Config) { config.Timeout = 0 }, want: "timeout"},
		{name: "too little total budget", mutate: func(config *Config) { config.TotalDocumentBytes = 2_000 }, want: "total document budget"},
		{name: "too few candidate slots", mutate: func(config *Config) { config.MaxCandidates = 1 }, want: "max candidates"},
		{name: "response bound below document bound", mutate: func(config *Config) { config.MaxResponseBytes = 1_000 }, want: "response byte bound"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validConfig(&fixtureTransport{}, ProviderDuckDuckGo, ProviderGoogle)
			test.mutate(&config)
			if _, err := New(config); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestServicePublishesExactAcquisitionLimits(t *testing.T) {
	config := validConfig(&fixtureTransport{}, ProviderDuckDuckGo)
	config.MaxDocuments = 2
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if limits := service.Limits(); limits.MaxDocuments != 2 {
		t.Fatalf("acquisition limits=%+v want max documents 2", limits)
	}
}

func TestDiscoverRunsEveryProviderAndDeduplicatesCanonicalURLs(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://duckduckgo.com/html/?q=ocean+circulation": {
			status: http.StatusOK,
			body:   `<a class="result__a" href="https://science.example/articles/current?utm_source=ddg&amp;b=2&amp;a=1">Ocean current</a>`,
		},
		"https://www.google.com/search?q=ocean+circulation": {
			status: http.StatusOK,
			body:   `<a href="/url?q=https%3A%2F%2Fscience.example%2Farticles%2Fcurrent%3Fa%3D1%26b%3D2">Current research</a>`,
		},
	}}
	service, err := New(validConfig(transport, ProviderDuckDuckGo, ProviderGoogle))
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Discover(context.Background(), QueryRequest{Query: "ocean circulation"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(transport.requests); got != 2 {
		t.Fatalf("provider requests=%d want 2", got)
	}
	if got := len(report.Diagnostics); got != 2 {
		t.Fatalf("diagnostics=%d want 2", got)
	}
	if got := len(report.Candidates); got != 1 {
		t.Fatalf("candidates=%d want 1: %#v", got, report.Candidates)
	}
	candidate := report.Candidates[0]
	if candidate.URL != "https://science.example/articles/current?a=1&b=2" {
		t.Fatalf("canonical URL=%q", candidate.URL)
	}
	if !strings.HasPrefix(string(candidate.ID), "candidate_") || strings.Contains(string(candidate.ID), "science") {
		t.Fatalf("candidate ID is not opaque: %q", candidate.ID)
	}
	if got := len(candidate.Sources); got != 2 {
		t.Fatalf("merged sources=%d want 2", got)
	}
	if report.Diagnostics[0].Outcome != DiscoverySucceeded || report.Diagnostics[1].Outcome != DiscoverySucceeded {
		t.Fatalf("unexpected diagnostics: %#v", report.Diagnostics)
	}
}

func TestDiscoverHasNoRawResultsPageFallback(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://duckduckgo.com/html/?q=compiler+diagnostics": {
			status: http.StatusOK,
			body:   `<html><body>This search response contains lots of text but no result records.</body></html>`,
		},
	}}
	config := validConfig(transport, ProviderDuckDuckGo)
	config.MaxCandidates = 4
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Discover(context.Background(), QueryRequest{Query: "compiler diagnostics"})
	if err == nil || !strings.Contains(err.Error(), "no candidates") {
		t.Fatalf("Discover error=%v want no candidates", err)
	}
	if len(report.Candidates) != 0 {
		t.Fatalf("raw search response became a candidate: %#v", report.Candidates)
	}
	if len(report.Diagnostics) != 1 || report.Diagnostics[0].Outcome != DiscoveryEmpty {
		t.Fatalf("diagnostics=%#v", report.Diagnostics)
	}
}

func TestDiscoverRejectsOversizedProviderFieldsBeforeNormalization(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://duckduckgo.com/html/?q=oversized": {
			status: http.StatusOK,
			body: `<a class="result__a" href="https://docs.example/oversized">` +
				strings.Repeat("x", maxCandidateTextBytes+1) + `</a>`,
		},
	}}
	service, err := New(validConfig(transport, ProviderDuckDuckGo))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Discover(context.Background(), QueryRequest{Query: "oversized"}); !errors.Is(err, ErrBoundExceeded) {
		t.Fatalf("Discover error=%v want ErrBoundExceeded", err)
	}
}

func TestDiscoverReturnsSuccessfulCandidatesAndFailedProviderDiagnostic(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://duckduckgo.com/html/?q=distributed+tracing": {
			status: http.StatusBadGateway,
			body:   "provider unavailable",
		},
		"https://www.google.com/search?q=distributed+tracing": {
			status: http.StatusOK,
			body:   `<a href="/url?q=https%3A%2F%2Fobservability.example%2Ftracing">Tracing guide</a>`,
		},
	}}
	service, err := New(validConfig(transport, ProviderDuckDuckGo, ProviderGoogle))
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Discover(context.Background(), QueryRequest{Query: "distributed tracing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 1 || len(report.Diagnostics) != 2 {
		t.Fatalf("report=%#v", report)
	}
	if report.Diagnostics[0].Outcome != DiscoveryFailed || report.Diagnostics[0].Failure == "" {
		t.Fatalf("provider failure was hidden: %#v", report.Diagnostics[0])
	}
}
