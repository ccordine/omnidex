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

func TestExactPreparedRequestAcceptsOutputBudgetEqualToNativeContext(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()
	prepared.MaxOutputTokens = prepared.ContextTokens
	raw, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatalf("num_predict equal to num_ctx was rejected: %v", err)
	}
	var request struct {
		Options map[string]json.RawMessage `json:"options"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	want := "8192"
	if got := string(request.Options["num_predict"]); got != want {
		t.Fatalf("num_predict = %s, want %s", got, want)
	}
	if got := string(request.Options["num_ctx"]); got != want {
		t.Fatalf("num_ctx = %s, want %s", got, want)
	}
}

func TestExactPreparedRequestUsesProviderNativeUnlimitedNumPredict(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()
	prepared.MaxOutputTokens = -1
	raw, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Options map[string]json.RawMessage `json:"options"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	if got := string(request.Options["num_predict"]); got != "-1" {
		t.Fatalf("num_predict=%s; want -1", got)
	}
}

func TestValidateExactPreparedNativeUsageEnforcesProviderTokenAuthorities(t *testing.T) {
	t.Parallel()
	if err := ValidateExactPreparedNativeUsage(8192, 512, ProviderGenerationUsage{
		PromptEvalCount: 7681,
		EvalCount:       511,
	}); err != nil {
		t.Fatalf("usage within aggregate native context was rejected: %v", err)
	}
	if err := ValidateExactPreparedNativeUsage(8192, 512, ProviderGenerationUsage{
		PromptEvalCount: 128,
		EvalCount:       513,
	}); err == nil {
		t.Fatal("usage exceeding the explicit output budget was accepted")
	}
	if err := ValidateExactPreparedNativeUsage(8192, 512, ProviderGenerationUsage{
		PromptEvalCount: 7681,
		EvalCount:       512,
	}); err == nil {
		t.Fatal("usage exceeding the aggregate native context was accepted")
	}
	if err := ValidateExactPreparedNativeUsage(8192, -1, ProviderGenerationUsage{
		PromptEvalCount: 1024,
		EvalCount:       4096,
	}); err != nil {
		t.Fatalf("provider-unlimited usage inside native context was rejected: %v", err)
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
