package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func validateStationCallOpening(record StationCallOpenRecord) (StationCallOpening, error) {
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return StationCallOpening{}, err
	}
	if record.Gap.ID < 1 || record.Gap.GapID == "" ||
		record.Gap.JobID != record.Authority.JobID || record.Gap.Generation != record.Authority.Generation ||
		record.Gap.StepID != record.Authority.StepID || record.Gap.StepAttempt != record.Authority.Attempt ||
		record.Gap.WorkerID != record.Authority.WorkerID {
		return StationCallOpening{}, fmt.Errorf("station call requires its exact persisted gap authority")
	}
	if err := ValidateStationGapSemanticUncertainty(record.Gap); err != nil {
		return StationCallOpening{}, fmt.Errorf("station call semantic uncertainty: %w", err)
	}
	if record.Discovery.ID < 1 || record.Discovery.Status != "succeeded" ||
		record.Discovery.GapID != record.Gap.GapID || record.Discovery.JobID != record.Authority.JobID ||
		record.Discovery.Generation != record.Authority.Generation ||
		record.Discovery.StepID != record.Authority.StepID ||
		record.Discovery.StepAttempt != record.Authority.Attempt ||
		record.Discovery.WorkerID != record.Authority.WorkerID {
		return StationCallOpening{}, fmt.Errorf("station call requires its successful discovery receipt")
	}
	prepared := record.Prepared
	if prepared.ProviderIdentityExpectation == nil {
		return StationCallOpening{}, fmt.Errorf("station call requires one discovered provider expectation")
	}
	if err := llm.ValidateExactPreparedProviderExpectation(*prepared.ProviderIdentityExpectation); err != nil {
		return StationCallOpening{}, err
	}
	wire, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return StationCallOpening{}, fmt.Errorf("render exact station call wire request: %w", err)
	}
	if len(wire) > maxStationRequestResourceBytes {
		return StationCallOpening{}, fmt.Errorf(
			"station call wire request exceeds coarse %d-byte request resource ceiling",
			maxStationRequestResourceBytes,
		)
	}
	expectedScope, err := stationGapScope(assemblyline.WorkKind(record.Gap.WorkKind))
	if err != nil {
		return StationCallOpening{}, err
	}
	if prepared.Prompt != record.Gap.Prompt || prepared.ContextTokens != record.Gap.ContextTokens ||
		prepared.MaxOutputTokens != record.Gap.MaxOutputTokens ||
		prepared.OutputLimitMode != record.Gap.OutputLimitMode ||
		prepared.Protocol != llm.ExactPreparedProtocolRawTextV2 ||
		expectedScope != record.Gap.Scope {
		return StationCallOpening{}, fmt.Errorf("station call prepared authority differs from its gap projection")
	}
	expectedStop, err := ExpectedStationCallStopSequence(
		record.Gap, *prepared.ProviderIdentityExpectation,
	)
	if err != nil {
		return StationCallOpening{}, err
	}
	if prepared.RawTextStopSequence != expectedStop {
		return StationCallOpening{}, fmt.Errorf("station call stop sequence differs from its raw response transport")
	}
	expectation, err := exactjson.Canonical(*prepared.ProviderIdentityExpectation)
	if err != nil {
		return StationCallOpening{}, err
	}
	if !bytes.Equal(record.Discovery.Expectation, expectation) {
		return StationCallOpening{}, fmt.Errorf("station call expectation differs from its discovery receipt")
	}
	modelInput, err := llm.ExactPreparedRequestModelInput(prepared)
	if err != nil {
		return StationCallOpening{}, err
	}
	maxInputTokens := prepared.ContextTokens - prepared.MaxOutputTokens
	modelInputTokenCeiling := maxInputTokens
	if prepared.OutputLimitMode == llm.ExactPreparedOutputLimitNatural {
		maxInputTokens = prepared.ContextTokens
		modelInputTokenCeiling = prepared.ContextTokens
	}
	return StationCallOpening{
		GapOpeningID: record.Gap.ID, DiscoveryReceiptID: record.Discovery.ID,
		JobID:      record.Authority.JobID,
		Generation: record.Authority.Generation, StepID: record.Authority.StepID,
		StepAttempt: record.Authority.Attempt, WorkerID: record.Authority.WorkerID,
		GapID: record.Gap.GapID, Protocol: string(prepared.Protocol),
		TokenizerProfile: prepared.ProviderIdentityExpectation.TokenizerProfile,
		ProviderMethod:   "POST", ProviderEndpoint: "/api/generate",
		WireRequest: append([]byte(nil), wire...), WireRequestSHA256: stationGapSHA256(string(wire)),
		WireRequestBytes: len(wire), Expectation: append(json.RawMessage(nil), expectation...),
		ExpectationSHA256:    stationGapSHA256(string(expectation)),
		ObservationChallenge: prepared.ProviderObservationChallenge,
		Model:                prepared.ContextModel, ContextTokens: prepared.ContextTokens,
		MaxInputTokens:  maxInputTokens,
		MaxOutputTokens: prepared.MaxOutputTokens, ModelInput: modelInput,
		OutputLimitMode:  prepared.OutputLimitMode,
		ModelInputSHA256: stationGapSHA256(modelInput), ModelInputBytes: len(modelInput),
		// The provider owns tokenization. This is the declared native input
		// ceiling; the receipt records and validates the actual prompt count.
		ModelInputTokenCeiling: modelInputTokenCeiling,
	}, nil
}

func stationAuthorityFailureReason(
	status model.StepAttemptStatus,
) (llm.ProviderRequestFailureReason, error) {
	switch status {
	case model.StepAttemptCanceled:
		return llm.ProviderRequestFailureAuthorityCanceled, nil
	case model.StepAttemptSuperseded:
		return llm.ProviderRequestFailureAuthoritySuperseded, nil
	case model.StepAttemptExpired:
		return llm.ProviderRequestFailureAuthorityExpired, nil
	default:
		return "", fmt.Errorf("attempt status %q is not ended station authority", status)
	}
}

func validateStationCallReceipt(
	record StationCallReceiptRecord,
	opening StationCallOpening,
) ([]byte, error) {
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return nil, err
	}
	if record.OpeningID != opening.ID || record.GapID != opening.GapID ||
		opening.JobID != record.Authority.JobID || opening.Generation != record.Authority.Generation ||
		opening.StepID != record.Authority.StepID || opening.StepAttempt != record.Authority.Attempt ||
		opening.WorkerID != record.Authority.WorkerID {
		return nil, fmt.Errorf("station call receipt differs from its exact original attempt")
	}
	if len(record.Error) > maxStationCallErrorBytes || record.Error != strings.TrimSpace(record.Error) ||
		(record.Error != "" && strings.TrimSpace(record.Error) == "") {
		return nil, fmt.Errorf("station call receipt error must be exact bounded text")
	}
	authorityEnded := record.Result.ProviderRequestDisposition == llm.ProviderRequestNotDispatched &&
		record.Result.ProviderRequestFailureReason.Validate() == nil
	identityFailure := record.Result.ProviderRequestDisposition == llm.ProviderRequestNotDispatched &&
		record.Result.ProviderResponseDisposition == "" && !authorityEnded
	if authorityEnded {
		if record.Error == "" || record.Result.ProviderObservation != (llm.ProviderIdentityObservation{}) ||
			record.Result.ProviderResponseDisposition != "" || record.Result.ProviderRequestSHA256 != opening.WireRequestSHA256 ||
			record.Result.ProviderRequestFailureReason.Validate() != nil {
			return nil, fmt.Errorf("authority-ended station call receipt is invalid")
		}
	} else if identityFailure {
		if record.Error == "" || record.Result.ProviderObservation != (llm.ProviderIdentityObservation{}) ||
			record.Result.ProviderResponseDisposition != "" || record.Result.ProviderRequestSHA256 != "" {
			return nil, fmt.Errorf("undispatched station call receipt must contain only identity failure evidence")
		}
	} else if record.Error == "" {
		if record.Result.ProviderRequestFailureReason != "" {
			return nil, fmt.Errorf("successful station call cannot claim a request failure reason")
		}
		if err := record.Result.Validate(); err != nil {
			return nil, fmt.Errorf("validate successful station call receipt: %w", err)
		}
	} else {
		if record.Result.ProviderRequestFailureReason != "" {
			return nil, fmt.Errorf("provider station failure reason is invalid for a dispatched call")
		}
		if record.Result.Validate() == nil {
			return nil, fmt.Errorf("successful station call cannot be recorded as failed")
		}
		if err := record.Result.ValidateInvocationEvidence(); err != nil {
			return nil, fmt.Errorf("validate failed station call receipt: %w", err)
		}
	}
	if string(record.Result.Protocol) != opening.Protocol || (!identityFailure && !authorityEnded &&
		(record.Result.ProviderRequestSHA256 != opening.WireRequestSHA256 ||
			record.Result.ProviderObservation.ChallengeSHA256 != opening.ObservationChallenge)) {
		return nil, fmt.Errorf("station call result differs from its opened wire authority")
	}
	var expected llm.ProviderIdentityExpectation
	if err := json.Unmarshal(opening.Expectation, &expected); err != nil {
		return nil, fmt.Errorf("decode station call expectation: %w", err)
	}
	selection, err := llm.ProviderIdentitySelectionForExpectation(expected)
	if err != nil {
		return nil, fmt.Errorf("derive station call provider selection: %w", err)
	}
	if authorityEnded {
		successfulDiscovery := record.Result.ProviderIdentityEvidence.ValidateRequests(selection) == nil &&
			record.Result.ProviderIdentityEvidence.Successful()
		exactIdentityFailure := record.Result.ProviderIdentityEvidence.ValidateFailure(selection, &expected) == nil
		if !successfulDiscovery && !exactIdentityFailure {
			return nil, fmt.Errorf("authority-ended station call lacks exact provider identity evidence")
		}
	} else if identityFailure {
		if err := record.Result.ProviderIdentityEvidence.ValidateFailure(selection, &expected); err != nil {
			return nil, fmt.Errorf("station call receipt lacks exact identity failure evidence: %w", err)
		}
	} else {
		derived, err := llm.DeriveExactProviderIdentityExpectation(record.Result.ProviderIdentityEvidence, selection)
		if err != nil || derived != expected {
			return nil, fmt.Errorf("station call identity evidence differs from its opened expectation")
		}
	}
	type durableGeneration struct {
		llm.PreparedGeneration
		ProviderIdentityEvidence llm.ProviderIdentityEvidence `json:"provider_identity_evidence"`
	}
	return exactjson.Canonical(durableGeneration{record.Result, record.Result.ProviderIdentityEvidence})
}

// ValidateStationCallNativeUsage compares the provider's tokenizer-owned
// receipt with the immutable context ceilings opened for this exact call.
// Callers run it after persisting the raw provider receipt so an invariant
// failure cannot erase the evidence that proved it.
func ValidateStationCallNativeUsage(
	opening StationCallOpening,
	result llm.PreparedGeneration,
) error {
	if result.ProviderResponseDisposition != llm.ProviderResponseSucceeded ||
		!result.UsagePresent {
		return fmt.Errorf("station call native usage requires one successful provider receipt")
	}
	switch opening.OutputLimitMode {
	case llm.ExactPreparedOutputLimitExplicit:
		return llm.ValidateExactPreparedNativeUsage(
			opening.ContextTokens,
			opening.MaxInputTokens,
			opening.MaxOutputTokens,
			result.Usage,
		)
	case llm.ExactPreparedOutputLimitNatural:
		var expected llm.ProviderIdentityExpectation
		if err := json.Unmarshal(opening.Expectation, &expected); err != nil {
			return fmt.Errorf("decode station call natural-output expectation: %w", err)
		}
		settings, err := llm.ResolveExactPreparedTransport(expected)
		if err != nil {
			return err
		}
		if settings.NaturalOutputCeiling {
			return llm.ValidateExactPreparedNaturalUsageWithOutputCeiling(
				opening.ContextTokens, opening.MaxOutputTokens, result.Usage,
			)
		}
		return llm.ValidateExactPreparedNaturalUsage(opening.ContextTokens, result.Usage)
	default:
		return fmt.Errorf("station call output-limit mode %q is not registered", opening.OutputLimitMode)
	}
}
