package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/browserinference"
)

func TestBrowserContextRelevanceConfigIsDisabledWithoutProvider(t *testing.T) {
	server := NewServerWithOptions(nil, nil, ServerOptions{})
	request := httptest.NewRequest(http.MethodGet, "/v1/browser-inference/context-relevance", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var config browserContextRelevanceConfig
	if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if config.Schema != browserContextRelevanceConfigSchemaV1 || config.Enabled || config.Model != "" {
		t.Fatalf("config=%#v", config)
	}
}

func TestBrowserContextRelevanceWebSocketExecutesOneRealStationPacket(t *testing.T) {
	broker := browserinference.NewContextRelevanceBroker()
	server := NewServerWithOptions(nil, nil, ServerOptions{
		BrowserContextRelevance: broker,
		BrowserContextModel:     "exact-browser-model",
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/v1/browser-inference/context-relevance/ws"
	connection, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if response != nil {
			t.Fatalf("dial status=%d error=%v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	defer connection.Close()

	input := apiBrowserContextRelevanceFixture(t)
	type outcome struct {
		decision assemblyline.ContextRelevanceSelectionDecision
		err      error
	}
	completed := make(chan outcome, 1)
	go func() {
		decision, err := broker.ExecuteContextRelevance(t.Context(), "exact-browser-model", input)
		completed <- outcome{decision: decision, err: err}
	}()
	var packet browserinference.ContextRelevanceJob
	if err := connection.ReadJSON(&packet); err != nil {
		t.Fatal(err)
	}
	if err := connection.WriteJSON(browserinference.ContextRelevanceSubmission{
		Schema: browserinference.ContextRelevanceSubmissionSchemaV1,
		JobID:  packet.JobID, Model: packet.Model, RawResult: "CTX_1",
	}); err != nil {
		t.Fatal(err)
	}
	result := <-completed
	if result.err != nil || result.decision.CandidateID != "CTX_1" {
		t.Fatalf("result=%#v", result)
	}
}

func TestBrowserSubmissionReaderSendHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if sendBrowserSubmissionRead(ctx, make(chan browserSubmissionRead), browserSubmissionRead{}) {
		t.Fatal("browser submission send ignored cancellation")
	}
}

func apiBrowserContextRelevanceFixture(t *testing.T) assemblyline.ContextRelevanceSelectionInput {
	t.Helper()
	candidate, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_1", "The west gate was closed before dusk.",
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ContextRelevanceSelectionInput{
		Authority: assemblyline.ContextRelevanceInput{
			ExactInstruction: "Do that again.", RetrievalConcepts: []string{"previous gate action"},
			CandidateAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
			MaxSelections:        1,
		},
		AcceptedCandidateIDs: []string{},
	}
}
