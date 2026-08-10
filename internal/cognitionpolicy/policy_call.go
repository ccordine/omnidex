package cognitionpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func (policy *Policy) executeReservedCall(
	ctx context.Context,
	attempt CallAttempt,
	snapshot cognition.RuntimeSnapshot,
) (cognition.PolicyOutcome, error) {
	expected, err := policy.brain.Ref.ProviderExpectation()
	if err != nil {
		failure := fmt.Errorf("%w: frozen provider expectation: %v", ErrProviderIdentity, err)
		return cognition.PolicyOutcome{}, policy.finishCall(
			ctx, attempt, providerIdentityFailedCallResult(attempt, failure), failure,
		)
	}
	providerAttestation, err := llm.RequireProviderIdentityAttestation(
		ctx, policy.client, expected,
	)
	if err != nil {
		failure := fmt.Errorf("%w: %v", ErrProviderIdentity, err)
		return cognition.PolicyOutcome{}, policy.finishCall(
			ctx, attempt, providerIdentityFailedCallResult(attempt, failure), failure,
		)
	}
	prepared, err := policy.prepareModelCall(ctx, attempt, snapshot)
	if err != nil {
		failure := fmt.Errorf("%w: %v", ErrGeneration, err)
		return cognition.PolicyOutcome{}, policy.finishCall(
			ctx, attempt, failedCallResult(attempt, providerAttestation, err), failure,
		)
	}
	defer policy.client.CleanupPreparedModel(prepared)
	contractSHA, err := preparedModelSHA(prepared)
	if err != nil {
		failure := fmt.Errorf("%w: %v", ErrGeneration, err)
		return cognition.PolicyOutcome{}, policy.finishCall(
			ctx, attempt, failedCallResult(attempt, providerAttestation, err), failure,
		)
	}
	response, err := policy.client.GeneratePrepared(ctx, prepared)
	executed := cognition.PolicyOutcome{InferenceExecuted: true}
	if err != nil {
		failure := fmt.Errorf("%w: %v", ErrGeneration, err)
		return executed, policy.finishCall(
			ctx, attempt, failedCallResult(attempt, providerAttestation, err), failure,
		)
	}
	if after, hashErr := preparedModelSHA(prepared); hashErr != nil || after != contractSHA {
		failure := fmt.Errorf("%w: prepared generation contract was mutated", ErrGeneration)
		return executed, policy.finishCall(
			ctx, attempt, failedCallResult(attempt, providerAttestation, failure), failure,
		)
	}
	budget := attempt.RuntimeBudget
	if !validBoundedText(response, MaxResponseBytes) || len(response) > budget.MaxOutputBytes ||
		estimatePolicyTokens(len(response)) > budget.MaxOutputTokens {
		failure := fmt.Errorf(
			"%w: response must be nonempty UTF-8 without NUL and within %d bytes/%d estimated tokens",
			ErrResponseLimit, budget.MaxOutputBytes, budget.MaxOutputTokens,
		)
		result := rejectedCallResult(
			attempt, providerAttestation, response, CallFailureResponseLimit, failure,
		)
		return executed, policy.finishCall(ctx, attempt, result, failure)
	}

	decision, schema, err := decodePolicyDecision(response, snapshot)
	if err != nil {
		failureCode := CallFailureInvalidDecision
		if errors.Is(err, cognition.ErrAuthorityDenied) {
			failureCode = CallFailureAuthorityDenied
		}
		result := rejectedCallResult(
			attempt, providerAttestation, response, failureCode, err,
		)
		return executed, policy.finishCall(ctx, attempt, result, err)
	}
	decisionRaw, err := json.Marshal(decision)
	if err != nil {
		failure := fmt.Errorf("%w: encode accepted decision: %v", ErrInvalidDecision, err)
		result := rejectedCallResult(
			attempt, providerAttestation, response, CallFailureInvalidDecision, failure,
		)
		return executed, policy.finishCall(ctx, attempt, result, failure)
	}
	result := acceptedCallResult(
		attempt, providerAttestation, response, schema.Ref(), policySHA256(string(decisionRaw)),
	)
	if err := policy.finishCall(ctx, attempt, result, nil); err != nil {
		return executed, err
	}
	executed.Decision = decision.Clone()
	return executed, nil
}

func (policy *Policy) prepareModelCall(
	ctx context.Context,
	attempt CallAttempt,
	snapshot cognition.RuntimeSnapshot,
) (llm.PreparedModel, error) {
	if attempt.RuntimeBudget.MaxOutputTokens > policy.brain.Ref.Sampling.MaxOutputTokens {
		return llm.PreparedModel{}, fmt.Errorf("runtime output limit exceeds the frozen cognition station ceiling")
	}
	prepared, err := policy.client.PrepareContextModel(ctx, policy.brain.Ref.Model, attempt.Envelope)
	if err != nil {
		return llm.PreparedModel{}, fmt.Errorf("prepare exact cognition model context: %w", err)
	}
	if prepared.BaseModel != policy.brain.Ref.Model || prepared.ContextModel != policy.brain.Ref.Model ||
		prepared.Prompt != attempt.Envelope {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, fmt.Errorf("prepared model changed the frozen model or exact envelope")
	}
	schemaRaw, err := decisionSchemaJSON(snapshot.ActionCatalog())
	if err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, fmt.Errorf("decode exact cognition response schema: %w", err)
	}
	zero := 0.0
	prepared.PromptHint = llm.MinimalGeneratePrompt
	prepared.MaxOutputTokens = attempt.RuntimeBudget.MaxOutputTokens
	prepared.ContextTokens = policy.brain.Ref.NativeContextLimit
	prepared.ResponseFormat = llm.ResponseFormatJSON
	prepared.ResponseSchema = schema
	prepared.ThinkingEnabled = false
	prepared.Temperature = &zero
	if err := llm.ValidateResponseContract(prepared); err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	if err := llm.ValidateInferenceBudget(
		prepared.ContextTokens, prepared.MaxOutputTokens, prepared.Prompt, prepared.PromptHint,
	); err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, err
	}
	if err := policy.exactClient.ValidateExactPreparedContract(prepared); err != nil {
		policy.client.CleanupPreparedModel(prepared)
		return llm.PreparedModel{}, fmt.Errorf("provider rejected exact prepared contract: %w", err)
	}
	return prepared, nil
}

func preparedModelSHA(prepared llm.PreparedModel) (string, error) {
	raw, err := json.Marshal(prepared)
	if err != nil {
		return "", err
	}
	return policySHA256(string(raw)), nil
}

func decodePolicyDecision(
	response string,
	snapshot cognition.RuntimeSnapshot,
) (cognition.CognitionDecision, cognition.ActionSchema, error) {
	kind, err := responseActionKind(response)
	if err != nil {
		return cognition.CognitionDecision{}, cognition.ActionSchema{}, err
	}
	schema, exists := snapshot.ActionCatalog().Schema(kind)
	if !exists {
		return cognition.CognitionDecision{}, cognition.ActionSchema{}, fmt.Errorf(
			"%w: response action kind %q is absent from the bound catalog", ErrInvalidDecision, kind,
		)
	}
	decision, err := cognition.DecodeCognitionDecision([]byte(response), schema)
	if err != nil {
		return cognition.CognitionDecision{}, cognition.ActionSchema{}, fmt.Errorf("%w: %w", ErrInvalidDecision, err)
	}
	if err := validateDecisionForSnapshot(decision, snapshot); err != nil {
		return cognition.CognitionDecision{}, cognition.ActionSchema{}, err
	}
	return decision, schema, nil
}

func (policy *Policy) replayReservedCall(
	reservation CallReservation,
	snapshot cognition.RuntimeSnapshot,
) (cognition.PolicyOutcome, error) {
	if reservation.ExistingResult == nil {
		return cognition.PolicyOutcome{}, fmt.Errorf(
			"%w: reserved call %s has no durable terminal result",
			ErrCallIndeterminate, reservation.Attempt.ID,
		)
	}
	result := *reservation.ExistingResult
	switch result.Status {
	case CallResultAccepted:
		decision, err := decisionFromAcceptedResult(result, snapshot)
		return cognition.PolicyOutcome{Decision: decision}, err
	case CallResultRejected:
		sentinel := error(ErrInvalidDecision)
		if result.FailureCode == CallFailureResponseLimit {
			sentinel = ErrResponseLimit
		}
		if result.FailureCode == CallFailureAuthorityDenied {
			return cognition.PolicyOutcome{}, fmt.Errorf(
				"%w: %w: prior call: %s",
				ErrInvalidDecision, cognition.ErrAuthorityDenied, result.FailureMessage,
			)
		}
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: prior call: %s", sentinel, result.FailureMessage)
	case CallResultFailed:
		sentinel := error(ErrGeneration)
		if result.FailureCode == CallFailureProviderIdentity {
			sentinel = ErrProviderIdentity
		}
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: prior call: %s", sentinel, result.FailureMessage)
	default:
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: prior call result is invalid", ErrInvalidEvidence)
	}
}

func (policy *Policy) finishCall(
	ctx context.Context,
	attempt CallAttempt,
	result CallResult,
	primary error,
) error {
	if err := result.Validate(attempt); err != nil {
		if primary == nil {
			return err
		}
		return errors.Join(primary, err)
	}
	if err := policy.journal.Finish(ctx, attempt, result); err != nil {
		journalErr := fmt.Errorf("%w: finish call: %v", ErrCallJournal, err)
		if primary == nil {
			return journalErr
		}
		return errors.Join(primary, journalErr)
	}
	return primary
}
