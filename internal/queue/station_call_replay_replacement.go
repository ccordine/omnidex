package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
)

func validateStationCallReplayReplacementOriginTx(
	ctx context.Context,
	tx pgx.Tx,
	replacement StationGapOpening,
	replacementCall StationCallOpening,
) error {
	if replacement.WorkKind != string(assemblyline.WorkFragmentGenerationReplacement) {
		if replacement.OriginGapOpeningID != 0 || replacement.OriginCallReceiptID != 0 {
			return fmt.Errorf("station replay non-replacement gap claims origin authority")
		}
		return nil
	}
	if replacement.OriginGapOpeningID < 1 || replacement.OriginCallReceiptID < 1 {
		return fmt.Errorf("station replay replacement lacks persisted origin identities")
	}

	var input assemblyline.FragmentGenerationReplacementInput
	if err := decodePortableGapPayload([]byte(replacement.PortablePayload), &input); err != nil {
		return fmt.Errorf("decode station replay replacement payload: %w", err)
	}
	replacementJob := assemblyline.PortableJob{
		Schema:  replacement.PortableSchema,
		ID:      replacement.WorkID,
		Kind:    assemblyline.WorkKind(replacement.WorkKind),
		Payload: json.RawMessage(replacement.PortablePayload),
	}
	if err := replacementJob.Validate(); err != nil {
		return fmt.Errorf("validate station replay replacement payload: %w", err)
	}
	if replacement.GapID != replacementJob.ID ||
		replacement.GapID != replacement.WorkID ||
		replacement.PortablePayloadSHA256 != stationGapSHA256(replacement.PortablePayload) {
		return fmt.Errorf("station replay replacement differs from its validated portable payload")
	}
	chain, err := loadStationCallReplayReplacementOriginTx(
		ctx, tx, replacement.OriginGapOpeningID, replacement.OriginCallReceiptID,
	)
	if err != nil {
		return err
	}
	return validateStationCallReplayReplacementOrigin(
		replacement, replacementCall, input, chain,
	)
}

func validateStationCallReplayReplacementOrigin(
	replacement StationGapOpening,
	replacementCall StationCallOpening,
	replacementInput assemblyline.FragmentGenerationReplacementInput,
	chain stationCallReplayReplacementOrigin,
) error {
	origin := chain.Gap
	originJob := assemblyline.PortableJob{
		Schema:  origin.PortableSchema,
		ID:      origin.WorkID,
		Kind:    assemblyline.WorkKind(origin.WorkKind),
		Payload: json.RawMessage(origin.PortablePayload),
	}
	if err := originJob.Validate(); err != nil {
		return fmt.Errorf("validate station replay replacement origin payload: %w", err)
	}
	var originInput assemblyline.FragmentGenerationInput
	if err := decodePortableGapPayload(originJob.Payload, &originInput); err != nil {
		return fmt.Errorf("decode station replay replacement origin payload: %w", err)
	}
	canonicalOrigin, err := assemblyline.NewFragmentGenerationJob(originInput)
	if err != nil {
		return fmt.Errorf("canonicalize station replay replacement origin payload: %w", err)
	}
	canonicalReplacementOrigin, err := assemblyline.NewFragmentGenerationJob(
		replacementInput.Original,
	)
	if err != nil {
		return fmt.Errorf("canonicalize station replay replacement input: %w", err)
	}
	if origin.ID != replacement.OriginGapOpeningID ||
		origin.WorkKind != string(assemblyline.WorkFragmentGeneration) ||
		origin.GapID != origin.WorkID ||
		origin.PortablePayloadSHA256 != stationGapSHA256(origin.PortablePayload) ||
		origin.OriginGapOpeningID != 0 || origin.OriginCallReceiptID != 0 ||
		!sameStationReplayGapAttempt(origin, replacement) ||
		!bytes.Equal(canonicalOrigin.Payload, canonicalReplacementOrigin.Payload) {
		return fmt.Errorf("station replay replacement origin gap differs from its exact portable authority")
	}
	if origin.ContextTokens != replacement.ContextTokens ||
		origin.MaxOutputTokens != replacement.MaxOutputTokens ||
		origin.OutputLimitMode != replacement.OutputLimitMode ||
		origin.OutputLimitMode != llm.ExactPreparedOutputLimitNatural {
		return fmt.Errorf("station replay replacement origin budget differs from its exact authority")
	}

	call := chain.Call
	if call.GapOpeningID != origin.ID || !stationReplayCallMatchesGap(call, origin) ||
		call.ContextTokens != origin.ContextTokens ||
		call.MaxOutputTokens != origin.MaxOutputTokens ||
		call.OutputLimitMode != origin.OutputLimitMode ||
		replacementCall.Model != call.Model {
		return fmt.Errorf("station replay replacement origin call differs from its exact gap authority")
	}
	receipt := chain.Receipt
	if receipt.ID != replacement.OriginCallReceiptID ||
		receipt.OpeningID != call.ID || !stationReplayReceiptMatchesGap(receipt, origin) ||
		receipt.Status != "failed" || strings.Trim(receipt.Error, " ") == "" ||
		receipt.GenerationSHA256 != stationGapSHA256(string(receipt.GenerationJSON)) {
		return fmt.Errorf("station replay replacement origin receipt lacks exact failed authority")
	}

	generation, err := decodeStationCallReplayReplacementGeneration(receipt.GenerationJSON)
	if err != nil {
		return fmt.Errorf("decode station replay replacement origin receipt: %w", err)
	}
	if err := generation.ValidateProviderResponseReceipt(); err != nil {
		return fmt.Errorf("validate station replay replacement origin receipt: %w", err)
	}
	if generation.ProviderRequestDisposition != llm.ProviderRequestDispatched ||
		generation.ProviderResponseDisposition != llm.ProviderResponseSucceeded ||
		!generation.ProviderResponseComplete || !generation.ProviderResponseBytesKnown ||
		!generation.ProviderDonePresent || !generation.ProviderDone ||
		generation.ProviderDoneReason != "length" || !generation.UsagePresent ||
		generation.ProviderRequestSHA256 != call.WireRequestSHA256 ||
		generation.ProviderResponseModel != call.Model ||
		string(generation.Protocol) != call.Protocol ||
		generation.Content == "" ||
		len(generation.Content) > llm.MaxExactPreparedProviderResponseBytes {
		return fmt.Errorf("station replay replacement origin receipt lacks exact output-limit evidence")
	}
	if err := llm.ValidateExactPreparedNaturalUsage(call.ContextTokens, generation.Usage); err != nil {
		return fmt.Errorf("validate station replay replacement origin native usage: %w", err)
	}

	if err := validateStationCallReplayReplacementEvidence(chain.Evidence, origin, call, receipt, generation); err != nil {
		return err
	}
	outcome := chain.Outcome
	if outcome.OpeningID != origin.ID || !stationReplayOutcomeMatchesGap(outcome, origin) ||
		outcome.Status != StationGapFailed || strings.Trim(outcome.Error, " ") == "" ||
		outcome.Response != "" || outcome.ResponseSHA256 != "" ||
		outcome.ProjectionKind != "" || outcome.CallReceiptSHA256 != "" ||
		outcome.SourceResponseSHA256 != "" || outcome.SourceStartByte != 0 ||
		outcome.SourceEndByte != 0 {
		return fmt.Errorf("station replay replacement origin outcome lacks exact failed authority")
	}
	return nil
}

// decodeStationCallReplayReplacementGeneration excludes the provider identity
// observation because replacement lineage does not consume it. The immutable
// origin call and receipt already own that separate evidence; replay decodes
// only the exact provider-response fields used to prove the output-limit fact.
func decodeStationCallReplayReplacementGeneration(
	raw json.RawMessage,
) (llm.PreparedGeneration, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return llm.PreparedGeneration{}, err
	}
	if document == nil {
		return llm.PreparedGeneration{}, fmt.Errorf("replacement origin receipt must be one JSON object")
	}
	delete(document, "provider_observation")
	projected, err := json.Marshal(document)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	var generation llm.PreparedGeneration
	if err := json.Unmarshal(projected, &generation); err != nil {
		return llm.PreparedGeneration{}, err
	}
	return generation, nil
}

func validateStationCallReplayReplacementEvidence(
	evidence LLMCallEvidence,
	origin StationGapOpening,
	call StationCallOpening,
	receipt StationCallReceipt,
	generation llm.PreparedGeneration,
) error {
	if evidence.StationCallOpeningID != call.ID || evidence.JobID != origin.JobID ||
		evidence.JobGeneration != origin.Generation || evidence.StepID != origin.StepID ||
		evidence.StepAttempt != origin.StepAttempt || evidence.WorkerID != origin.WorkerID ||
		evidence.Scope != origin.Scope || evidence.WorkID != origin.WorkID ||
		evidence.WorkKind != origin.WorkKind || evidence.ContextProjectionID != "" ||
		evidence.RequestedModel != call.Model || evidence.Model != call.Model ||
		evidence.SystemPrompt != origin.Prompt || evidence.UserPrompt != llm.MinimalGeneratePrompt ||
		evidence.WireRequestSHA256 != call.WireRequestSHA256 ||
		evidence.ContextTokens != origin.ContextTokens ||
		evidence.MaxOutputTokens != origin.MaxOutputTokens ||
		evidence.Status != LLMEvidenceGenerationFailed || evidence.Error != receipt.Error ||
		evidence.Response != generation.Content ||
		evidence.ResponseSHA256 != llmEvidenceSHA256(generation.Content) {
		return fmt.Errorf("station replay replacement origin evidence differs from its failed call receipt")
	}
	return nil
}

func sameStationReplayGapAttempt(left, right StationGapOpening) bool {
	return left.JobID == right.JobID && left.Generation == right.Generation &&
		left.StepID == right.StepID && left.StepAttempt == right.StepAttempt &&
		left.WorkerID == right.WorkerID
}

func stationReplayCallMatchesGap(call StationCallOpening, gap StationGapOpening) bool {
	return call.JobID == gap.JobID && call.Generation == gap.Generation &&
		call.StepID == gap.StepID && call.StepAttempt == gap.StepAttempt &&
		call.WorkerID == gap.WorkerID && call.GapID == gap.GapID
}

func stationReplayReceiptMatchesGap(receipt StationCallReceipt, gap StationGapOpening) bool {
	return receipt.JobID == gap.JobID && receipt.Generation == gap.Generation &&
		receipt.StepID == gap.StepID && receipt.StepAttempt == gap.StepAttempt &&
		receipt.WorkerID == gap.WorkerID && receipt.GapID == gap.GapID
}

func stationReplayOutcomeMatchesGap(outcome StationGapOutcome, gap StationGapOpening) bool {
	return outcome.JobID == gap.JobID && outcome.Generation == gap.Generation &&
		outcome.StepID == gap.StepID && outcome.StepAttempt == gap.StepAttempt &&
		outcome.WorkerID == gap.WorkerID && outcome.GapID == gap.GapID
}
