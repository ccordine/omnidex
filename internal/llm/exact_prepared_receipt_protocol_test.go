package llm

import (
	"strings"
	"testing"
)

func TestPreparedGenerationReceiptRequiresExplicitRegisteredProtocol(t *testing.T) {
	valid := preparedGenerationProtocolReceiptFixture(t)
	if err := valid.ValidateProviderResponseReceipt(); err != nil {
		t.Fatal(err)
	}
	for name, protocol := range map[string]ExactPreparedProtocol{
		"missing": "",
		"unknown": "unknown",
	} {
		name, protocol := name, protocol
		t.Run(name, func(t *testing.T) {
			changed := valid
			changed.Protocol = protocol
			if err := changed.ValidateProviderResponseReceipt(); err == nil {
				t.Fatal("receipt without explicit registered protocol was accepted")
			}
		})
	}
}

func preparedGenerationProtocolReceiptFixture(t *testing.T) PreparedGeneration {
	t.Helper()
	body := exactProtocolResponseBody(t, `{}`)
	return PreparedGeneration{
		Schema: PreparedGenerationSchemaV1, Protocol: ExactPreparedProtocolStructuredV1,
		ProviderRequestDisposition:    ProviderRequestDispatched,
		ProviderRequestSHA256:         strings.Repeat("a", 64),
		ProviderHTTPStatus:            200,
		ProviderResponseDisposition:   ProviderResponseSucceeded,
		ProviderResponseComplete:      true,
		ProviderContentEncoding:       NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown:    true,
		ProviderResponseSHA256:        providerBodySHA256(body),
		ProviderResponseBytes:         int64(len(body)),
		ProviderResponseCaptureSHA256: providerBodySHA256(body),
		ProviderResponseCapturedBytes: len(body),
		ProviderResponseModel:         "qwen:9b",
		ProviderDonePresent:           true, ProviderDone: true, ProviderDoneReason: "stop",
		UsagePresent: true,
		Usage: ProviderGenerationUsage{
			PromptEvalCount: 41, EvalCount: 7, TotalDurationNanos: 101,
			LoadDurationNanos: 11, PromptEvalDurationNanos: 21, EvalDurationNanos: 31,
		},
	}
}
