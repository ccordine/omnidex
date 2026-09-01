package llm

import (
	"encoding/json"
	"testing"
)

func TestExactPreparedCompletionRequestIsBoundedAndNonStreaming(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()

	raw, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Raw      bool                       `json:"raw"`
		Shift    bool                       `json:"shift"`
		Stream   bool                       `json:"stream"`
		Think    *bool                      `json:"think"`
		Truncate bool                       `json:"truncate"`
		Options  map[string]json.RawMessage `json:"options"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"format", "system", "template", "suffix", "messages"} {
		if _, exists := fields[forbidden]; exists {
			t.Fatalf("completion request contains forbidden response/context field %q: %s", forbidden, raw)
		}
	}
	if got := string(request.Options["num_predict"]); got != "512" {
		t.Fatalf("num_predict = %s, want 512", got)
	}
	if request.Raw || request.Shift || request.Stream || request.Truncate ||
		request.Think == nil || *request.Think {
		t.Fatalf("completion request is not one bounded non-streaming response: %s", raw)
	}
	if got := request.Options["num_ctx"]; string(got) != "8192" {
		t.Fatalf("num_ctx = %s, want 8192", got)
	}
}

func TestExactPreparedRequestRejectsOutputBudgetConsumingNativeContext(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()
	prepared.MaxOutputTokens = prepared.ContextTokens
	if _, err := ExactPreparedRequestBytes(prepared); err == nil {
		t.Fatal("request accepted an output budget leaving no native input authority")
	}
}

func TestValidateExactPreparedNativeUsageEnforcesOutputBudget(t *testing.T) {
	t.Parallel()
	if err := ValidateExactPreparedNativeUsage(8192, 7680, 512, ProviderGenerationUsage{
		PromptEvalCount: 128,
		EvalCount:       512,
	}); err != nil {
		t.Fatalf("usage at the explicit output budget was rejected: %v", err)
	}
	if err := ValidateExactPreparedNativeUsage(8192, 7680, 512, ProviderGenerationUsage{
		PromptEvalCount: 128,
		EvalCount:       513,
	}); err == nil {
		t.Fatal("usage exceeding the explicit output budget was accepted")
	}
}

func exactPreparedRequestFixture() PreparedModel {
	temperature := ExactPreparedTemperature(0)
	return PreparedModel{
		Protocol:        ExactPreparedProtocolPlainCompletionV4,
		BaseModel:       "fixture-model",
		ContextModel:    "fixture-model",
		Prompt:          "What single semantic result follows?",
		MaxOutputTokens: 512,
		OutputLimitMode: ExactPreparedOutputLimitExplicit,
		ContextTokens:   MinInferenceContextTokens,
		Temperature:     &temperature,
	}
}
