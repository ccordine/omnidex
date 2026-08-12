package cognitiongauntlet

import (
	"bytes"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/llm"
)

func verifyAblationCalls(
	values []ablationCallEvidence,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) error {
	if values == nil || len(values) > int(cognition.MaxPolicyCallsPerEpisode) {
		return fmt.Errorf("ablation call evidence must be an explicit bounded array")
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value.Ordinal != uint32(index+1) {
			return fmt.Errorf("ablation call %d ordinal is invalid", index+1)
		}
		if _, duplicate := seen[value.Attempt.ID]; duplicate {
			return fmt.Errorf("ablation call %d duplicates its identity", index+1)
		}
		seen[value.Attempt.ID] = struct{}{}
		if err := verifyAblationCall(value, store, role); err != nil {
			return fmt.Errorf("ablation call %d: %w", index+1, err)
		}
	}
	return nil
}

func rederiveAblationDecisions(
	values []ablationCallEvidence,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) (map[string]*cognition.CognitionDecision, error) {
	decisions := make(map[string]*cognition.CognitionDecision, len(values))
	for index, value := range values {
		snapshot, err := value.Snapshot.runtimeSnapshot()
		if err != nil {
			return nil, fmt.Errorf("ablation call %d runtime snapshot: %w", index+1, err)
		}
		evidence, err := reconstructAblationCallEvidence(value, store, role)
		if err != nil {
			return nil, fmt.Errorf("ablation call %d: %w", index+1, err)
		}
		decision, err := cognitionpolicy.VerifyCallOutcome(
			snapshot, value.Attempt, value.Result, evidence,
		)
		if err != nil {
			return nil, fmt.Errorf("ablation call %d outcome: %w", index+1, err)
		}
		decisions[value.Attempt.ID] = decision
	}
	return decisions, nil
}

func verifyAblationCall(
	value ablationCallEvidence,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) error {
	projection := value.Projection.Projection
	if err := projection.Validate(); err != nil {
		return fmt.Errorf("projection: %w", err)
	}
	rendered, err := store.read(value.Projection.Content, role)
	if err != nil || !bytes.Equal(rendered, []byte(projection.Rendered)) {
		return fmt.Errorf("projection content differs from rendered input: %v", err)
	}
	snapshot, err := value.Snapshot.runtimeSnapshot()
	if err != nil {
		return fmt.Errorf("runtime snapshot: %w", err)
	}
	if err := cognitionpolicy.VerifyCallAttempt(snapshot, projection, value.Attempt); err != nil {
		return fmt.Errorf("attempt differs from runtime input: %w", err)
	}
	if err := value.Result.Validate(value.Attempt); err != nil {
		return fmt.Errorf("result: %w", err)
	}
	evidence, err := reconstructAblationCallEvidence(value, store, role)
	if err != nil {
		return err
	}
	if _, err := cognitionpolicy.VerifyCallOutcome(
		snapshot, value.Attempt, value.Result, evidence,
	); err != nil {
		return fmt.Errorf("exact call outcome: %w", err)
	}
	return nil
}

func reconstructAblationCallEvidence(
	value ablationCallEvidence,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) (cognitionpolicy.CallEvidence, error) {
	var evidence cognitionpolicy.CallEvidence
	modelPresent := value.Evidence.ModelResponse != nil
	wantModel := value.Result.ResponseEvidence != (cognitionpolicy.ModelResponseEvidenceRef{})
	identityPresent := value.Evidence.ProviderIdentity != nil
	wantIdentity := value.Result.ProviderIdentityEvidence != (llm.ProviderIdentityEvidenceRef{})
	capturePresent := value.Evidence.ProviderResponseCapture != nil
	wantCapture := value.Result.ProviderResponseCapture !=
		(cognitionpolicy.ProviderResponseCaptureEvidenceRef{})
	generationPresent := value.Evidence.ProviderGeneration != nil
	wantGeneration := value.Result.ProviderGenerationEvidence !=
		(cognitionpolicy.ProviderGenerationEvidenceRef{})
	if modelPresent != wantModel || identityPresent != wantIdentity ||
		capturePresent != wantCapture || generationPresent != wantGeneration {
		return cognitionpolicy.CallEvidence{}, fmt.Errorf("evidence presence differs from result refs")
	}
	var err error
	if modelPresent {
		evidence.Response, err = reconstructAblationModelResponse(
			value.Attempt.ID, *value.Evidence.ModelResponse, store, role,
		)
		if err != nil {
			return cognitionpolicy.CallEvidence{}, err
		}
	}
	if identityPresent {
		evidence.ProviderIdentity, err = reconstructAblationIdentity(
			*value.Evidence.ProviderIdentity, store, role,
		)
		if err != nil {
			return cognitionpolicy.CallEvidence{}, err
		}
	}
	if capturePresent {
		evidence.ProviderResponseCapture, err = reconstructAblationCapture(
			value.Attempt.ID, *value.Evidence.ProviderResponseCapture, store, role,
		)
		if err != nil {
			return cognitionpolicy.CallEvidence{}, err
		}
	}
	if generationPresent {
		evidence.ProviderGeneration, err = reconstructAblationGeneration(
			value.Attempt.ID, *value.Evidence.ProviderGeneration, store, role,
		)
		if err != nil {
			return cognitionpolicy.CallEvidence{}, err
		}
	}
	return evidence, nil
}

func reconstructAblationModelResponse(
	callID string,
	value ablationModelResponseEvidence,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) (cognitionpolicy.ModelResponseEvidence, error) {
	raw, err := store.read(value.Content, role)
	if err != nil {
		return cognitionpolicy.ModelResponseEvidence{}, err
	}
	evidence, err := cognitionpolicy.NewModelResponseEvidence(callID, string(raw))
	if err != nil || evidence.Ref != value.Ref || evidence.CallID != value.CallID {
		return cognitionpolicy.ModelResponseEvidence{}, fmt.Errorf("model response authority changed: %v", err)
	}
	return evidence, nil
}

func reconstructAblationCapture(
	callID string,
	value ablationProviderCaptureEvidence,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) (cognitionpolicy.ProviderResponseCaptureEvidence, error) {
	raw, err := store.read(value.Content, role)
	if err != nil {
		return cognitionpolicy.ProviderResponseCaptureEvidence{}, err
	}
	evidence, err := cognitionpolicy.NewProviderResponseCaptureEvidence(callID, raw)
	if err != nil || evidence.Ref != value.Ref || evidence.CallID != value.CallID {
		return cognitionpolicy.ProviderResponseCaptureEvidence{}, fmt.Errorf(
			"provider response capture authority changed: %v", err,
		)
	}
	return evidence, nil
}

func reconstructAblationGeneration(
	callID string,
	value ablationProviderGenerationEvidence,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) (cognitionpolicy.ProviderGenerationEvidence, error) {
	raw, err := store.read(value.Content, role)
	if err != nil {
		return cognitionpolicy.ProviderGenerationEvidence{}, err
	}
	evidence := cognitionpolicy.ProviderGenerationEvidence{
		Ref: value.Ref, CallID: value.CallID, Generation: raw,
	}
	if value.CallID != callID || evidence.Validate() != nil {
		return cognitionpolicy.ProviderGenerationEvidence{}, fmt.Errorf(
			"provider generation authority changed",
		)
	}
	return evidence, nil
}

func reconstructAblationIdentity(
	value ablationIdentityEvidence,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) (llm.ProviderIdentityEvidence, error) {
	operations := make([]llm.ProviderIdentityOperationEvidence, len(value.Operations))
	for index, encoded := range value.Operations {
		request, err := store.read(encoded.RequestContent, role)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, err
		}
		response, err := store.read(encoded.ResponseContent, role)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, err
		}
		operation, err := llm.NewProviderIdentityOperationEvidence(
			encoded.Operation, encoded.Method, encoded.Endpoint, encoded.RequestDisposition,
			request, encoded.HTTPStatus, encoded.Disposition, encoded.ResponseComplete,
			encoded.ContentEncoding, response,
		)
		if err != nil || operation.RequestSHA256 != encoded.RequestSHA256 ||
			operation.RequestBytes != encoded.RequestBytes ||
			operation.ResponseSHA256 != encoded.ResponseSHA256 ||
			operation.ResponseBytes != encoded.ResponseBytes {
			return llm.ProviderIdentityEvidence{}, fmt.Errorf(
				"provider identity operation %d authority changed: %v", index+1, err,
			)
		}
		operations[index] = operation
	}
	evidence, err := llm.NewProviderIdentityEvidence(operations)
	if err != nil || evidence.Schema != value.Schema || evidence.Ref != value.Ref {
		return llm.ProviderIdentityEvidence{}, fmt.Errorf(
			"provider identity authority changed: %v schema=%q/%q ref=%+v/%+v",
			err, evidence.Schema, value.Schema, evidence.Ref, value.Ref,
		)
	}
	return evidence, nil
}
