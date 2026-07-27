package omni

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type routerRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn routerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestRouteToolsFailsWhenClientIsMissing(t *testing.T) {
	result, err := RouteTools(context.Background(), nil, DefaultRegistry(), "run a command")
	if err == nil {
		t.Fatal("expected missing router client to fail")
	}
	if len(result.SelectedTools) != 0 {
		t.Fatalf("expected no selected tools after failure, got %#v", result.SelectedTools)
	}
	var routerErr RouterError
	if !errors.As(err, &routerErr) || routerErr.Kind != RouterFailureClientUnavailable {
		t.Fatalf("expected typed client-unavailable error, got %T: %v", err, err)
	}
}

func TestRouteToolsFailsAfterMalformedRepairResponse(t *testing.T) {
	responses := []string{"not valid csv", "still invalid output"}
	calls := 0
	client := NewOllamaClient("http://router.invalid/api/chat", "router-test")
	client.Client = &http.Client{Transport: routerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		content := responses[calls]
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"message":{"role":"assistant","content":"` + content + `"},"done":true}`,
			)),
			Header: make(http.Header),
		}, nil
	})}

	result, err := RouteTools(context.Background(), client, DefaultRegistry(), "run a command")
	if err == nil {
		t.Fatal("expected malformed repaired router output to fail")
	}
	if calls != 2 {
		t.Fatalf("expected one bounded repair attempt, got %d calls", calls)
	}
	if len(result.SelectedTools) != 0 {
		t.Fatalf("expected no heuristic tool selection, got %#v", result.SelectedTools)
	}
	var routerErr RouterError
	if !errors.As(err, &routerErr) || routerErr.Kind != RouterFailureInvalidOutput || routerErr.Attempt != 2 {
		t.Fatalf("expected typed invalid-output error on attempt two, got %T: %v", err, err)
	}
}

func TestRouteToolsAcceptsCorrectedRepairResponse(t *testing.T) {
	responses := []string{"invalid output", "linux_command"}
	calls := 0
	client := NewOllamaClient("http://router.invalid/api/chat", "router-test")
	client.Client = &http.Client{Transport: routerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		content := responses[calls]
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"message":{"role":"assistant","content":"` + content + `"},"done":true}`,
			)),
			Header: make(http.Header),
		}, nil
	})}

	result, err := RouteTools(context.Background(), client, DefaultRegistry(), "run a command")
	if err != nil {
		t.Fatalf("expected corrected response to pass: %v", err)
	}
	if calls != 2 || result.Source != RouterSourceOllamaRetry {
		t.Fatalf("expected accepted repair response, calls=%d source=%q", calls, result.Source)
	}
	if len(result.SelectedTools) != 1 || result.SelectedTools[0] != "linux_command" {
		t.Fatalf("unexpected selected tools: %#v", result.SelectedTools)
	}
}

func TestExecuteDeterministicPipelineStopsWhenRoutingFails(t *testing.T) {
	nextID := 0
	response, events, err := ExecuteDeterministicPipeline(
		context.Background(),
		&Session{WorkspacePath: t.TempDir()},
		"run a command",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		DefaultRegistry(),
		nil,
		func() string {
			nextID++
			return "event-" + string(rune('0'+nextID))
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected routing failure to stop the pipeline")
	}
	if response != "" {
		t.Fatalf("expected no synthetic response after routing failure, got %q", response)
	}
	if len(events) != 1 || events[0].Type != "routing_failed" {
		t.Fatalf("expected one explicit routing_failed event, got %#v", events)
	}
}
