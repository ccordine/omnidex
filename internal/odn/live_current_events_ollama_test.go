package odn

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestLiveOllamaFindsCurrentEventsFromWebEvidence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live current-events search in short mode")
	}
	skipUnlessLiveOllamaEnabled(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecision(ctx, "What are the current events in Saipan?", client, stdout, stderr)
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live current-events test: %v", err)
		}
		t.Fatalf("current-events lookup failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
	if !hasSuccessfulCommandObservation(result.Observations) {
		t.Fatalf("expected successful command observation: %#v", result.Observations)
	}

	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	evidence := strings.ToLower(strings.Join([]string{stdout.String(), stderr.String(), result.Answer}, "\n"))
	if !strings.Contains(evidence, "saipan") {
		t.Fatalf("current-events evidence missing location\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s",
			result.Command, result.Answer, stdout.String(), stderr.String())
	}
}
