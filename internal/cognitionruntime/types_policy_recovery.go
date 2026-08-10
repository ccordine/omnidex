package cognitionruntime

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
)

const PolicyCallAbandonmentSchemaV1 = "omnidex.cognition-policy-call-abandonment.v1"

type SourceAttemptDisposition string

const (
	SourceAttemptExpired    SourceAttemptDisposition = "expired"
	SourceAttemptSuperseded SourceAttemptDisposition = "superseded"
)

type PolicyCallAbandonmentRef struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	CallID string `json:"call_id"`
}

type PolicyCallAbandonment struct {
	Schema               string                   `json:"schema"`
	ID                   string                   `json:"id"`
	SHA256               string                   `json:"sha256"`
	Episode              cognition.EpisodeRef     `json:"episode"`
	CallID               string                   `json:"call_id"`
	CallOrdinal          uint64                   `json:"call_ordinal"`
	SourceAttemptSHA256  string                   `json:"source_attempt_sha256"`
	SourceSnapshotSHA256 string                   `json:"source_snapshot_sha256"`
	SourceActor          cognition.AttemptRef     `json:"source_actor"`
	SourceDisposition    SourceAttemptDisposition `json:"source_disposition"`
	RecoveryActor        cognition.AttemptRef     `json:"recovery_actor"`
}

func NewPolicyCallAbandonment(
	episode cognition.EpisodeRef,
	callID string,
	callOrdinal uint64,
	callAttemptSHA256, snapshotSHA256 string,
	source cognition.AttemptRef,
	disposition SourceAttemptDisposition,
	recovery cognition.AttemptRef,
) (PolicyCallAbandonment, error) {
	value := PolicyCallAbandonment{
		Schema: PolicyCallAbandonmentSchemaV1, Episode: episode, CallID: callID,
		CallOrdinal: callOrdinal, SourceAttemptSHA256: callAttemptSHA256,
		SourceSnapshotSHA256: snapshotSHA256, SourceActor: source,
		SourceDisposition: disposition, RecoveryActor: recovery,
	}
	digest, err := policyCallAbandonmentSHA(value)
	if err != nil {
		return PolicyCallAbandonment{}, err
	}
	value.SHA256, value.ID = digest, "cognition_call_abandonment_"+digest
	if err := value.Validate(); err != nil {
		return PolicyCallAbandonment{}, err
	}
	return value, nil
}

func (value PolicyCallAbandonment) Ref() PolicyCallAbandonmentRef {
	return PolicyCallAbandonmentRef{ID: value.ID, SHA256: value.SHA256, CallID: value.CallID}
}

func (value PolicyCallAbandonmentRef) Validate() error {
	if value.ID != "cognition_call_abandonment_"+value.SHA256 || !validSHA256(value.SHA256) ||
		!strings.HasPrefix(value.CallID, "cognition_call_") ||
		!validSHA256(strings.TrimPrefix(value.CallID, "cognition_call_")) {
		return fmt.Errorf("%w: policy-call abandonment reference is invalid", ErrInvalidJournalState)
	}
	return nil
}

func (value PolicyCallAbandonment) Validate() error {
	if value.Schema != PolicyCallAbandonmentSchemaV1 ||
		value.ID != "cognition_call_abandonment_"+value.SHA256 ||
		!validSHA256(value.SHA256) || !validSHA256(value.SourceAttemptSHA256) ||
		!validSHA256(value.SourceSnapshotSHA256) || value.CallOrdinal == 0 ||
		!strings.HasPrefix(value.CallID, "cognition_call_") ||
		!validSHA256(strings.TrimPrefix(value.CallID, "cognition_call_")) {
		return fmt.Errorf("%w: policy-call abandonment identity is invalid", ErrInvalidJournalState)
	}
	if err := value.Episode.Validate(); err != nil {
		return fmt.Errorf("%w: abandonment episode: %v", ErrInvalidJournalState, err)
	}
	if err := value.SourceActor.Validate(); err != nil {
		return fmt.Errorf("%w: abandonment source actor: %v", ErrInvalidJournalState, err)
	}
	if err := value.RecoveryActor.Validate(); err != nil {
		return fmt.Errorf("%w: abandonment recovery actor: %v", ErrInvalidJournalState, err)
	}
	if !sameStep(value.SourceActor, value.RecoveryActor) ||
		value.RecoveryActor.Attempt <= value.SourceActor.Attempt ||
		(value.SourceDisposition != SourceAttemptExpired && value.SourceDisposition != SourceAttemptSuperseded) {
		return fmt.Errorf("%w: abandonment replacement authority is invalid", ErrInvalidJournalState)
	}
	expected, err := policyCallAbandonmentSHA(value)
	if err != nil || expected != value.SHA256 {
		return fmt.Errorf("%w: policy-call abandonment hash changed", ErrInvalidJournalState)
	}
	return nil
}

func (value PolicyCallAbandonment) ValidateFor(binding Binding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Episode != binding.Episode || value.RecoveryActor != binding.Attempt {
		return fmt.Errorf("%w: policy-call abandonment targets another actor", ErrInvalidJournalState)
	}
	return nil
}

func policyCallAbandonmentSHA(value PolicyCallAbandonment) (string, error) {
	copy := value
	copy.ID, copy.SHA256 = "", ""
	return valueSHA256(copy)
}
