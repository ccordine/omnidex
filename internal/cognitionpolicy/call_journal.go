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
	CallAttemptSchemaV2 = "omnidex.cognition-policy-call-attempt.v2"
	CallResultSchemaV2  = "omnidex.cognition-policy-call-result.v2"
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
	CallFailureGeneration       CallFailureCode = "generation_error"
	CallFailureProviderIdentity CallFailureCode = "provider_identity_error"
	CallFailureResponseLimit    CallFailureCode = "response_limit"
	CallFailureInvalidDecision  CallFailureCode = "invalid_decision"
	CallFailureAuthorityDenied  CallFailureCode = "authority_denied"
)

type CallAttempt struct {
	Schema                  string                          `json:"schema"`
	ID                      string                          `json:"id"`
	Actor                   cognition.AttemptRef            `json:"actor"`
	SnapshotSHA256          string                          `json:"snapshot_sha256"`
	ExpectedRevision        cognition.WorldRevision         `json:"expected_revision"`
	ObligationID            cognition.ObligationID          `json:"obligation_id"`
	RuntimeBudget           cognition.RuntimeBudget         `json:"runtime_budget"`
	ContextProjection       cognition.ContextProjectionRef  `json:"context_projection"`
	Brain                   BrainRef                        `json:"brain"`
	ProviderAttestation     llm.ProviderIdentityAttestation `json:"provider_attestation"`
	HostHardwareAttestation HostHardwareAttestation         `json:"host_hardware_attestation"`
	EnvelopeRendererVersion string                          `json:"envelope_renderer_version"`
	EnvelopeTokenEstimator  string                          `json:"envelope_token_estimator"`
	EnvelopeEstimatedTokens int                             `json:"envelope_estimated_tokens"`
	EnvelopeSHA256          string                          `json:"envelope_sha256"`
	EnvelopeBytes           int                             `json:"envelope_bytes"`
	Envelope                string                          `json:"envelope"`
}

type CallResult struct {
	Schema                  string                          `json:"schema"`
	CallID                  string                          `json:"call_id"`
	Status                  CallResultStatus                `json:"status"`
	ProviderIdentityChecked bool                            `json:"provider_identity_checked"`
	ProviderAttestation     llm.ProviderIdentityAttestation `json:"provider_attestation"`
	ResponseStored          bool                            `json:"response_stored"`
	ResponseSHA256          string                          `json:"response_sha256,omitempty"`
	ResponseBytes           int                             `json:"response_bytes"`
	Response                string                          `json:"response,omitempty"`
	ActionSchema            cognition.ActionSchemaRef       `json:"action_schema,omitempty"`
	DecisionSHA256          string                          `json:"decision_sha256,omitempty"`
	FailureCode             CallFailureCode                 `json:"failure_code,omitempty"`
	FailureMessage          string                          `json:"failure_message,omitempty"`
}

type CallReservation struct {
	Attempt        CallAttempt `json:"attempt"`
	ExistingResult *CallResult `json:"existing_result,omitempty"`
	Created        bool        `json:"created"`
}

type CallJournal interface {
	Start(context.Context, CallAttempt) (CallReservation, error)
	Finish(context.Context, CallAttempt, CallResult) error
}

func (reservation CallReservation) ValidateFor(attempt CallAttempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(reservation.Attempt, attempt) ||
		(reservation.Created && reservation.ExistingResult != nil) {
		return fmt.Errorf("%w: call reservation differs from the exact attempt", ErrInvalidEvidence)
	}
	if reservation.ExistingResult != nil {
		if err := reservation.ExistingResult.Validate(attempt); err != nil {
			return err
		}
	}
	return nil
}

func (attempt CallAttempt) Clone() CallAttempt { return attempt }

func (result CallResult) Clone() CallResult { return result }

func newCallAttempt(
	snapshot cognition.RuntimeSnapshot,
	brain AttestedBrain,
	envelope RenderedEnvelope,
) CallAttempt {
	attempt := CallAttempt{
		Schema: CallAttemptSchemaV2, Actor: snapshot.Attempt(),
		SnapshotSHA256: snapshot.SHA256(), ExpectedRevision: snapshot.CurrentRevision(),
		ObligationID: snapshot.CurrentObligation().ID, RuntimeBudget: snapshot.Budget(),
		ContextProjection: snapshot.ContextProjection(), Brain: brain.Ref,
		ProviderAttestation:     brain.Attestation,
		HostHardwareAttestation: brain.Host,
		EnvelopeRendererVersion: envelope.Version, EnvelopeTokenEstimator: envelope.TokenEstimator,
		EnvelopeEstimatedTokens: envelope.EstimatedTokens, EnvelopeSHA256: envelope.SHA256,
		EnvelopeBytes: envelope.Bytes, Envelope: envelope.JSON,
	}
	attempt.ID = callAttemptID(attempt)
	return attempt
}

func (attempt CallAttempt) Validate() error {
	if attempt.Schema != CallAttemptSchemaV2 || attempt.ID != callAttemptID(attempt) {
		return fmt.Errorf("%w: call attempt identity is invalid", ErrInvalidEvidence)
	}
	if err := attempt.Actor.Validate(); err != nil {
		return fmt.Errorf("%w: actor: %v", ErrInvalidEvidence, err)
	}
	if !validPolicySHA256(attempt.SnapshotSHA256) || attempt.ExpectedRevision.EpisodeID == "" ||
		attempt.ObligationID == "" {
		return fmt.Errorf("%w: snapshot authority is invalid", ErrInvalidEvidence)
	}
	if err := attempt.ExpectedRevision.Validate(); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidEvidence, err)
	}
	if err := attempt.RuntimeBudget.Validate(); err != nil || attempt.RuntimeBudget.RemainingPolicyCalls == 0 {
		return fmt.Errorf("%w: runtime budget is invalid", ErrInvalidEvidence)
	}
	if err := attempt.ContextProjection.Validate(); err != nil {
		return fmt.Errorf("%w: projection: %v", ErrInvalidEvidence, err)
	}
	if err := attempt.Brain.Validate(); err != nil {
		return fmt.Errorf("%w: brain: %v", ErrInvalidEvidence, err)
	}
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil || attempt.ProviderAttestation.ValidateFor(expected) != nil {
		return fmt.Errorf("%w: live provider attestation is invalid", ErrInvalidEvidence)
	}
	if err := attempt.HostHardwareAttestation.Validate(); err != nil {
		return fmt.Errorf("%w: live host hardware attestation is invalid", ErrInvalidEvidence)
	}
	if attempt.EnvelopeRendererVersion != RendererVersionV2 ||
		attempt.EnvelopeTokenEstimator != policyTokenEstimator ||
		attempt.EnvelopeEstimatedTokens != estimatePolicyTokens(attempt.EnvelopeBytes) ||
		attempt.EnvelopeBytes != len(attempt.Envelope) ||
		attempt.EnvelopeBytes > attempt.RuntimeBudget.MaxInputBytes ||
		attempt.EnvelopeEstimatedTokens > attempt.RuntimeBudget.MaxInputTokens ||
		attempt.EnvelopeBytes > attempt.Brain.ContextCeilingBytes ||
		attempt.RuntimeBudget.MaxInputTokens+attempt.RuntimeBudget.MaxOutputTokens >
			attempt.Brain.NativeContextLimit ||
		attempt.RuntimeBudget.MaxOutputTokens > attempt.Brain.Sampling.MaxOutputTokens ||
		!validBoundedText(attempt.Envelope, MaxEnvelopeBytes) ||
		policySHA256(attempt.Envelope) != attempt.EnvelopeSHA256 {
		return fmt.Errorf("%w: envelope authority is invalid", ErrInvalidEvidence)
	}
	return nil
}

func (result CallResult) Validate(attempt CallAttempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	if result.Schema != CallResultSchemaV2 || result.CallID != attempt.ID {
		return fmt.Errorf("%w: call result identity is invalid", ErrInvalidEvidence)
	}
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil {
		return fmt.Errorf("%w: call result brain is invalid", ErrInvalidEvidence)
	}
	if result.ProviderIdentityChecked {
		if err := result.ProviderAttestation.ValidateFor(expected); err != nil {
			return fmt.Errorf("%w: fresh provider attestation is invalid", ErrInvalidEvidence)
		}
	} else if !reflect.DeepEqual(result.ProviderAttestation, llm.ProviderIdentityAttestation{}) {
		return fmt.Errorf("%w: unchecked provider result contains attestation evidence", ErrInvalidEvidence)
	}
	if err := validateCallResponse(result, attempt.RuntimeBudget); err != nil {
		return err
	}
	switch result.Status {
	case CallResultAccepted:
		if !result.ProviderIdentityChecked || !result.ResponseStored ||
			result.FailureCode != "" || result.FailureMessage != "" ||
			!validPolicySHA256(result.DecisionSHA256) || result.ActionSchema.Validate() != nil {
			return fmt.Errorf("%w: accepted call result is incomplete", ErrInvalidEvidence)
		}
	case CallResultRejected:
		if !result.ProviderIdentityChecked {
			return fmt.Errorf("%w: rejected result has no fresh provider attestation", ErrInvalidEvidence)
		}
		if result.FailureCode != CallFailureResponseLimit &&
			result.FailureCode != CallFailureInvalidDecision &&
			result.FailureCode != CallFailureAuthorityDenied {
			return fmt.Errorf("%w: rejected call failure code is invalid", ErrInvalidEvidence)
		}
		if !reflect.DeepEqual(result.ActionSchema, cognition.ActionSchemaRef{}) || result.DecisionSHA256 != "" {
			return fmt.Errorf("%w: rejected result claims an accepted decision", ErrInvalidEvidence)
		}
	case CallResultFailed:
		if result.FailureCode != CallFailureGeneration &&
			result.FailureCode != CallFailureProviderIdentity {
			return fmt.Errorf("%w: failed call failure code is invalid", ErrInvalidEvidence)
		}
		if (result.FailureCode == CallFailureGeneration) != result.ProviderIdentityChecked ||
			result.ResponseBytes != 0 ||
			result.ResponseSHA256 != "" || result.ResponseStored || result.Response != "" ||
			!reflect.DeepEqual(result.ActionSchema, cognition.ActionSchemaRef{}) || result.DecisionSHA256 != "" {
			return fmt.Errorf("%w: failed call result shape is invalid", ErrInvalidEvidence)
		}
	default:
		return fmt.Errorf("%w: call result status %q is not registered", ErrInvalidEvidence, result.Status)
	}
	if result.Status != CallResultAccepted && !validBoundedText(result.FailureMessage, MaxCallFailureBytes) {
		return fmt.Errorf("%w: call failure message is invalid", ErrInvalidEvidence)
	}
	return nil
}

func validateCallResponse(result CallResult, budget cognition.RuntimeBudget) error {
	if result.ResponseBytes < 0 || !validPolicySHA256OrEmpty(result.ResponseSHA256) {
		return fmt.Errorf("%w: response identity is invalid", ErrInvalidEvidence)
	}
	if result.ResponseStored {
		if result.ResponseBytes != len(result.Response) || result.ResponseBytes > budget.MaxOutputBytes ||
			estimatePolicyTokens(result.ResponseBytes) > budget.MaxOutputTokens ||
			!validBoundedText(result.Response, MaxResponseBytes) ||
			policySHA256(result.Response) != result.ResponseSHA256 {
			return fmt.Errorf("%w: stored response is invalid", ErrInvalidEvidence)
		}
		return nil
	}
	if result.Response != "" || (result.ResponseBytes > 0 && result.ResponseSHA256 == "") {
		return fmt.Errorf("%w: omitted response identity is invalid", ErrInvalidEvidence)
	}
	return nil
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
