package cognitionpolicy

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func providerIdentityFailedCallResult(attempt CallAttempt, cause error) CallResult {
	return CallResult{
		Schema: CallResultSchemaV2, CallID: attempt.ID, Status: CallResultFailed,
		FailureCode: CallFailureProviderIdentity, FailureMessage: boundedFailureMessage(cause),
	}
}

func failedCallResult(
	attempt CallAttempt,
	attestation llm.ProviderIdentityAttestation,
	cause error,
) CallResult {
	return CallResult{
		Schema: CallResultSchemaV2, CallID: attempt.ID, Status: CallResultFailed,
		ProviderIdentityChecked: true, ProviderAttestation: attestation,
		FailureCode: CallFailureGeneration, FailureMessage: boundedFailureMessage(cause),
	}
}

func rejectedCallResult(
	attempt CallAttempt,
	attestation llm.ProviderIdentityAttestation,
	response string,
	code CallFailureCode,
	cause error,
) CallResult {
	result := CallResult{
		Schema: CallResultSchemaV2, CallID: attempt.ID, Status: CallResultRejected,
		ProviderIdentityChecked: true, ProviderAttestation: attestation,
		ResponseSHA256: policySHA256(response), ResponseBytes: len(response),
		FailureCode: code, FailureMessage: boundedFailureMessage(cause),
	}
	if responseCanBeStored(response, attempt.RuntimeBudget) {
		result.ResponseStored, result.Response = true, response
	}
	return result
}

func acceptedCallResult(
	attempt CallAttempt,
	attestation llm.ProviderIdentityAttestation,
	response string,
	schema cognition.ActionSchemaRef,
	decisionSHA string,
) CallResult {
	return CallResult{
		Schema: CallResultSchemaV2, CallID: attempt.ID, Status: CallResultAccepted,
		ProviderIdentityChecked: true, ProviderAttestation: attestation,
		ResponseStored: true, ResponseSHA256: policySHA256(response),
		ResponseBytes: len(response), Response: response,
		ActionSchema: schema, DecisionSHA256: decisionSHA,
	}
}

func responseCanBeStored(response string, budget cognition.RuntimeBudget) bool {
	return response != "" && utf8.ValidString(response) && !strings.ContainsRune(response, 0) &&
		len(response) <= MaxResponseBytes && len(response) <= budget.MaxOutputBytes &&
		estimatePolicyTokens(len(response)) <= budget.MaxOutputTokens
}

func boundedFailureMessage(cause error) string {
	if cause == nil {
		return "The cognition policy call failed without a registered detail."
	}
	message := strings.TrimSpace(cause.Error())
	if validBoundedText(message, MaxCallFailureBytes) {
		return message
	}
	return fmt.Sprintf(
		"The failure detail was not safe to persist; exact byte SHA-256 is %s.",
		policySHA256(cause.Error()),
	)
}

func decisionFromAcceptedResult(
	result CallResult,
	snapshot cognition.RuntimeSnapshot,
) (cognition.CognitionDecision, error) {
	if result.Status != CallResultAccepted || !result.ResponseStored {
		return cognition.CognitionDecision{}, fmt.Errorf("%w: prior call was not accepted", ErrCallRejected)
	}
	catalog := snapshot.ActionCatalog()
	kind, err := responseActionKind(result.Response)
	if err != nil {
		return cognition.CognitionDecision{}, err
	}
	schema, exists := catalog.Schema(kind)
	if !exists || schema.Ref() != result.ActionSchema {
		return cognition.CognitionDecision{}, fmt.Errorf("%w: prior call schema is no longer exact", ErrInvalidEvidence)
	}
	decision, err := cognition.DecodeCognitionDecision([]byte(result.Response), schema)
	if err != nil {
		return cognition.CognitionDecision{}, fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	if err := validateDecisionForSnapshot(decision, snapshot); err != nil {
		return cognition.CognitionDecision{}, err
	}
	raw, err := json.Marshal(decision)
	if err != nil || policySHA256(string(raw)) != result.DecisionSHA256 {
		return cognition.CognitionDecision{}, fmt.Errorf("%w: prior decision hash changed", ErrInvalidEvidence)
	}
	return decision.Clone(), nil
}
