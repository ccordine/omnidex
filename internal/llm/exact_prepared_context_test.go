package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExactPreparedContextPreservesNativeIDsOutsideSemanticPrompt(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`[0,17,17,32500]`, `[90,18,22,6,54]`} {
		prepared := exactPreparedRequestFixture()
		prepared.Prompt = "Which expression satisfies the required result?\n\nleft - right"
		context, err := DecodeExactPreparedRetainedContext(prepared.Protocol,
			[]byte(`{"response":"prior body", "context":`+raw+`}`), prepared.ContextTokens)
		if err != nil {
			t.Fatal(err)
		}
		prepared.RetainedContext = context
		request, err := ExactPreparedRequestBytes(prepared)
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Prompt               string `json:"prompt"`
			Context              []int  `json:"context"`
			Raw, Shift, Truncate bool
		}
		if err := json.Unmarshal(request, &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded.Context, context) || decoded.Prompt != prepared.Prompt ||
			decoded.Raw || decoded.Shift || decoded.Truncate {
			t.Fatalf("continued request changed native context or new question: %s", request)
		}
	}
	initial, err := ExactPreparedRequestBytes(exactPreparedRequestFixture())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(initial), `"context"`) {
		t.Fatalf("independent request inherited context: %s", initial)
	}
}

func TestExactPreparedContextRejectsMissingMalformedAndExcessiveNativeIDs(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"absent": `{}`, "null": `{"context":null}`, "empty": `{"context":[]}`,
		"string": `{"context":"[1]"}`, "object": `{"context":{}}`,
		"null token": `{"context":[1,null]}`, "string token": `{"context":["1"]}`,
		"boolean token": `{"context":[true]}`, "negative": `{"context":[-1]}`,
		"fraction": `{"context":[1.5]}`, "exponent": `{"context":[1e1]}`,
		"overflow": `{"context":[9223372036854775808]}`,
		"alias":    `{"Context":[1]}`, "duplicate": `{"context":[1],"context":[2]}`,
		"trailing": `{"context":[1]} {}`, "nested": `{"context":[[1]]}`,
		"too many": `{"context":[` + strings.Repeat("1,", MinInferenceContextTokens) + `1]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeExactPreparedRetainedContext(ExactPreparedProtocolPlainCompletionV4,
				[]byte(raw), MinInferenceContextTokens); err == nil {
				t.Fatal("invalid retained context was accepted")
			}
		})
	}
	for _, context := range [][]int{{}, {-1}, make([]int, MinInferenceContextTokens+1)} {
		prepared := exactPreparedRequestFixture()
		prepared.RetainedContext = context
		if _, err := ExactPreparedRequestBytes(prepared); err == nil {
			t.Fatal("invalid native context reached provider request rendering")
		}
	}
}

func TestExactPreparedContextRequestAllowsDeclaredMaximumWithoutTruncation(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()
	prepared.ContextTokens = MaxInferenceContextTokens
	prepared.RetainedContext = make([]int, prepared.ContextTokens)
	for index := range prepared.RetainedContext {
		prepared.RetainedContext[index] = 200_000
	}
	request, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if len(request) <= 1024*1024 || len(request) > MaxExactPreparedProviderRequestBytes {
		t.Fatalf("request bytes=%d", len(request))
	}
	var decoded struct {
		Context []int `json:"context"`
	}
	if err := json.Unmarshal(request, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Context, prepared.RetainedContext) {
		t.Fatal("maximum retained context was truncated")
	}
}
