package ollama

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestGeneratePreparedExactRawTextReturnsTypeScriptWithoutFormat(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	seen := make(map[string]int)
	captured := make(map[string][]byte)
	typeScript := "export const answer: number = 42;\n"
	body := strings.Replace(
		exactRawBody(), `"response":"semantic leaf"`, `"response":`+strconv.Quote(typeScript), 1,
	)
	client := exactPreparedIdentityClient(t, expected, http.StatusOK, body, seen, captured)
	prepared := exactPreparedRequest(expected)
	prepared.RawTextStopSequence = llm.ExactPreparedRawChatEndV1
	result, err := client.GeneratePreparedExact(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	request := string(captured["/api/generate"])
	if result.Protocol != llm.ExactPreparedProtocolRawTextV2 ||
		result.Content != typeScript || strings.Contains(request, `"format"`) ||
		!strings.Contains(request, `"raw":true`) {
		t.Fatalf("raw result=%+v request=%s", result, request)
	}
}

func TestGeneratePreparedExactDispatchesMeasuredCorrectionEnvelopeWithoutByteTokenGuess(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	expected.NativeContextLimit = 8192
	seen := make(map[string]int)
	captured := make(map[string][]byte)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, exactRawBody(), seen, captured,
	)
	prepared := exactPreparedRequest(expected)
	prepared.MaxOutputTokens = 2048
	const measuredRawInputBytes = 6485
	promptBytes := measuredRawInputBytes - len(llm.ExactPreparedPromptJoiner) - len(llm.MinimalGeneratePrompt)
	prepared.Prompt = strings.Repeat("x", promptBytes)

	if _, err := client.GeneratePreparedExact(context.Background(), prepared); err != nil {
		t.Fatalf("measured correction envelope did not reach provider: %v", err)
	}
	// Exact provider identity performs one fixed preload request before the one
	// raw workload generation request.
	if seen["/api/generate"] != 2 {
		t.Fatalf("provider /api/generate calls=%d want preload+workload", seen["/api/generate"])
	}
	request := string(captured["/api/generate"])
	for _, required := range []string{
		`"num_ctx":8192`, `"num_predict":2048`, `"raw":true`, `"truncate":false`,
	} {
		if !strings.Contains(request, required) {
			t.Fatalf("provider request omitted %s", required)
		}
	}
}

func TestGeneratePreparedExactRawTextRejectsUnknownProtocolBeforeProviderObservation(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	seen := make(map[string]int)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, exactRawBody(), seen, make(map[string][]byte),
	)
	prepared := exactPreparedRequest(expected)
	prepared.Protocol = "unknown"
	if _, err := client.GeneratePreparedExact(context.Background(), prepared); err == nil {
		t.Fatal("raw-text transport accepted an unknown protocol")
	}
	if len(seen) != 0 {
		t.Fatalf("invalid raw-text request reached provider endpoints: %#v", seen)
	}
}
