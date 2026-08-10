package cognitiontransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestCompletionClientAndServerPreserveExactTypedResult(t *testing.T) {
	world := newTransportWorld(t)
	request := transportCompletionRequest(t, world)
	result, err := cognition.NewCompletionResult(
		request.Obligation.ID, request.Obligation.CompletionCheck, request.Revision,
		cognition.CompletionSatisfied, request.EvidenceRefs,
	)
	if err != nil {
		t.Fatal(err)
	}
	world.environment.completion = result
	client := transportCompletionClient(t, world.environment, "secret-token")

	actual, err := client.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Outcome != cognition.CompletionSatisfied ||
		actual.EvidenceRefs[0] != request.EvidenceRefs[0] || world.environment.evaluateCalls != 1 {
		t.Fatalf("completion=%#v calls=%d", actual, world.environment.evaluateCalls)
	}
}

func TestCompletionServerRejectsUnauthorizedMalformedAndOversizeBeforeEvaluator(t *testing.T) {
	world := newTransportWorld(t)
	handler, err := NewHandler(world.environment, world.environment, mustAuthenticator(t, "secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		body       string
		authorized bool
		want       int
	}{
		"unauthorized": {body: `{}`, want: http.StatusUnauthorized},
		"malformed":    {body: `{"protocol":"` + ProtocolVersionV1 + `","unknown":true}`, authorized: true, want: http.StatusBadRequest},
		"oversize":     {body: strings.Repeat("x", maxRequestBytes+1), authorized: true, want: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, evaluatePath, strings.NewReader(test.body))
			if test.authorized {
				request.Header.Set("Authorization", "Bearer secret-token")
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want || world.environment.evaluateCalls != 0 {
				t.Fatalf("status=%d calls=%d", recorder.Code, world.environment.evaluateCalls)
			}
		})
	}
}

func TestCompletionTransportPreservesStaleAuthorityAndRejectsAbsentSources(t *testing.T) {
	world := newTransportWorld(t)
	request := transportCompletionRequest(t, world)
	world.environment.evaluateErr = cognition.ErrAuthorityDenied
	client := transportCompletionClient(t, world.environment, "secret-token")
	if _, err := client.Evaluate(context.Background(), request); !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("stale authority error=%v", err)
	}

	world.environment.evaluateErr = nil
	absent := request.EvidenceRefs[0]
	absent.ObservationID = "observation-absent"
	absent.SHA256 = strings.Repeat("e", 64)
	result, err := cognition.NewCompletionResult(
		request.Obligation.ID, request.Obligation.CompletionCheck, request.Revision,
		cognition.CompletionSatisfied, []cognition.EvidenceRef{absent},
	)
	if err != nil {
		t.Fatal(err)
	}
	world.environment.completion = result
	if _, err := client.Evaluate(context.Background(), request); !errors.Is(err, ErrRemote) {
		t.Fatalf("absent evaluator source error=%v", err)
	}

	missingPacket := request
	missingPacket.EvidenceRefs = []cognition.EvidenceRef{}
	before := world.environment.evaluateCalls
	if _, err := client.Evaluate(context.Background(), missingPacket); !errors.Is(err, cognition.ErrInvalidEvidence) {
		t.Fatalf("missing request source error=%v", err)
	}
	if world.environment.evaluateCalls != before {
		t.Fatal("client sent completion request with absent supporting evidence")
	}
}

func transportCompletionClient(
	t *testing.T,
	environment *testEnvironment,
	token string,
) *Client {
	t.Helper()
	handler, err := NewHandler(environment, environment, mustAuthenticator(t, token))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://environment.invalid", token, &http.Client{
		Transport: handlerRoundTripper{handler: handler},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func transportCompletionRequest(
	t *testing.T,
	world transportWorld,
) cognitionruntime.CompletionRequest {
	t.Helper()
	predicate, err := cognition.NewPredicate("condition.complete", []string{"target"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	check := cognition.CompletionCheckRef{
		ID: "check.generic", Version: "1.0.0", SHA256: strings.Repeat("d", 64),
	}
	evidence := world.start.Observations[0].EvidenceRef()
	return cognitionruntime.CompletionRequest{
		Binding:        cognitionruntime.Binding{Episode: world.episode, Attempt: world.action.Actor},
		SnapshotSHA256: strings.Repeat("a", 64), Goal: goal, Revision: world.start.Current,
		Obligation: cognition.Obligation{
			ID: "obligation-root", Desired: goal, Status: cognition.ObligationActive,
			DependsOn: []cognition.ObligationID{}, SupportingRefs: []cognition.EvidenceRef{evidence},
			CompletionCheck: check, CreatedGeneration: 1,
		},
		EvidenceRefs: []cognition.EvidenceRef{evidence},
	}
}
