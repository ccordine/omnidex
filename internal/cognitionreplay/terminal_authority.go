package cognitionreplay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/exactjson"
)

const TerminalAuthoritySchemaV1 = "omnidex.replay-terminal-authority.v1"

type TerminalAuthorityKind string

const (
	TerminalSealedEpisode                   TerminalAuthorityKind = "sealed_episode"
	TerminalPreEpisodeBrainBootstrapFailure TerminalAuthorityKind = "pre_episode_brain_bootstrap_failure"
)

type SealedEpisodeTerminal struct {
	EpisodeID         string    `json:"episode_id"`
	EpisodeSealSHA256 string    `json:"episode_seal_sha256"`
	TraceSHA256       string    `json:"trace_sha256"`
	EpisodeStartedAt  time.Time `json:"episode_started_at"`
	SealedAt          time.Time `json:"sealed_at"`
}

type PreEpisodeBrainBootstrapFailureTerminal struct {
	RecordID           string               `json:"record_id"`
	RequestedEpisodeID string               `json:"requested_episode_id"`
	Actor              cognition.AttemptRef `json:"actor"`
	FailureID          string               `json:"failure_id"`
	PublicRunAuthority SourceRef            `json:"public_run_authority"`
	FailureAuthority   SourceRef            `json:"failure_authority"`
	FailureReceipt     SourceRef            `json:"failure_receipt"`
	IdentityEvidence   SourceRef            `json:"identity_evidence"`
}

// TerminalAuthority is encoded as one discriminator and one exact variant value.
// The custom JSON boundary prevents a caller from supplying both variants.
type TerminalAuthority struct {
	Schema            string
	Kind              TerminalAuthorityKind
	sealedEpisode     *SealedEpisodeTerminal
	preEpisodeFailure *PreEpisodeBrainBootstrapFailureTerminal
}

func NewSealedEpisodeTerminal(value SealedEpisodeTerminal) (TerminalAuthority, error) {
	authority := TerminalAuthority{
		Schema: TerminalAuthoritySchemaV1, Kind: TerminalSealedEpisode,
		sealedEpisode: &value,
	}
	return authority, authority.Validate()
}

func NewPreEpisodeBrainBootstrapFailureTerminal(
	value PreEpisodeBrainBootstrapFailureTerminal,
) (TerminalAuthority, error) {
	authority := TerminalAuthority{
		Schema: TerminalAuthoritySchemaV1, Kind: TerminalPreEpisodeBrainBootstrapFailure,
		preEpisodeFailure: &value,
	}
	return authority, authority.Validate()
}

func (authority TerminalAuthority) SealedEpisode() (SealedEpisodeTerminal, bool) {
	if authority.sealedEpisode == nil {
		return SealedEpisodeTerminal{}, false
	}
	return *authority.sealedEpisode, true
}

func (authority TerminalAuthority) PreEpisodeBrainBootstrapFailure() (
	PreEpisodeBrainBootstrapFailureTerminal,
	bool,
) {
	if authority.preEpisodeFailure == nil {
		return PreEpisodeBrainBootstrapFailureTerminal{}, false
	}
	return *authority.preEpisodeFailure, true
}

func (authority TerminalAuthority) Validate() error {
	if authority.Schema != TerminalAuthoritySchemaV1 {
		return fmt.Errorf("replay terminal authority schema is invalid")
	}
	switch authority.Kind {
	case TerminalSealedEpisode:
		if authority.sealedEpisode == nil || authority.preEpisodeFailure != nil ||
			validateSealedEpisodeTerminal(*authority.sealedEpisode) != nil {
			return fmt.Errorf("sealed replay terminal authority is invalid")
		}
	case TerminalPreEpisodeBrainBootstrapFailure:
		if authority.preEpisodeFailure == nil || authority.sealedEpisode != nil ||
			validatePreEpisodeTerminal(*authority.preEpisodeFailure) != nil {
			return fmt.Errorf("pre-episode replay terminal authority is invalid")
		}
	default:
		return fmt.Errorf("replay terminal authority kind is invalid")
	}
	return nil
}

func validateSealedEpisodeTerminal(value SealedEpisodeTerminal) error {
	if requireExact(value.EpisodeID, "replay episode ID") != nil ||
		!validDigest(value.EpisodeSealSHA256) || !validDigest(value.TraceSHA256) ||
		value.EpisodeStartedAt.IsZero() || value.SealedAt.Before(value.EpisodeStartedAt) {
		return fmt.Errorf("sealed episode terminal value is invalid")
	}
	return nil
}

func validatePreEpisodeTerminal(value PreEpisodeBrainBootstrapFailureTerminal) error {
	if !validPrefixedDigest(value.RecordID, "cognition_provider_failure_") ||
		!validPrefixedDigest(value.RequestedEpisodeID, "episode-") ||
		value.Actor.Validate() != nil ||
		!validPrefixedDigest(value.FailureID, "brain_bootstrap_failure_") ||
		value.PublicRunAuthority.Validate() != nil || value.FailureAuthority.Validate() != nil ||
		value.FailureReceipt.Validate() != nil || value.IdentityEvidence.Validate() != nil ||
		value.PublicRunAuthority.Kind != SourcePublicRunAuthority ||
		value.PublicRunAuthority.ID != value.RequestedEpisodeID ||
		value.FailureAuthority.Kind != SourceProviderFailureAuthority ||
		value.FailureAuthority.ID != value.RecordID ||
		value.FailureReceipt.Kind != SourceBrainBootstrapFailureReceipt ||
		value.FailureReceipt.ID != value.FailureID ||
		value.IdentityEvidence.Kind != SourceProviderIdentityEvidence ||
		!validPrefixedDigest(value.IdentityEvidence.ID, "provider_identity_") {
		return fmt.Errorf("pre-episode Brain bootstrap terminal value is invalid")
	}
	return nil
}

func validPrefixedDigest(value, prefix string) bool {
	return len(value) == len(prefix)+64 && value[:len(prefix)] == prefix &&
		validDigest(value[len(prefix):])
}

func (authority TerminalAuthority) MarshalJSON() ([]byte, error) {
	if err := authority.Validate(); err != nil {
		return nil, err
	}
	var value any
	if authority.Kind == TerminalSealedEpisode {
		value = *authority.sealedEpisode
	} else {
		value = *authority.preEpisodeFailure
	}
	return json.Marshal(struct {
		Schema string                `json:"schema"`
		Kind   TerminalAuthorityKind `json:"kind"`
		Value  any                   `json:"value"`
	}{authority.Schema, authority.Kind, value})
}

func (authority *TerminalAuthority) UnmarshalJSON(raw []byte) error {
	if authority == nil {
		return fmt.Errorf("replay terminal authority target is nil")
	}
	wire := struct {
		Schema string                `json:"schema"`
		Kind   TerminalAuthorityKind `json:"kind"`
		Value  json.RawMessage       `json:"value"`
	}{}
	if err := exactjson.ValidateObject(raw, wire, "replay terminal authority"); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	result := TerminalAuthority{Schema: wire.Schema, Kind: wire.Kind}
	switch wire.Kind {
	case TerminalSealedEpisode:
		var value SealedEpisodeTerminal
		if err := decodeTerminalValue(wire.Value, &value, "sealed episode terminal value"); err != nil {
			return err
		}
		result.sealedEpisode = &value
	case TerminalPreEpisodeBrainBootstrapFailure:
		var value PreEpisodeBrainBootstrapFailureTerminal
		if err := decodeTerminalValue(wire.Value, &value, "pre-episode terminal value"); err != nil {
			return err
		}
		result.preEpisodeFailure = &value
	default:
		return fmt.Errorf("replay terminal authority kind is invalid")
	}
	if err := result.Validate(); err != nil {
		return err
	}
	*authority = result
	return nil
}

func decodeTerminalValue(raw []byte, target any, label string) error {
	if err := exactjson.ValidateObject(raw, target, label); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	return requireJSONEOF(decoder)
}
