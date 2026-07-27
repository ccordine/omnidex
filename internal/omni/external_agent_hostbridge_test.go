package omni

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/hostbridge"
)

type externalAgentRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn externalAgentRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestUseHostBridgeExternalAgents(t *testing.T) {
	t.Setenv("OMNI_EXTERNAL_AGENT_FORCE_LOCAL", "")
	t.Setenv("HOST_AGENT_URL", "")
	if UseHostBridgeExternalAgents() {
		t.Fatal("expected false without HOST_AGENT_URL")
	}

	t.Setenv("HOST_AGENT_URL", "http://127.0.0.1:8091")
	if !UseHostBridgeExternalAgents() {
		t.Fatal("expected true when HOST_AGENT_URL is set")
	}

	t.Setenv("OMNI_EXTERNAL_AGENT_FORCE_LOCAL", "true")
	if UseHostBridgeExternalAgents() {
		t.Fatal("expected false when OMNI_EXTERNAL_AGENT_FORCE_LOCAL=true")
	}
}

func TestHostBridgeExternalAgentSessionPropagatesMalformedStream(t *testing.T) {
	session := &hostBridgeExternalAgentSession{
		agent: "codex",
		client: &hostbridge.Client{
			BaseURL: "http://host-bridge.test",
			HTTPClient: &http.Client{Transport: externalAgentRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/v1/codex/run" {
					t.Fatalf("path=%q want /v1/codex/run", req.URL.Path)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/x-ndjson"}},
					Body:       io.NopCloser(strings.NewReader("{not-json}\n")),
					Request:    req,
				}, nil
			})},
		},
	}
	_, err := StreamExternalAgentSession(t.Context(), session, ExternalAgentJob{
		SessionID: "codex-session",
		Agent:     "codex",
		Prompt:    "bounded task",
		Workspace: t.TempDir(),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "decode host bridge agent event") {
		t.Fatalf("error=%v want malformed stream failure", err)
	}
}
