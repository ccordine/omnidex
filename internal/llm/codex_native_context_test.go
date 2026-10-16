package llm

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCodexCompletionUsesItsNativeWindowNotTheOllamaSetting(t *testing.T) {
	prepared := PreparedModel{Protocol: ExactPreparedProtocolCodexV1, BaseModel: "fixture-model", ContextModel: "fixture-model",
		Prompt: strings.Repeat("Necessary context. ", 10000), ContextTokens: 8192, MaxOutputTokens: -1, OutputLimitMode: ExactPreparedOutputLimitExplicit}
	if _, err := ExactPreparedRequestBytes(prepared); err != nil {
		t.Fatalf("prompt retained the former 128 KiB ceiling: %v", err)
	}
	raw := bytes.Replace(codexResponseFixture(t), []byte(`"inputTokens":40`), []byte(`"inputTokens":17000`), 1)
	raw = bytes.Replace(raw, []byte(`"totalTokens":52`), []byte(`"totalTokens":17012`), 1)
	response, err := DecodeExactPreparedResponseForProtocol(prepared.Protocol, 0, raw)
	if err != nil {
		t.Fatal(err)
	}
	generation := PreparedGeneration{Schema: PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: ProviderRequestDispatched, Content: response.Content,
		ProviderResponseDisposition: response.Disposition, ProviderResponseComplete: true, ProviderContentEncoding: ProviderEncodingIdentity,
		ProviderResponseBytesKnown: true, ProviderResponseBytes: int64(len(raw)), ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: raw,
		ProviderDonePresent: response.DonePresent, ProviderDone: response.Done, ProviderDoneReason: response.DoneReason,
		UsagePresent: response.UsagePresent, Usage: response.Usage}
	if err := ValidateExactPreparedGenerationForRequest(prepared, generation); err != nil {
		t.Fatalf("completed native call rejected by foreign context limit: %v", err)
	}
	for _, mutate := range []func(*PreparedGeneration){
		func(value *PreparedGeneration) { value.Usage.PromptEvalCount++ },
		func(value *PreparedGeneration) { value.ProviderResponseCapture = json.RawMessage(`{}`) },
	} {
		invalid := generation
		mutate(&invalid)
		if err := ValidateExactPreparedGenerationForRequest(prepared, invalid); err == nil {
			t.Fatal("invalid provider evidence accepted")
		}
	}
}
