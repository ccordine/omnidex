package cognitionpolicy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	CallAttemptSchemaV3 = "omnidex.cognition-policy-call-attempt.v3"
	CallResultSchemaV3  = "omnidex.cognition-policy-call-result.v3"
	MaxCallFailureBytes = 4096
)

type CallResultStatus string

const (
	CallResultAccepted CallResultStatus = "accepted"
	CallResultRejected CallResultStatus = "rejected"
	CallResultFailed   CallResultStatus = "failed"
)

type CallFailureCode string

const (
	CallFailureGeneration         CallFailureCode = "generation_error"
	CallFailureProviderIdentity   CallFailureCode = "provider_identity_error"
	CallFailureResponseLimit      CallFailureCode = "response_limit"
	CallFailureInvalidDecision    CallFailureCode = "invalid_decision"
	CallFailureAuthorityDenied    CallFailureCode = "authority_denied"
	CallFailureProviderUsage      CallFailureCode = "provider_usage_error"
	CallFailureProviderUsageLimit CallFailureCode = "provider_usage_limit"
	CallFailureProviderRequest    CallFailureCode = "provider_request_mismatch"
	CallFailurePolicyAuthority    CallFailureCode = "policy_authority_error"
	CallFailureProviderEvidence   CallFailureCode = "provider_evidence_invalid"
)

type CallAttempt struct {
	Schema                        string                             `json:"schema"`
	ID                            string                             `json:"id"`
	Actor                         cognition.AttemptRef               `json:"actor"`
	SnapshotSHA256                string                             `json:"snapshot_sha256"`
	ExpectedRevision              cognition.WorldRevision            `json:"expected_revision"`
	ObligationID                  cognition.ObligationID             `json:"obligation_id"`
	RuntimeBudget                 cognition.RuntimeBudget            `json:"runtime_budget"`
	ContextProjection             cognition.ContextProjectionRef     `json:"context_projection"`
	Brain                         BrainRef                           `json:"brain"`
	ProviderAttestation           llm.ProviderIdentityAttestation    `json:"provider_attestation"`
	HostHardwareAttestation       HostHardwareAttestation            `json:"host_hardware_attestation"`
	ProviderProcessActivation     ProviderProcessActivationAuthority `json:"provider_process_activation"`
	EnvelopeRendererVersion       string                             `json:"envelope_renderer_version"`
	EnvelopeTokenEstimator        string                             `json:"envelope_token_estimator"`
	EnvelopeEstimatedTokens       int                                `json:"envelope_estimated_tokens"`
	EnvelopeSHA256                string                             `json:"envelope_sha256"`
	EnvelopeBytes                 int                                `json:"envelope_bytes"`
	Envelope                      string                             `json:"envelope"`
	PromptHint                    string                             `json:"prompt_hint"`
	PromptHintSHA256              string                             `json:"prompt_hint_sha256"`
	PromptHintBytes               int                                `json:"prompt_hint_bytes"`
	ModelVisibleInputSHA256       string                             `json:"model_visible_input_sha256"`
	ModelVisibleInputBytes        int                                `json:"model_visible_input_bytes"`
	ModelVisibleEstimatedTokens   int                                `json:"model_visible_estimated_tokens"`
	ModelInputTokenUpperBound     int                                `json:"model_input_token_upper_bound"`
	ResponseContractSHA256        string                             `json:"response_contract_sha256"`
	ExpectedProviderRequestSHA256 string                             `json:"expected_provider_request_sha256"`
}

type CallResult struct {
	Schema                        string                              `json:"schema"`
	CallID                        string                              `json:"call_id"`
	Status                        CallResultStatus                    `json:"status"`
	ProviderIdentityChecked       bool                                `json:"provider_identity_checked"`
	ProviderAttestation           llm.ProviderIdentityAttestation     `json:"provider_attestation"`
	ProviderObservation           llm.ProviderIdentityObservation     `json:"provider_observation"`
	ProviderIdentityEvidence      llm.ProviderIdentityEvidenceRef     `json:"provider_identity_evidence"`
	ProviderRequestDisposition    llm.ProviderRequestDisposition      `json:"provider_request_disposition"`
	ProviderRequestSHA256         string                              `json:"provider_request_sha256,omitempty"`
	ProviderHTTPStatus            int                                 `json:"provider_http_status"`
	ProviderResponseDisposition   llm.ProviderResponseDisposition     `json:"provider_response_disposition,omitempty"`
	ProviderResponseComplete      bool                                `json:"provider_response_complete"`
	ProviderContentEncoding       llm.ProviderContentEncodingEvidence `json:"provider_content_encoding"`
	ProviderResponseBytesKnown    bool                                `json:"provider_response_bytes_known"`
	ProviderResponseSHA256        string                              `json:"provider_response_sha256,omitempty"`
	ProviderResponseBytes         int64                               `json:"provider_response_bytes"`
	ProviderResponseCaptureSHA256 string                              `json:"provider_response_capture_sha256,omitempty"`
	ProviderResponseCapturedBytes int                                 `json:"provider_response_captured_bytes"`
	ProviderResponseModel         string                              `json:"provider_response_model"`
	ProviderDonePresent           bool                                `json:"provider_done_present"`
	ProviderDone                  bool                                `json:"provider_done"`
	ProviderDoneReason            string                              `json:"provider_done_reason"`
	ProviderUsagePresent          bool                                `json:"provider_usage_present"`
	ProviderUsage                 llm.ProviderGenerationUsage         `json:"provider_usage"`
	ProviderResponseCapture       ProviderResponseCaptureEvidenceRef  `json:"provider_response_capture_evidence"`
	ProviderGenerationEvidence    ProviderGenerationEvidenceRef       `json:"provider_generation_evidence"`
	ResponseSHA256                string                              `json:"response_sha256,omitempty"`
	ResponseBytes                 int                                 `json:"response_bytes"`
	ResponseEvidence              ModelResponseEvidenceRef            `json:"response_evidence"`
	ActionSchema                  cognition.ActionSchemaRef           `json:"action_schema,omitempty"`
	DecisionSHA256                string                              `json:"decision_sha256,omitempty"`
	FailureCode                   CallFailureCode                     `json:"failure_code,omitempty"`
	FailureMessage                string                              `json:"failure_message,omitempty"`
}

type CallReservation struct {
	Attempt                  CallAttempt            `json:"attempt"`
	ExistingResult           *CallResult            `json:"existing_result,omitempty"`
	ExistingResponseEvidence *ModelResponseEvidence `json:"existing_response_evidence,omitempty"`
	Created                  bool                   `json:"created"`
}

type CallJournal interface {
	Start(context.Context, CallAttempt) (CallReservation, error)
	Finish(context.Context, CallAttempt, CallResult, CallEvidence) error
}

func (reservation CallReservation) ValidateFor(attempt CallAttempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(reservation.Attempt, attempt) ||
		(reservation.Created && (reservation.ExistingResult != nil ||
			reservation.ExistingResponseEvidence != nil)) {
		return fmt.Errorf("%w: call reservation differs from the exact attempt", ErrInvalidEvidence)
	}
	if reservation.ExistingResult != nil {
		if err := reservation.ExistingResult.Validate(attempt); err != nil {
			return err
		}
		if reservation.ExistingResult.Status == CallResultAccepted {
			if reservation.ExistingResponseEvidence == nil ||
				reservation.ExistingResponseEvidence.Validate() != nil ||
				reservation.ExistingResponseEvidence.Ref != reservation.ExistingResult.ResponseEvidence {
				return fmt.Errorf("%w: call reservation lacks exact model response evidence", ErrInvalidEvidence)
			}
		} else if reservation.ExistingResponseEvidence != nil {
			return fmt.Errorf("%w: nonaccepted replay loaded model response content", ErrInvalidEvidence)
		}
	} else if reservation.ExistingResponseEvidence != nil {
		return fmt.Errorf("%w: call reservation has response evidence without a result", ErrInvalidEvidence)
	}
	return nil
}

func (attempt CallAttempt) Clone() CallAttempt { return attempt }

func (result CallResult) Clone() CallResult { return result }

func newCallAttempt(
	snapshot cognition.RuntimeSnapshot,
	brain AttestedBrain,
	activation ProviderProcessActivationAuthority,
	envelope RenderedEnvelope,
) (CallAttempt, error) {
	contractSHA, err := responseContractSHA(snapshot.ActionCatalog())
	if err != nil {
		return CallAttempt{}, err
	}
	promptHint := llm.MinimalGeneratePrompt
	modelInput, err := llm.ExactPreparedModelInput(envelope.JSON, promptHint)
	if err != nil {
		return CallAttempt{}, err
	}
	prepared, err := exactPreparedAuthorityModel(snapshot, brain.Ref, envelope.JSON)
	if err != nil {
		return CallAttempt{}, err
	}
	expectedRequestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return CallAttempt{}, err
	}
	attempt := CallAttempt{
		Schema: CallAttemptSchemaV3, Actor: snapshot.Attempt(),
		SnapshotSHA256: snapshot.SHA256(), ExpectedRevision: snapshot.CurrentRevision(),
		ObligationID: snapshot.CurrentObligation().ID, RuntimeBudget: snapshot.Budget(),
		ContextProjection: snapshot.ContextProjection(), Brain: brain.Ref,
		ProviderAttestation:       brain.Attestation,
		HostHardwareAttestation:   brain.Host,
		ProviderProcessActivation: activation,
		EnvelopeRendererVersion:   envelope.Version, EnvelopeTokenEstimator: envelope.TokenEstimator,
		EnvelopeEstimatedTokens: envelope.EstimatedTokens, EnvelopeSHA256: envelope.SHA256,
		EnvelopeBytes: envelope.Bytes, Envelope: envelope.JSON,
		PromptHint: promptHint, PromptHintSHA256: policySHA256(promptHint),
		PromptHintBytes:               len(promptHint),
		ModelVisibleInputBytes:        len(modelInput),
		ModelVisibleEstimatedTokens:   estimatePolicyTokens(len(modelInput)),
		ResponseContractSHA256:        contractSHA,
		ExpectedProviderRequestSHA256: expectedRequestSHA,
	}
	attempt.ModelInputTokenUpperBound, err = llm.ModelInputTokenUpperBound(
		modelInput, brain.Ref.Sampling.InputSpecialTokenReserve,
	)
	if err != nil {
		return CallAttempt{}, err
	}
	attempt.ModelVisibleInputSHA256 = modelVisibleInputSHA(attempt)
	attempt.ID = callAttemptID(attempt)
	return attempt, attempt.Validate()
}

func callAttemptID(attempt CallAttempt) string {
	raw, err := json.Marshal(attempt)
	if err != nil {
		panic(fmt.Sprintf("marshal cognition call attempt identity: %v", err))
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var identity map[string]any
	if err := decoder.Decode(&identity); err != nil {
		panic(fmt.Sprintf("decode cognition call attempt identity: %v", err))
	}
	delete(identity, "id")
	canonical, err := canonicalPolicyJSON(identity)
	if err != nil {
		panic(fmt.Sprintf("canonicalize cognition call attempt identity: %v", err))
	}
	return "cognition_call_" + policySHA256(string(canonical))
}

func validPolicySHA256OrEmpty(value string) bool {
	return value == "" || validPolicySHA256(value)
}
