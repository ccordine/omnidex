package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const cognitionObligationMaterializationPolicyV1 = "cognition-obligation-materialization-v1"
const cognitionPlanRevisionMaterializationPolicyV1 = "cognition-plan-revision-materialization-v1"
const cognitionProposalFailurePolicyV1 = "cognition-proposal-action-failure-v1"
const cognitionProposalTerminalPolicyV1 = "cognition-proposal-terminal-transition-v1"

func persistCognitionProposalDispositionTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	episode CognitionEpisode,
	record CognitionActionRecord,
	link cognitionActionGraphMaterialization,
	outcome cognitionProposalOutcome,
	proof taskstate.Ref,
	authority model.StepAttemptAuthority,
) (taskLedgerHeader, error) {
	restored, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return header, err
	}
	ledger := restored.MaterializedState()
	var candidate taskstate.Entry
	for _, entry := range ledger.Entries {
		if entry.ID == link.Candidate {
			candidate = entry
			break
		}
	}
	if candidate.ID == "" || candidate.Status != taskstate.EntryActive ||
		candidate.Kind != taskstate.EntryDecisionCandidate || candidate.Authority != taskstate.AuthorityModelProposal {
		return header, fmt.Errorf("%w: materialized proposal candidate is not active", ErrCognitionConflict)
	}
	value := cognitionProposalDisposition{
		Schema: cognitionProposalDispositionSchemaV1, EpisodeID: episode.EpisodeID,
		LedgerID: header.ID, ReconciliationID: record.ReconciliationID, ActionID: record.Action.ID,
		CandidateEntryID: candidate.ID, ProposalKind: link.Kind,
		Outcome: outcome, ProofRef: proof,
	}
	descriptorID, descriptorSHA, proposalIndex, err := link.descriptor()
	if err != nil {
		return header, err
	}
	value.ProposalIndex = proposalIndex
	value.SourceDescriptorID, value.SourceDescriptorSHA = descriptorID, descriptorSHA
	var command taskstate.Command
	if outcome == cognitionProposalAcceptedMaterialization {
		refs := append([]taskstate.Ref{}, candidate.Refs...)
		descriptorURI := "cognition:obligation-materialization/" + descriptorID
		acceptancePolicy := cognitionObligationMaterializationPolicyV1
		if link.Kind == cognition.ProposalPlanRevision {
			descriptorURI = "cognition:plan-revision/" + descriptorID
			acceptancePolicy = cognitionPlanRevisionMaterializationPolicyV1
		}
		refs = append(refs, taskstate.Ref{
			URI: descriptorURI, Version: "applied", Hash: descriptorSHA, Relation: taskstate.RefVerifies,
		}, proof)
		seedRaw, _, err := cognitionJSON(struct {
			Schema          string             `json:"schema"`
			Candidate       string             `json:"candidate_entry_id"`
			Materialization string             `json:"materialization_id"`
			Action          cognition.ActionID `json:"action_id"`
		}{cognitionProposalDispositionSchemaV1, string(candidate.ID), descriptorID, record.Action.ID})
		if err != nil {
			return header, err
		}
		metadata, err := taskstate.NewJSONObject(seedRaw)
		if err != nil {
			return header, err
		}
		_, digest, err := cognitionJSON(struct {
			Candidate taskstate.EntryID
			Source    string
			Action    cognition.ActionID
		}{candidate.ID, descriptorID, record.Action.ID})
		if err != nil {
			return header, err
		}
		value.ResultEntryID = taskstate.EntryID(cognitionEntryPrefix + digest)
		command = taskstate.AcceptDecisionCommand{
			ExpectedVersion: ledger.Version, Actor: taskstate.AuthorityCode,
			CandidateID: candidate.ID, AcceptedEntryID: value.ResultEntryID,
			AcceptancePolicy: acceptancePolicy,
			AcceptanceRefs:   refs, CreatedStepID: &episode.Authority.StepID, Metadata: metadata,
		}
	} else {
		reason := cognitionProposalFailurePolicyV1
		if outcome == cognitionProposalRejectedTerminal {
			reason = cognitionProposalTerminalPolicyV1
		}
		command = taskstate.RejectEntryCommand{
			ExpectedVersion: ledger.Version, Actor: taskstate.AuthorityCode,
			EntryID: candidate.ID, Reason: reason, Refs: []taskstate.Ref{proof},
		}
	}
	commandID, err := cognitionTaskCommandID(
		cognitionProposalDispositionSchemaV1, string(candidate.ID), string(outcome), record.ReconciliationID,
	)
	if err != nil {
		return header, err
	}
	switch typed := command.(type) {
	case taskstate.AcceptDecisionCommand:
		typed.CommandID = commandID
		command = typed
	case taskstate.RejectEntryCommand:
		typed.CommandID = commandID
		command = typed
	}
	descriptor, err := taskstate.DescribeCommand(command)
	if err != nil {
		return header, err
	}
	value.CommandID, value.CommandSHA256 = commandID, descriptor.SHA256
	_, value.SHA256, err = cognitionJSON(value.identity())
	if err != nil {
		return header, err
	}
	value.ID = "cognition_proposal_disposition_" + value.SHA256
	if err := value.validate(); err != nil {
		return header, err
	}
	event, err := applyQueueOwnedTaskCommandTx(ctx, tx, authority.JobID, authority.Generation, command)
	if err != nil {
		return header, fmt.Errorf("persist cognition proposal disposition: %w", err)
	}
	header.Version = event.Version
	if err := insertCognitionProposalDispositionTx(ctx, tx, value); err != nil {
		return header, err
	}
	return header, nil
}
