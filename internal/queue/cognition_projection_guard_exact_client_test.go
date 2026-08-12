package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func (client cognitionGuardPolicyClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	if prepared.ProviderIdentityExpectation == nil {
		return llm.PreparedGeneration{}, fmt.Errorf("queue test exact generation lacks provider expectation")
	}
	attestation, err := llm.NewProviderIdentityAttestation(
		*prepared.ProviderIdentityExpectation,
		"queue-test:/version", "queue-test:/installed", "queue-test:/runner",
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	observed, err := queueTestObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond),
		attestation, prepared.ProviderObservationChallenge,
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	providerBody, err := json.Marshal(struct {
		Model              string `json:"model"`
		CreatedAt          string `json:"created_at"`
		Response           string `json:"response"`
		Done               bool   `json:"done"`
		DoneReason         string `json:"done_reason"`
		TotalDuration      int64  `json:"total_duration"`
		LoadDuration       int64  `json:"load_duration"`
		PromptEvalCount    int    `json:"prompt_eval_count"`
		PromptEvalDuration int64  `json:"prompt_eval_duration"`
		EvalCount          int    `json:"eval_count"`
		EvalDuration       int64  `json:"eval_duration"`
	}{
		prepared.ContextModel, "2026-08-09T22:00:00Z", client.response, true, "stop",
		1_000, 100, 100, 400, 20, 500,
	})
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	responseSHA := cognitionGuardSHA(providerBody)
	requestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	return llm.PreparedGeneration{
		Schema:                     llm.PreparedGenerationSchemaV1,
		ProviderRequestDisposition: llm.ProviderRequestDispatched,
		Content:                    client.response, ProviderRequestSHA256: requestSHA,
		ProviderHTTPStatus: 200, ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		ProviderResponseComplete: true, ProviderResponseBytesKnown: true,
		ProviderContentEncoding: llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseSHA256:  responseSHA,
		ProviderResponseBytes:   int64(len(providerBody)), ProviderResponseCaptureSHA256: responseSHA,
		ProviderResponseCapturedBytes: len(providerBody), ProviderResponseCapture: providerBody,
		ProviderResponseModel: prepared.ContextModel,
		ProviderDonePresent:   true, ProviderDone: true,
		ProviderDoneReason: "stop", UsagePresent: true,
		Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: 100, EvalCount: 20, TotalDurationNanos: 1_000,
			LoadDurationNanos: 100, PromptEvalDurationNanos: 400, EvalDurationNanos: 500,
		},
		ProviderObservation:      observed.Observation,
		ProviderIdentityEvidence: observed.Evidence,
	}, nil
}

func cognitionGuardSHA(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (cognitionGuardPolicyClient) ObserveProviderIdentity(
	_ context.Context,
	request llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	attestation, err := llm.NewProviderIdentityAttestation(
		request.Expectation,
		"queue-test:/version", "queue-test:/installed", "queue-test:/runner",
	)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	return queueTestObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, request.ChallengeSHA256,
	)
}
