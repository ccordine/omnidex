package cognitionruntime

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

const (
	AcceptedDecisionRecoverySchemaV1 = "omnidex.cognition-accepted-decision-recovery.v1"
	MaxPolicyCallIDBytes             = 256
)

// AcceptedDecisionRecoveryRef is code authority for replaying one durable
// accepted policy decision under its exact actor or a later replacement.
type AcceptedDecisionRecoveryRef struct {
	ID           string `json:"id"`
	SHA256       string `json:"sha256"`
	PolicyCallID string `json:"policy_call_id"`
}

type ReconciliationReplay struct {
	Command ReconciliationCommand `json:"command"`
	Receipt ReconciliationReceipt `json:"receipt"`
}

type AcceptedDecisionRecovery struct {
	Schema                 string                      `json:"schema"`
	ID                     string                      `json:"id"`
	SHA256                 string                      `json:"sha256"`
	Binding                Binding                     `json:"binding"`
	SourcePolicyCallID     string                      `json:"source_policy_call_id"`
	SourceActor            cognition.AttemptRef        `json:"source_actor"`
	Prepared               PreparedSnapshot            `json:"prepared"`
	Decision               cognition.CognitionDecision `json:"decision"`
	ActionSchema           cognition.ActionSchema      `json:"action_schema"`
	ExistingReconciliation *ReconciliationReplay       `json:"existing_reconciliation,omitempty"`
}

func NewAcceptedDecisionRecovery(
	binding Binding,
	sourcePolicyCallID string,
	prepared PreparedSnapshot,
	decision cognition.CognitionDecision,
	actionSchema cognition.ActionSchema,
	existing *ReconciliationReplay,
) (AcceptedDecisionRecovery, error) {
	recovery := AcceptedDecisionRecovery{
		Schema: AcceptedDecisionRecoverySchemaV1, Binding: binding,
		SourcePolicyCallID: sourcePolicyCallID, SourceActor: prepared.Snapshot.Attempt(),
		Prepared: prepared.clone(), Decision: decision.Clone(), ActionSchema: actionSchema.Clone(),
	}
	if existing != nil {
		copy := ReconciliationReplay{
			Command: existing.Command.Clone(), Receipt: existing.Receipt.Clone(),
		}
		recovery.ExistingReconciliation = &copy
	}
	digest, err := recovery.identitySHA256()
	if err != nil {
		return AcceptedDecisionRecovery{}, err
	}
	recovery.ID, recovery.SHA256 = "cognition_recovery_"+digest, digest
	if err := recovery.ValidateFor(binding); err != nil {
		return AcceptedDecisionRecovery{}, err
	}
	return recovery, nil
}

func (recovery AcceptedDecisionRecovery) Ref() AcceptedDecisionRecoveryRef {
	return AcceptedDecisionRecoveryRef{
		ID: recovery.ID, SHA256: recovery.SHA256, PolicyCallID: recovery.SourcePolicyCallID,
	}
}

func (recovery AcceptedDecisionRecovery) ValidateFor(binding Binding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	if recovery.Schema != AcceptedDecisionRecoverySchemaV1 || recovery.Binding != binding ||
		recovery.SourceActor != recovery.Prepared.Snapshot.Attempt() ||
		!validExactPolicyCallID(recovery.SourcePolicyCallID) {
		return fmt.Errorf("%w: accepted decision recovery authority is incomplete", ErrInvalidJournalState)
	}
	source := Binding{Episode: binding.Episode, Attempt: recovery.SourceActor}
	if err := recovery.Prepared.ValidateFor(source); err != nil {
		return err
	}
	if !acceptedDecisionActorCanReplay(recovery.SourceActor, binding.Attempt) {
		return fmt.Errorf(
			"%w: accepted decision replay requires its exact actor or a newer attempt",
			ErrInvalidJournalState,
		)
	}
	if err := validateRecoveredDecision(recovery); err != nil {
		return err
	}
	if recovery.ExistingReconciliation != nil {
		command := recovery.ExistingReconciliation.Command
		if err := recovery.ExistingReconciliation.Receipt.ValidateFor(command); err != nil {
			return err
		}
		if command.SnapshotSHA256 != recovery.Prepared.Snapshot.SHA256() ||
			command.Projection != recovery.Prepared.Snapshot.ContextProjection() ||
			command.ActionSchema.Ref() != recovery.ActionSchema.Ref() ||
			!reflect.DeepEqual(command.Decision, recovery.Decision) {
			return fmt.Errorf("%w: existing reconciliation changed the accepted decision", ErrInvalidJournalState)
		}
		if command.Binding.Episode != binding.Episode ||
			!sameStep(command.Binding.Attempt, binding.Attempt) ||
			command.Binding.Attempt.Attempt > binding.Attempt.Attempt {
			return fmt.Errorf("%w: existing reconciliation belongs to another attempt lineage", ErrInvalidJournalState)
		}
		if command.Recovery == nil {
			if command.Binding.Attempt != recovery.SourceActor {
				return fmt.Errorf("%w: source reconciliation actor changed", ErrInvalidJournalState)
			}
		} else if command.Recovery.PolicyCallID != recovery.SourcePolicyCallID ||
			!acceptedDecisionActorCanReplay(recovery.SourceActor, command.Binding.Attempt) {
			return fmt.Errorf("%w: recovered reconciliation authority changed", ErrInvalidJournalState)
		}
	}
	digest, err := recovery.identitySHA256()
	if err != nil || recovery.ID != "cognition_recovery_"+digest || recovery.SHA256 != digest {
		return fmt.Errorf("%w: accepted decision recovery identity is invalid", ErrInvalidJournalState)
	}
	return nil
}

func acceptedDecisionActorCanReplay(source, current cognition.AttemptRef) bool {
	if !sameStep(source, current) || source.Attempt > current.Attempt {
		return false
	}
	return source.Attempt < current.Attempt || source == current
}

func validateRecoveredDecision(recovery AcceptedDecisionRecovery) error {
	if err := recovery.ActionSchema.Validate(); err != nil {
		return fmt.Errorf("%w: recovered action schema: %v", ErrInvalidJournalState, err)
	}
	if err := recovery.Decision.Validate(recovery.ActionSchema); err != nil {
		return fmt.Errorf("%w: recovered decision: %v", ErrInvalidJournalState, err)
	}
	snapshot := recovery.Prepared.Snapshot
	current := snapshot.CurrentObligation()
	schema, exists := snapshot.ActionCatalog().Schema(recovery.Decision.Action.Kind)
	if !exists || !reflect.DeepEqual(schema, recovery.ActionSchema) || recovery.Decision.ObligationID != current.ID {
		return fmt.Errorf("%w: recovered decision is not bound to the active obligation", ErrInvalidJournalState)
	}
	available := make(map[string]struct{}, len(snapshot.EvidenceRefs()))
	for _, ref := range snapshot.EvidenceRefs() {
		available[recoveryEvidenceIdentity(ref)] = struct{}{}
	}
	for _, ref := range recovery.Decision.EvidenceRefs {
		if _, exists := available[recoveryEvidenceIdentity(ref)]; !exists {
			return fmt.Errorf("%w: recovered decision cites unavailable evidence", ErrInvalidJournalState)
		}
	}
	return nil
}

func (recovery AcceptedDecisionRecovery) identitySHA256() (string, error) {
	return valueSHA256(struct {
		Schema, SourcePolicyCallID, SnapshotSHA256, GraphSHA256 string
		Binding                                                 Binding
		SourceActor                                             cognition.AttemptRef
		GraphVersion                                            uint64
		EnvironmentTerminal                                     bool
		PublicOutcome                                           string
		Decision                                                cognition.CognitionDecision
		ActionSchema                                            cognition.ActionSchema
		CompletionEvidenceRefs                                  []cognition.EvidenceRef
	}{
		recovery.Schema, recovery.SourcePolicyCallID, recovery.Prepared.Snapshot.SHA256(),
		recovery.Prepared.ObligationGraph.SHA256, recovery.Binding, recovery.SourceActor,
		recovery.Prepared.GraphVersion, recovery.Prepared.EnvironmentTerminal,
		recovery.Prepared.PublicOutcome, recovery.Decision, recovery.ActionSchema,
		recovery.Prepared.CompletionEvidenceRefs,
	})
}

func validExactPolicyCallID(value string) bool {
	return value != "" && len(value) <= MaxPolicyCallIDBytes && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0)
}

func recoveryEvidenceIdentity(ref cognition.EvidenceRef) string {
	return string(ref.ObservationID) + "\x00" + ref.SHA256 + "\x00" +
		fmt.Sprintf("%d", ref.Revision.Number)
}
