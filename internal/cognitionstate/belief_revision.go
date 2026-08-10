package cognitionstate

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	BeliefRevisionSchemaV1 = "omnidex.cognition-state-belief-revision.v1"
	beliefRevisionReason   = "Exact current evidence contradicts the active hypothesis."
)

type BeliefRevisionMaterialization struct {
	Schema               string                       `json:"schema"`
	ID                   string                       `json:"id"`
	SHA256               string                       `json:"sha256"`
	SourceSnapshotSHA256 string                       `json:"source_snapshot_sha256"`
	SourceDecisionSHA256 string                       `json:"source_decision_sha256"`
	LedgerID             taskstate.LedgerID           `json:"ledger_id"`
	ExpectedLedgerSHA256 string                       `json:"expected_ledger_sha256"`
	ExpectedVersion      uint64                       `json:"expected_version"`
	TargetRef            cognition.EpistemicRef       `json:"target_ref"`
	EvidenceRefs         []cognition.EvidenceRef      `json:"evidence_refs"`
	Rejection            taskstate.RejectEntryCommand `json:"rejection"`
	ResultLedgerSHA256   string                       `json:"result_ledger_sha256"`
}

func PlanBeliefRevision(input ModelProposalInput) (BeliefRevisionMaterialization, bool, error) {
	if err := validateBeliefRevisionInput(input); err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	if len(input.Decision.Proposals) == 0 || input.Decision.Proposals[0].Kind != cognition.ProposalRevision {
		return BeliefRevisionMaterialization{}, false, nil
	}
	proposal := input.Decision.Proposals[0].Revision
	entry, err := exactRevisionTarget(input, proposal.TargetRef)
	if err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	evidence := append([]cognition.EvidenceRef{}, proposal.EvidenceRefs...)
	sort.Slice(evidence, func(left, right int) bool {
		return evidenceRefKey(evidence[left]) < evidenceRefKey(evidence[right])
	})
	refs, err := contradictionRefs(input.Ledger, evidence)
	if err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	ledgerSHA, err := mappingDigest(input.Ledger)
	if err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	decisionSHA, err := mappingDigest(input.Decision.Clone())
	if err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	commandID, err := taskstate.NewCommandID(
		BeliefRevisionSchemaV1, input.Snapshot.SHA256(), decisionSHA,
		ledgerSHA, string(entry.ID),
	)
	if err != nil {
		return BeliefRevisionMaterialization{}, false, fmt.Errorf("%w: command identity: %v", ErrInvalidMapping, err)
	}
	command := taskstate.RejectEntryCommand{
		CommandID: commandID, ExpectedVersion: input.Ledger.Version,
		Actor: taskstate.AuthorityCode, EntryID: entry.ID,
		Reason: beliefRevisionReason, Refs: refs,
	}
	result, err := applyRevisionCommand(input.Ledger, command)
	if err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	resultSHA, err := mappingDigest(result)
	if err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	materialization := BeliefRevisionMaterialization{
		Schema: BeliefRevisionSchemaV1, SourceSnapshotSHA256: input.Snapshot.SHA256(),
		SourceDecisionSHA256: decisionSHA, LedgerID: input.Ledger.ID,
		ExpectedLedgerSHA256: ledgerSHA, ExpectedVersion: input.Ledger.Version,
		TargetRef: proposal.TargetRef, EvidenceRefs: evidence,
		Rejection: command, ResultLedgerSHA256: resultSHA,
	}
	materialization.SHA256, err = beliefRevisionSHA(materialization)
	if err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	materialization.ID = "cognition_revision_" + materialization.SHA256
	if err := materialization.Validate(); err != nil {
		return BeliefRevisionMaterialization{}, false, err
	}
	return materialization, true, nil
}

func (materialization BeliefRevisionMaterialization) Validate() error {
	if materialization.Schema != BeliefRevisionSchemaV1 ||
		materialization.ID != "cognition_revision_"+materialization.SHA256 ||
		!validMappingDigest(materialization.SHA256) ||
		!validMappingDigest(materialization.SourceSnapshotSHA256) ||
		!validMappingDigest(materialization.SourceDecisionSHA256) ||
		!validMappingDigest(materialization.ExpectedLedgerSHA256) ||
		!validMappingDigest(materialization.ResultLedgerSHA256) || materialization.ExpectedVersion == 0 {
		return fmt.Errorf("%w: belief revision identity is invalid", ErrInvalidMapping)
	}
	if err := materialization.TargetRef.Validate(); err != nil {
		return fmt.Errorf("%w: target: %v", ErrInvalidMapping, err)
	}
	if err := (&cognition.BeliefRevisionProposal{
		TargetRef: materialization.TargetRef, EvidenceRefs: materialization.EvidenceRefs,
	}).Validate(); err != nil {
		return fmt.Errorf("%w: evidence: %v", ErrInvalidMapping, err)
	}
	command := materialization.Rejection
	if command.ExpectedVersion != materialization.ExpectedVersion || command.Actor != taskstate.AuthorityCode ||
		command.Reason != beliefRevisionReason || len(command.Refs) != len(materialization.EvidenceRefs) {
		return fmt.Errorf("%w: rejection command authority is invalid", ErrInvalidMapping)
	}
	wantURI := "task:ledger/" + string(materialization.LedgerID) + "/entry/" + string(command.EntryID)
	if materialization.TargetRef.URI != wantURI {
		return fmt.Errorf("%w: target reference differs from the rejected entry", ErrInvalidMapping)
	}
	for index, ref := range materialization.EvidenceRefs {
		want := evidenceLedgerRef(ref)
		want.Relation = taskstate.RefContradicts
		if command.Refs[index] != want {
			return fmt.Errorf("%w: rejection evidence %d differs", ErrInvalidMapping, index)
		}
	}
	descriptor, err := taskstate.DescribeCommand(command)
	if err != nil || descriptor.Kind != taskstate.CommandRejectEntry || descriptor.Actor != taskstate.AuthorityCode {
		return fmt.Errorf("%w: rejection command is invalid", ErrInvalidMapping)
	}
	wantCommandID, err := taskstate.NewCommandID(
		BeliefRevisionSchemaV1, materialization.SourceSnapshotSHA256,
		materialization.SourceDecisionSHA256, materialization.ExpectedLedgerSHA256,
		string(command.EntryID),
	)
	if err != nil || command.CommandID != wantCommandID {
		return fmt.Errorf("%w: rejection command identity changed", ErrInvalidMapping)
	}
	want, err := beliefRevisionSHA(materialization)
	if err != nil || want != materialization.SHA256 {
		return fmt.Errorf("%w: belief revision hash changed", ErrInvalidMapping)
	}
	return nil
}

func ApplyBeliefRevision(
	state taskstate.MaterializedState,
	materialization BeliefRevisionMaterialization,
) (taskstate.MaterializedState, error) {
	if err := materialization.Validate(); err != nil {
		return taskstate.MaterializedState{}, err
	}
	stateSHA, err := mappingDigest(state)
	if err != nil || state.ID != materialization.LedgerID || state.Version != materialization.ExpectedVersion ||
		stateSHA != materialization.ExpectedLedgerSHA256 {
		return taskstate.MaterializedState{}, fmt.Errorf("%w: belief revision source ledger changed", ErrInvalidMapping)
	}
	result, err := applyRevisionCommand(state, materialization.Rejection)
	if err != nil {
		return taskstate.MaterializedState{}, err
	}
	resultSHA, err := mappingDigest(result)
	if err != nil || resultSHA != materialization.ResultLedgerSHA256 {
		return taskstate.MaterializedState{}, fmt.Errorf("%w: belief revision result changed", ErrInvalidMapping)
	}
	return result, nil
}

func validateBeliefRevisionInput(input ModelProposalInput) error {
	if err := taskstate.ValidateMaterializedState(input.Ledger); err != nil {
		return fmt.Errorf("%w: ledger: %v", ErrInvalidMapping, err)
	}
	if err := input.Snapshot.Validate(); err != nil {
		return fmt.Errorf("%w: snapshot: %v", ErrInvalidMapping, err)
	}
	schema, exists := input.Snapshot.ActionCatalog().Schema(input.Decision.Action.Kind)
	if !exists || schema.Ref() != input.ActionSchema.Ref() || input.Decision.Validate(input.ActionSchema) != nil {
		return fmt.Errorf("%w: decision or action schema is invalid", ErrInvalidMapping)
	}
	if input.ScopeNodeID != taskstate.NodeID(input.Snapshot.CurrentObligation().ID) ||
		input.Decision.ObligationID != input.Snapshot.CurrentObligation().ID {
		return fmt.Errorf("%w: revision scope differs from the current obligation", ErrInvalidMapping)
	}
	return nil
}

func exactRevisionTarget(input ModelProposalInput, ref cognition.EpistemicRef) (taskstate.Entry, error) {
	for _, entry := range input.Ledger.Entries {
		if entry.Status != taskstate.EntryActive || entry.Kind != taskstate.EntryHypothesis ||
			entry.Authority != taskstate.AuthorityModelProposal || entry.ScopeNodeID != input.ScopeNodeID {
			continue
		}
		if epistemicRef(input.Ledger.ID, entry) == ref {
			return entry, nil
		}
	}
	return taskstate.Entry{}, fmt.Errorf("%w: target is not an exact active model hypothesis on the current obligation", ErrInvalidMapping)
}

func epistemicRef(ledgerID taskstate.LedgerID, entry taskstate.Entry) cognition.EpistemicRef {
	return cognition.EpistemicRef{
		URI:     "task:ledger/" + string(ledgerID) + "/entry/" + string(entry.ID),
		Version: strconv.FormatUint(entry.UpdatedVersion, 10), SHA256: entry.ContentSHA256,
	}
}

func contradictionRefs(
	ledger taskstate.MaterializedState,
	evidence []cognition.EvidenceRef,
) ([]taskstate.Ref, error) {
	available := toolObservationEvidence(ledger.Entries)
	refs := make([]taskstate.Ref, len(evidence))
	for index, ref := range evidence {
		ledgerRef := evidenceLedgerRef(ref)
		if hash, exists := available[taskstate.RefIdentity(ledgerRef)]; !exists || hash != ledgerRef.Hash {
			return nil, fmt.Errorf("%w: revision evidence %d is not an active tool observation", ErrImmutableEvidence, index)
		}
		ledgerRef.Relation = taskstate.RefContradicts
		refs[index] = ledgerRef
	}
	sort.Slice(refs, func(left, right int) bool {
		return taskstate.RefIdentity(refs[left]) < taskstate.RefIdentity(refs[right])
	})
	return refs, nil
}

func applyRevisionCommand(
	state taskstate.MaterializedState,
	command taskstate.RejectEntryCommand,
) (taskstate.MaterializedState, error) {
	ledger, err := taskstate.RestoreLedger(state)
	if err != nil {
		return taskstate.MaterializedState{}, fmt.Errorf("%w: restore ledger: %v", ErrInvalidMapping, err)
	}
	if _, err := ledger.Apply(command); err != nil {
		return taskstate.MaterializedState{}, fmt.Errorf("%w: apply rejection: %v", ErrInvalidMapping, err)
	}
	return ledger.MaterializedState(), nil
}

func beliefRevisionSHA(materialization BeliefRevisionMaterialization) (string, error) {
	materialization.ID, materialization.SHA256 = "", ""
	return mappingDigest(materialization)
}
