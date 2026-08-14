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
		exactRawBody(), `"response":"{}"`, `"response":`+strconv.Quote(typeScript), 1,
	)
	client := exactPreparedIdentityClient(t, expected, http.StatusOK, body, seen, captured)
	prepared := exactPreparedRequest(expected)
	prepared.Protocol = llm.ExactPreparedProtocolRawTextV1
	prepared.ResponseFormat = ""
	prepared.ResponseSchema = nil
	result, err := client.GeneratePreparedExact(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	request := string(captured["/api/generate"])
	if result.Protocol != llm.ExactPreparedProtocolRawTextV1 ||
		result.Content != typeScript || strings.Contains(request, `"format"`) ||
		!strings.Contains(request, `"raw":true`) {
		t.Fatalf("raw result=%+v request=%s", result, request)
	}
}

func TestGeneratePreparedExactRawTextSendsRegisteredAdvisoryTerminator(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	captured := make(map[string][]byte)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, exactRawBody(), make(map[string]int), captured,
	)
	prepared := exactPreparedRequest(expected)
	prepared.Protocol = llm.ExactPreparedProtocolRawTextV1
	prepared.ResponseFormat = ""
	prepared.ResponseSchema = nil
	prepared.RawTextStopSequence = llm.ExactPreparedObjectiveAdvisoryStopV1
	if _, err := client.GeneratePreparedExact(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	request := string(captured["/api/generate"])
	if !strings.Contains(request, `"stop":["\n<END_OBJECTIVE_ADVISORY_V1>"]`) {
		t.Fatalf("raw provider request omitted exact advisory terminator: %s", request)
	}
}

func TestGeneratePreparedExactRawTextRejectsSchemaBeforeProviderObservation(t *testing.T) {
	t.Parallel()
	expected := ollamaIdentityExpectation()
	seen := make(map[string]int)
	client := exactPreparedIdentityClient(
		t, expected, http.StatusOK, exactRawBody(), seen, make(map[string][]byte),
	)
	prepared := exactPreparedRequest(expected)
	prepared.Protocol = llm.ExactPreparedProtocolRawTextV1
	if _, err := client.GeneratePreparedExact(context.Background(), prepared); err == nil {
		t.Fatal("raw-text protocol accepted a structured response schema")
	}
	if len(seen) != 0 {
		t.Fatalf("invalid raw-text request reached provider endpoints: %#v", seen)
	}
}
