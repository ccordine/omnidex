package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

const cognitionProposalDispositionSchemaV1 = "omnidex.cognition-proposal-disposition.v1"

type cognitionProposalOutcome string

const (
	cognitionProposalAcceptedMaterialization cognitionProposalOutcome = "accepted_materialization"
	cognitionProposalRejectedFailure         cognitionProposalOutcome = "rejected_action_failure"
	cognitionProposalRejectedTerminal        cognitionProposalOutcome = "rejected_terminal_transition"
)

type cognitionProposalDisposition struct {
	Schema              string                       `json:"schema"`
	ID                  string                       `json:"id"`
	SHA256              string                       `json:"sha256"`
	EpisodeID           cognition.EpisodeID          `json:"episode_id"`
	LedgerID            taskstate.LedgerID           `json:"ledger_id"`
	ReconciliationID    string                       `json:"reconciliation_id"`
	ActionID            cognition.ActionID           `json:"action_id"`
	CandidateEntryID    taskstate.EntryID            `json:"candidate_entry_id"`
	ProposalKind        cognition.LedgerProposalKind `json:"proposal_kind"`
	ProposalIndex       int                          `json:"proposal_index"`
	SourceDescriptorID  string                       `json:"source_descriptor_id"`
	SourceDescriptorSHA string                       `json:"source_descriptor_sha256"`
	Outcome             cognitionProposalOutcome     `json:"outcome"`
	ProofRef            taskstate.Ref                `json:"proof_ref"`
	ResultEntryID       taskstate.EntryID            `json:"result_entry_id,omitempty"`
	CommandID           taskstate.CommandID          `json:"command_id"`
	CommandSHA256       string                       `json:"command_sha256"`
}

func (value cognitionProposalDisposition) identity() any {
	return struct {
		Schema              string                       `json:"schema"`
		EpisodeID           cognition.EpisodeID          `json:"episode_id"`
		LedgerID            taskstate.LedgerID           `json:"ledger_id"`
		ReconciliationID    string                       `json:"reconciliation_id"`
		ActionID            cognition.ActionID           `json:"action_id"`
		CandidateEntryID    taskstate.EntryID            `json:"candidate_entry_id"`
		ProposalKind        cognition.LedgerProposalKind `json:"proposal_kind"`
		ProposalIndex       int                          `json:"proposal_index"`
		SourceDescriptorID  string                       `json:"source_descriptor_id"`
		SourceDescriptorSHA string                       `json:"source_descriptor_sha256"`
		Outcome             cognitionProposalOutcome     `json:"outcome"`
		ProofRef            taskstate.Ref                `json:"proof_ref"`
		ResultEntryID       taskstate.EntryID            `json:"result_entry_id,omitempty"`
		CommandID           taskstate.CommandID          `json:"command_id"`
		CommandSHA256       string                       `json:"command_sha256"`
	}{value.Schema, value.EpisodeID, value.LedgerID, value.ReconciliationID,
		value.ActionID, value.CandidateEntryID, value.ProposalKind, value.ProposalIndex,
		value.SourceDescriptorID, value.SourceDescriptorSHA, value.Outcome, value.ProofRef,
		value.ResultEntryID, value.CommandID, value.CommandSHA256}
}

func (value cognitionProposalDisposition) validate() error {
	_, digest, err := cognitionJSON(value.identity())
	accepted := value.Outcome == cognitionProposalAcceptedMaterialization
	if err != nil || value.Schema != cognitionProposalDispositionSchemaV1 ||
		value.ID != "cognition_proposal_disposition_"+value.SHA256 || value.SHA256 != digest ||
		value.EpisodeID == "" || value.LedgerID == "" || value.ReconciliationID == "" ||
		value.ActionID == "" || value.CandidateEntryID == "" ||
		(value.ProposalKind != cognition.ProposalObligation &&
			value.ProposalKind != cognition.ProposalPlanRevision) || value.ProposalIndex < 0 ||
		value.SourceDescriptorID == "" || !cognitionDigestPattern.MatchString(value.SourceDescriptorSHA) ||
		(value.Outcome != cognitionProposalAcceptedMaterialization &&
			value.Outcome != cognitionProposalRejectedFailure && value.Outcome != cognitionProposalRejectedTerminal) ||
		value.ProofRef.URI == "" || value.ProofRef.Version == "" ||
		!cognitionDigestPattern.MatchString(value.ProofRef.Hash) || value.ProofRef.Relation != taskstate.RefVerifies ||
		value.CommandID == "" || !cognitionDigestPattern.MatchString(value.CommandSHA256) ||
		(accepted && value.ResultEntryID == "") || (!accepted && value.ResultEntryID != "") {
		return fmt.Errorf("%w: cognition proposal disposition is invalid", ErrCognitionConflict)
	}
	return nil
}
