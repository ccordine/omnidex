package browserinference

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestContextRelevanceBrokerSendsOneExactPacketAndValidatesRawResult(t *testing.T) {
	broker := NewContextRelevanceBroker()
	session, err := broker.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(nil)
	input := contextRelevanceBrokerFixture(t)
	type outcome struct {
		decision assemblyline.ContextRelevanceSelectionDecision
		err      error
	}
	completed := make(chan outcome, 1)
	go func() {
		decision, err := broker.ExecuteContextRelevance(t.Context(), "exact-browser-model", input)
		completed <- outcome{decision: decision, err: err}
	}()

	packet, err := session.Next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if packet.Schema != ContextRelevanceJobSchemaV1 ||
		packet.Station != station.ContextRelevance ||
		packet.Model != "exact-browser-model" || packet.PromptHint == "" ||
		!strings.Contains(packet.Prompt, input.Authority.CandidateAuthorities[0].Content) ||
		strings.Contains(packet.Prompt, input.Authority.CandidateAuthorities[0].Namespace) ||
		strings.Contains(packet.Prompt, input.Authority.CandidateAuthorities[0].ContentSHA256) {
		t.Fatalf("packet=%#v", packet)
	}
	if err := session.Submit(ContextRelevanceSubmission{
		Schema: ContextRelevanceSubmissionSchemaV1,
		JobID:  packet.JobID, Model: packet.Model, RawResult: "CTX_1",
	}); err != nil {
		t.Fatal(err)
	}
	result := <-completed
	if result.err != nil || result.decision.CandidateID != "CTX_1" {
		t.Fatalf("result=%#v", result)
	}
}

func TestContextRelevanceBrokerFailsWithoutAReadyBrowser(t *testing.T) {
	broker := NewContextRelevanceBroker()
	_, err := broker.ExecuteContextRelevance(
		t.Context(), "exact-browser-model", contextRelevanceBrokerFixture(t),
	)
	if !errors.Is(err, ErrNoBrowserWorker) {
		t.Fatalf("error=%v", err)
	}
}

func TestContextRelevanceBrokerRejectsInvalidBrowserResultWithoutFallback(t *testing.T) {
	broker := NewContextRelevanceBroker()
	session, err := broker.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(nil)
	completed := make(chan error, 1)
	go func() {
		_, err := broker.ExecuteContextRelevance(
			t.Context(), "exact-browser-model", contextRelevanceBrokerFixture(t),
		)
		completed <- err
	}()
	packet, err := session.Next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	submitErr := session.Submit(ContextRelevanceSubmission{
		Schema: ContextRelevanceSubmissionSchemaV1,
		JobID:  packet.JobID, Model: packet.Model, RawResult: "CTX_99",
	})
	if submitErr == nil || !strings.Contains(submitErr.Error(), "unknown candidate") {
		t.Fatalf("submit error=%v", submitErr)
	}
	if executeErr := <-completed; executeErr == nil ||
		!strings.Contains(executeErr.Error(), "unknown candidate") {
		t.Fatalf("execute error=%v", executeErr)
	}
}

func TestContextRelevanceBrokerDisconnectFailsPendingInference(t *testing.T) {
	broker := NewContextRelevanceBroker()
	session, err := broker.Connect()
	if err != nil {
		t.Fatal(err)
	}
	completed := make(chan error, 1)
	go func() {
		_, err := broker.ExecuteContextRelevance(
			context.Background(), "exact-browser-model", contextRelevanceBrokerFixture(t),
		)
		completed <- err
	}()
	if _, err := session.Next(t.Context()); err != nil {
		t.Fatal(err)
	}
	disconnected := errors.New("browser socket closed")
	session.Close(disconnected)
	if err := <-completed; !errors.Is(err, disconnected) {
		t.Fatalf("error=%v", err)
	}
}

func TestContextRelevanceBrokerAllowsOnlyOneBrowserConnection(t *testing.T) {
	broker := NewContextRelevanceBroker()
	if broker.Ready() {
		t.Fatal("broker reported ready before a browser session connected")
	}
	session, err := broker.Connect()
	if err != nil {
		t.Fatal(err)
	}
	if !broker.Ready() {
		t.Fatal("broker did not report its connected browser session")
	}
	if _, err := broker.Connect(); !errors.Is(err, ErrBrowserWorkerConnected) {
		t.Fatalf("error=%v", err)
	}
	session.Close(nil)
	if broker.Ready() {
		t.Fatal("broker remained ready after its browser session closed")
	}
}

func TestContextRelevanceSubmissionFrameAllowsEscapedBoundedRawResult(t *testing.T) {
	raw := strings.Repeat("<", MaxContextRelevanceResultBytes)
	submission := ContextRelevanceSubmission{
		Schema: ContextRelevanceSubmissionSchemaV1,
		JobID:  "bcr_0123456789abcdef0123456789abcdef", Model: "exact-browser-model",
		RawResult: raw,
	}
	packet := ContextRelevanceJob{JobID: submission.JobID, Model: submission.Model}
	if err := validateContextRelevanceSubmission(submission, packet); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > MaxContextRelevanceSubmissionBytes {
		t.Fatalf("escaped frame bytes=%d limit=%d", len(encoded), MaxContextRelevanceSubmissionBytes)
	}
}

func contextRelevanceBrokerFixture(t *testing.T) assemblyline.ContextRelevanceSelectionInput {
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
