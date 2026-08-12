package cognitionreplay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
)

const providerFailureAuthoritySchemaV1 = "omnidex.cognition-provider-failure-authority.v1"

type providerFailureAuthority struct {
	Schema               string               `json:"schema"`
	RecordID             string               `json:"record_id"`
	FailureKind          string               `json:"failure_kind"`
	FailureID            string               `json:"failure_id"`
	EpisodeID            cognition.EpisodeID  `json:"episode_id"`
	Actor                cognition.AttemptRef `json:"actor"`
	EvidenceID           string               `json:"evidence_id"`
	ReceiptSHA256        string               `json:"receipt_sha256"`
	BootstrapEvidenceID  string               `json:"bootstrap_evidence_id"`
	BootstrapBrainSHA256 string               `json:"bootstrap_brain_sha256"`
}

func validateTerminalSemantics(
	manifest BaseManifest,
	sources []SourceRecord,
	events []Event,
	checkpoints []KnowledgeCheckpoint,
	chunked []ChunkedBlobBinding,
	blobs map[string]Blob,
) error {
	terminal, preEpisode := manifest.TerminalAuthority.PreEpisodeBrainBootstrapFailure()
	if !preEpisode {
		return nil
	}
	if terminal.RequestedEpisodeID != "episode-"+manifest.PublicAuthoritySHA256 {
		return fmt.Errorf("pre-episode replay requested ID differs from its public authority")
	}
	byOrdinal := make(map[uint64]SourceRecord, len(sources))
	for _, source := range sources {
		byOrdinal[source.Ordinal] = source
	}
	public, err := exactTerminalSource(terminal.PublicRunAuthority, byOrdinal)
	if err != nil || public.Payload.SHA256 != manifest.PublicAuthoritySHA256 {
		return fmt.Errorf("pre-episode replay public authority source changed")
	}
	authoritySource, err := exactTerminalSource(terminal.FailureAuthority, byOrdinal)
	if err != nil {
		return err
	}
	receiptSource, err := exactTerminalSource(terminal.FailureReceipt, byOrdinal)
	if err != nil {
		return err
	}
	evidenceSource, err := exactTerminalSource(terminal.IdentityEvidence, byOrdinal)
	if err != nil {
		return err
	}
	authorityRaw, err := exactSourcePayload(authoritySource, blobs)
	if err != nil {
		return err
	}
	receiptRaw, err := exactSourcePayload(receiptSource, blobs)
	if err != nil {
		return err
	}
	var authority providerFailureAuthority
	if err := decodeExactJSONAuthority(authorityRaw, &authority, "provider failure authority"); err != nil {
		return err
	}
	if err := validateProviderFailureAuthority(authority, terminal, receiptSource.Payload.SHA256); err != nil {
		return err
	}
	var receipt cognitionpolicy.BrainBootstrapFailureReceipt
	if err := decodeExactJSONAuthority(receiptRaw, &receipt, "Brain bootstrap failure receipt"); err != nil {
		return err
	}
	evidenceRaw, err := exactSourcePayload(evidenceSource, blobs)
	if err != nil {
		return err
	}
	var replayEvidence ProviderIdentityEvidenceReplay
	if err := decodeCanonical(evidenceRaw, &replayEvidence, "replay provider identity evidence"); err != nil ||
		replayEvidence.Validate() != nil || replayEvidence.Ref.ID != authority.EvidenceID {
		return fmt.Errorf("pre-episode replay provider identity manifest changed: %v", err)
	}
	evidence, bodySources, err := reconstructProviderIdentityEvidence(
		replayEvidence, byOrdinal, chunked, blobs,
	)
	if err != nil {
		return err
	}
	if err := (cognitionpolicy.BrainBootstrapFailure{
		Receipt: receipt, IdentityEvidence: evidence,
	}).Validate(); err != nil {
		return fmt.Errorf("pre-episode replay failure proof is invalid: %w", err)
	}
	if receipt.ID != terminal.FailureID || receipt.Evidence.ID != terminal.IdentityEvidence.ID {
		return fmt.Errorf("pre-episode replay receipt identity changed")
	}
	if err := validatePreEpisodeSources(sources, terminal, bodySources); err != nil {
		return err
	}
	if err := validatePreEpisodeEvents(
		events, terminal, replayEvidence, authoritySource, receiptSource, evidenceSource,
		byOrdinal, blobs,
	); err != nil {
		return err
	}
	return validatePreEpisodeCheckpoints(checkpoints, terminal, receiptSource.Payload)
}

func validateProviderFailureAuthority(
	authority providerFailureAuthority,
	terminal PreEpisodeBrainBootstrapFailureTerminal,
	receiptSHA string,
) error {
	copy := authority
	copy.RecordID = ""
	raw, err := exactjson.Canonical(copy)
	if err != nil {
		return err
	}
	wantRecordID := "cognition_provider_failure_" + digestBytes(raw)
	if authority.Schema != providerFailureAuthoritySchemaV1 ||
		authority.RecordID != wantRecordID || authority.RecordID != terminal.RecordID ||
		authority.FailureKind != "brain_bootstrap" || authority.FailureID != terminal.FailureID ||
		string(authority.EpisodeID) != terminal.RequestedEpisodeID || authority.Actor != terminal.Actor ||
		authority.EvidenceID != terminal.IdentityEvidence.ID || authority.ReceiptSHA256 != receiptSHA ||
		authority.BootstrapEvidenceID != "" || authority.BootstrapBrainSHA256 != "" {
		return fmt.Errorf("pre-episode replay failure authority changed")
	}
	return nil
}

func decodeExactJSONAuthority(raw []byte, target any, label string) error {
	if err := exactjson.ValidateObject(raw, target, label); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	want, err := exactjson.Canonical(target)
	if err != nil || !bytes.Equal(raw, want) {
		return fmt.Errorf("%s is not exact canonical JSON", label)
	}
	return nil
}

func exactTerminalSource(
	ref SourceRef,
	values map[uint64]SourceRecord,
) (SourceRecord, error) {
	source, exists := values[ref.Ordinal]
	if !exists || source.Ref() != ref {
		return SourceRecord{}, fmt.Errorf("pre-episode replay terminal source changed")
	}
	return source, nil
}

func exactSourcePayload(source SourceRecord, blobs map[string]Blob) ([]byte, error) {
	blob, exists := blobs[source.Payload.SHA256]
	if !exists || !source.Payload.matches(blob) {
		return nil, fmt.Errorf("pre-episode replay source payload is unavailable")
	}
	return bytes.Clone(blob.Data), nil
}

func sourceRefsEqual(left, right []SourceRef) bool { return reflect.DeepEqual(left, right) }
