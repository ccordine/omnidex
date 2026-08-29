package llm

import (
	"strings"
	"testing"
)

func TestDecodeExactPreparedResponseIgnoresProviderMetadataOutsideProjection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		baseline []byte
		withMeta []byte
	}{
		{
			name:     "Ministral native context and load metadata",
			baseline: exactProtocolResponseBody(t, "export function view() { return null; }"),
			withMeta: providerResponseMetadataFixture(
				exactProtocolResponseBody(t, "export function view() { return null; }"),
				`"context":[11,"provider-opaque",{"token":12}],"load_metadata":{"runner":"opaque"}`,
			),
		},
		{
			name:     "R1 native context and reasoning metrics",
			baseline: exactR1ResponseBody(t, "", "replacement source"),
			withMeta: providerResponseMetadataFixture(
				exactR1ResponseBody(t, "", "replacement source"),
				`"context":[null,true,[1,2,3]],"reasoning_metrics":{"passes":4}`,
			),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			want, err := DecodeExactPreparedResponseForProtocol(
				ExactPreparedProtocolRawTextV2, 200, test.baseline,
			)
			if err != nil {
				t.Fatal(err)
			}
			got, err := DecodeExactPreparedResponseForProtocol(
				ExactPreparedProtocolRawTextV2, 200, test.withMeta,
			)
			if err != nil {
				t.Fatalf("provider metadata blocked required response projection: %v", err)
			}
			if got != want {
				t.Fatalf("provider metadata entered normalized station state:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestDecodeExactPreparedResponseRejectsDuplicateProviderMetadataKeys(t *testing.T) {
	t.Parallel()
	body := providerResponseMetadataFixture(
		exactProtocolResponseBody(t, "candidate"),
		`"metadata":{"opaque":1,"opaque":2}`,
	)
	if _, err := DecodeExactPreparedResponseForProtocol(
		ExactPreparedProtocolRawTextV2, 200, body,
	); err == nil {
		t.Fatal("duplicate provider metadata keys were accepted")
	}
}

func TestDecodeExactPreparedResponseKeepsQwenRequiredProjectionFrozen(t *testing.T) {
	t.Parallel()
	body := []byte(`{"model":"qwen:9b","created_at":"2026-08-09T22:00:00Z",` +
		`"response":"semantic leaf","done":true,"done_reason":"stop",` +
		`"total_duration":101,"load_duration":11,"prompt_eval_count":41,` +
		`"prompt_eval_duration":21,"eval_count":7,"eval_duration":31}`)
	got, err := DecodeExactPreparedResponseForProtocol(
		ExactPreparedProtocolRawTextV2, 200, body,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := ExactPreparedResponse{
		Disposition:  ProviderResponseSucceeded,
		Model:        "qwen:9b",
		Content:      "semantic leaf",
		DonePresent:  true,
		Done:         true,
		DoneReason:   "stop",
		UsagePresent: true,
		Usage: ProviderGenerationUsage{
			PromptEvalCount: 41, EvalCount: 7, TotalDurationNanos: 101,
			LoadDurationNanos: 11, PromptEvalDurationNanos: 21, EvalDurationNanos: 31,
		},
	}
	if got != want {
		t.Fatalf("Qwen required response projection changed:\n got %+v\nwant %+v", got, want)
	}
}

func providerResponseMetadataFixture(body []byte, fields string) []byte {
	return []byte(strings.Replace(string(body), "{", "{"+fields+",", 1))
}
