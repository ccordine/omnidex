package cognitionpolicy

import (
	"encoding/json"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func policyTestPreparedGeneration(
	attempt CallAttempt,
	response string,
) llm.PreparedGeneration {
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil {
		panic(err)
	}
	challenge, err := callProviderObservationChallenge(attempt, expected)
	if err != nil {
		panic(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC), attempt.ProviderAttestation,
		challenge,
	)
	if err != nil {
		panic(err)
	}
	rawResponse := policyTestRawProviderResponse(attempt.Brain.Model, response)
	return llm.PreparedGeneration{
		Schema:                     llm.PreparedGenerationSchemaV1,
		ProviderRequestDisposition: llm.ProviderRequestDispatched,
		Content:                    response, ProviderRequestSHA256: attempt.ExpectedProviderRequestSHA256,
		ProviderHTTPStatus: 200, ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		ProviderResponseComplete:      true,
		ProviderContentEncoding:       llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown:    true,
		ProviderResponseSHA256:        policySHA256(string(rawResponse)),
		ProviderResponseBytes:         int64(len(rawResponse)),
		ProviderResponseCaptureSHA256: policySHA256(string(rawResponse)),
		ProviderResponseCapturedBytes: len(rawResponse),
		ProviderResponseCapture:       rawResponse,
		ProviderResponseModel:         attempt.Brain.Model,
		ProviderDonePresent:           true,
		ProviderDone:                  true,
		ProviderDoneReason:            "stop",
		UsagePresent:                  true, Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: 10, EvalCount: 1, TotalDurationNanos: 10,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 2, EvalDurationNanos: 3,
		}, ProviderObservation: observed.Observation,
		ProviderIdentityEvidence: observed.Evidence,
	}
}

func mustPolicyTestRequestSHA(prepared llm.PreparedModel) string {
	value, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		panic(err)
	}
	return value
}

func policyTestRawProviderResponse(model, response string) []byte {
	generation := llm.PreparedGeneration{
		Content: response, ProviderResponseModel: model,
		ProviderDonePresent: true, ProviderDone: true, ProviderDoneReason: "stop",
		UsagePresent: true, Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: 10, EvalCount: 1, TotalDurationNanos: 10,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 2, EvalDurationNanos: 3,
		},
	}
	return policyTestRawProviderResponseForGeneration(generation, true)
}

func policyTestRefreshRawProviderResponse(generation *llm.PreparedGeneration, includeEval bool) {
	raw := policyTestRawProviderResponseForGeneration(*generation, includeEval)
	generation.ProviderResponseSHA256 = policySHA256(string(raw))
	generation.ProviderResponseBytes = int64(len(raw))
	generation.ProviderResponseCaptureSHA256 = generation.ProviderResponseSHA256
	generation.ProviderResponseCapturedBytes = len(raw)
	generation.ProviderResponseCapture = raw
}

func policyTestRawProviderResponseForGeneration(
	generation llm.PreparedGeneration,
	includeEval bool,
) []byte {
	var evalCount *int
	var done *bool
	if includeEval {
		evalCount = &generation.Usage.EvalCount
	}
	if generation.ProviderDonePresent {
		done = &generation.ProviderDone
	}
	raw, err := json.Marshal(struct {
		Model              string `json:"model"`
		CreatedAt          string `json:"created_at"`
		Response           string `json:"response"`
		Done               *bool  `json:"done,omitempty"`
		DoneReason         string `json:"done_reason"`
		TotalDuration      int64  `json:"total_duration"`
		LoadDuration       int64  `json:"load_duration"`
		PromptEvalCount    int    `json:"prompt_eval_count"`
		PromptEvalDuration int64  `json:"prompt_eval_duration"`
		EvalCount          *int   `json:"eval_count,omitempty"`
		EvalDuration       int64  `json:"eval_duration"`
	}{
		generation.ProviderResponseModel, "2026-08-09T22:00:00Z",
		generation.Content, done,
		generation.ProviderDoneReason, generation.Usage.TotalDurationNanos,
		generation.Usage.LoadDurationNanos, generation.Usage.PromptEvalCount,
		generation.Usage.PromptEvalDurationNanos, evalCount, generation.Usage.EvalDurationNanos,
	})
	if err != nil {
		panic(err)
	}
	return raw
}

func policyTestFailedGeneration(attempt CallAttempt) llm.PreparedGeneration {
	generation := policyTestPreparedGeneration(attempt, "ignored")
	generation.Content = ""
	generation.ProviderHTTPStatus = 0
	generation.ProviderResponseDisposition = llm.ProviderResponseTransportError
	generation.ProviderResponseComplete = false
	generation.ProviderContentEncoding = llm.ProviderContentEncodingEvidence{}
	generation.ProviderResponseBytesKnown = false
	generation.ProviderResponseSHA256 = ""
	generation.ProviderResponseBytes = 0
	generation.ProviderResponseCaptureSHA256 = ""
	generation.ProviderResponseCapturedBytes = 0
	generation.ProviderResponseCapture = nil
	generation.ProviderResponseModel = ""
	generation.ProviderDonePresent = false
	generation.ProviderDone = false
	generation.ProviderDoneReason = ""
	generation.UsagePresent = false
	generation.Usage = llm.ProviderGenerationUsage{}
	return generation
}
