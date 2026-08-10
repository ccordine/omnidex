package cognitiontransport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

type testEnvironment struct {
	start         cognition.Transition
	apply         cognition.Transition
	failApply     bool
	startCalls    int
	applyCalls    int
	evaluateCalls int
	completion    cognition.CompletionResult
	evaluateErr   error
}

func (environment *testEnvironment) Evaluate(
	_ context.Context,
	_ cognitionruntime.CompletionRequest,
) (cognition.CompletionResult, error) {
	environment.evaluateCalls++
	return environment.completion.Clone(), environment.evaluateErr
}

func (environment *testEnvironment) Start(context.Context, cognition.ScenarioRef) (cognition.Transition, error) {
	environment.startCalls++
	return environment.start.Clone(), nil
}

func (environment *testEnvironment) Apply(
	_ context.Context,
	_ cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	environment.applyCalls++
	if environment.failApply {
		failure, err := cognition.NewActionFailure(
			cognition.ActionFailurePreconditionFailed, action, expected,
			"The registered precondition is not satisfied.", nil,
		)
		if err != nil {
			return cognition.Transition{}, err
		}
		return cognition.Transition{}, failure
	}
	return environment.apply.Clone(), nil
}

type handlerRoundTripper struct{ handler http.Handler }

func (transport handlerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	transport.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

func TestClientAndServerPreserveExactTransitionsAndTypedFailure(t *testing.T) {
	world := newTransportWorld(t)
	handler, err := NewHandler(world.environment, world.environment, mustAuthenticator(t, "secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://environment.invalid", "secret-token", &http.Client{
		Transport: handlerRoundTripper{handler: handler},
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	if started.Current != world.start.Current || world.environment.startCalls != 1 {
		t.Fatalf("start=%+v calls=%d", started, world.environment.startCalls)
	}
	applied, err := client.Apply(context.Background(), world.episode, started.Current, world.action)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Current != world.apply.Current || world.environment.applyCalls != 1 {
		t.Fatalf("apply=%+v calls=%d", applied, world.environment.applyCalls)
	}

	world.environment.failApply = true
	world.action.ID = "action-failure"
	_, err = client.Apply(context.Background(), world.episode, started.Current, world.action)
	if !errors.Is(err, cognition.ErrActionFailed) {
		t.Fatalf("typed failure=%v, want ErrActionFailed", err)
	}
	var failure cognition.ActionFailure
	if !errors.As(err, &failure) || failure.Code != cognition.ActionFailurePreconditionFailed {
		t.Fatalf("public failure=%+v", failure)
	}
}

func TestServerRejectsAuthenticationAndUnknownInputBeforeEnvironment(t *testing.T) {
	world := newTransportWorld(t)
	handler, err := NewHandler(world.environment, world.environment, mustAuthenticator(t, "secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, startPath, strings.NewReader(`{"protocol":"omnidex.cognition-environment-http.v1","unknown":true}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || world.environment.startCalls != 0 {
		t.Fatalf("unauthorized status=%d calls=%d", recorder.Code, world.environment.startCalls)
	}
	request = httptest.NewRequest(http.MethodPost, startPath, strings.NewReader(`{"protocol":"omnidex.cognition-environment-http.v1","unknown":true}`))
	request.Header.Set("Authorization", "Bearer secret-token")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || world.environment.startCalls != 0 {
		t.Fatalf("invalid request status=%d calls=%d", recorder.Code, world.environment.startCalls)
	}
}

func TestClientRejectsAmbiguousWireResult(t *testing.T) {
	world := newTransportWorld(t)
	failure, err := cognition.NewActionFailure(
		cognition.ActionFailureStaleRevision, world.action, world.start.Current,
		"The expected revision is stale.", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://environment.invalid", "secret-token", &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			body := `{"protocol":"` + ProtocolVersionV1 + `","transition":` + mustJSON(t, world.apply) + `,"failure":` + mustJSON(t, failure) + `}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Apply(context.Background(), world.episode, world.start.Current, world.action); !errors.Is(err, ErrInvalidWire) {
		t.Fatalf("ambiguous wire error=%v", err)
	}
}

func TestClientRejectsStatusAndRemoteErrorAmbiguity(t *testing.T) {
	world := newTransportWorld(t)
	client, err := NewClient("http://environment.invalid", "secret-token", &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			body := `{"protocol":"` + ProtocolVersionV1 + `","transition":` + mustJSON(t, world.start) + `}`
			return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Start(context.Background(), world.scenario); !errors.Is(err, ErrInvalidWire) {
		t.Fatalf("wrong status error=%v", err)
	}

	client.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := `{"protocol":"` + ProtocolVersionV1 + `","error":{"code":"","message":"missing code"}}`
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	if _, err := client.Start(context.Background(), world.scenario); !errors.Is(err, ErrInvalidWire) {
		t.Fatalf("invalid remote error=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
