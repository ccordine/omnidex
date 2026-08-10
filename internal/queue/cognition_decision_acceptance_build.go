package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func newCognitionDecisionAcceptanceTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	policyCallID string,
	prepared cognitionruntime.PreparedSnapshot,
	command cognitionruntime.ReconciliationCommand,
	candidateID taskstate.EntryID,
	ledgerVersion uint64,
) (cognitionDecisionAcceptance, taskstate.AcceptDecisionCommand, error) {
	var resultSHA string
	if err := tx.QueryRow(ctx, `
		SELECT result_sha256 FROM cognition_policy_calls
		WHERE call_id=$1 AND episode_id=$2 AND status='accepted'
	`, policyCallID, episode.EpisodeID).Scan(&resultSHA); err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	refs := cognitionEvidenceTaskRefs(command.Decision.EvidenceRefs)
	refs = append(refs,
		taskstate.Ref{
			URI: "cognition:policy-call/" + policyCallID, Version: "accepted",
			Hash: resultSHA, Relation: taskstate.RefVerifies,
		},
		taskstate.Ref{
			URI:     "cognition:action-schema/" + string(command.ActionSchema.ID),
			Version: command.ActionSchema.Version, Hash: command.ActionSchema.SHA256,
			Relation: taskstate.RefVerifies,
		},
	)
	decisionSHA, err := cognitionruntime.DecisionSHA256(command.Decision)
	if err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	seed := struct {
		Schema, PolicyCallID, SnapshotSHA256, DecisionSHA256 string
		Candidate                                            taskstate.EntryID
		ActionSchema                                         cognition.ActionSchemaRef
		Refs                                                 []taskstate.Ref
	}{
		cognitionDecisionAcceptanceSchemaV1, policyCallID, prepared.Snapshot.SHA256(),
		decisionSHA, candidateID, command.ActionSchema.Ref(), refs,
	}
	_, digest, err := cognitionJSON(seed)
	if err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	acceptedID := taskstate.EntryID(cognitionEntryPrefix + digest)
	commandID, err := cognitionTaskCommandID(
		cognitionDecisionAcceptanceSchemaV1, digest, string(candidateID), string(acceptedID),
	)
	if err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	metadataRaw, _, err := cognitionJSON(struct {
		Schema         string                    `json:"schema"`
		PolicyCallID   string                    `json:"policy_call_id"`
		SnapshotSHA256 string                    `json:"snapshot_sha256"`
		DecisionSHA256 string                    `json:"decision_sha256"`
		ActionSchema   cognition.ActionSchemaRef `json:"action_schema"`
	}{cognitionDecisionAcceptanceSchemaV1, policyCallID, prepared.Snapshot.SHA256(), decisionSHA, command.ActionSchema.Ref()})
	if err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	metadata, err := taskstate.NewJSONObject(metadataRaw)
	if err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	stepID := episode.Authority.StepID
	accept := taskstate.AcceptDecisionCommand{
		CommandID: commandID, ExpectedVersion: ledgerVersion, Actor: taskstate.AuthorityCode,
		CandidateID: candidateID, AcceptedEntryID: acceptedID,
		AcceptancePolicy: cognitionDecisionAcceptancePolicyV1,
		AcceptanceRefs:   refs, CreatedStepID: &stepID, Metadata: metadata,
	}
	descriptor, err := taskstate.DescribeCommand(accept)
	if err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	value := cognitionDecisionAcceptance{
		Schema: cognitionDecisionAcceptanceSchemaV1, LedgerID: episode.LedgerID,
		CandidateEntryID: candidateID,
		AcceptedEntryID:  acceptedID, PolicyCallID: policyCallID,
		SnapshotSHA256: prepared.Snapshot.SHA256(), DecisionSHA256: decisionSHA,
		ActionSchema: command.ActionSchema.Ref(), AcceptanceRefs: append([]taskstate.Ref{}, refs...),
		AcceptanceCommandID: commandID, AcceptanceCommandSHA: descriptor.SHA256,
	}
	_, value.SHA256, err = cognitionJSON(value.identity())
	if err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	value.ID = "cognition_decision_acceptance_" + value.SHA256
	if err := value.validate(); err != nil {
		return cognitionDecisionAcceptance{}, taskstate.AcceptDecisionCommand{}, err
	}
	return value, accept, nil
}

func (value cognitionDecisionAcceptance) identity() any {
	return struct {
		Schema               string                    `json:"schema"`
		LedgerID             taskstate.LedgerID        `json:"ledger_id"`
		CandidateEntryID     taskstate.EntryID         `json:"candidate_entry_id"`
		AcceptedEntryID      taskstate.EntryID         `json:"accepted_entry_id"`
		PolicyCallID         string                    `json:"policy_call_id"`
		SnapshotSHA256       string                    `json:"snapshot_sha256"`
		DecisionSHA256       string                    `json:"decision_sha256"`
		ActionSchema         cognition.ActionSchemaRef `json:"action_schema"`
		AcceptanceRefs       []taskstate.Ref           `json:"acceptance_refs"`
		AcceptanceCommandID  taskstate.CommandID       `json:"acceptance_command_id"`
		AcceptanceCommandSHA string                    `json:"acceptance_command_sha256"`
	}{value.Schema, value.LedgerID, value.CandidateEntryID, value.AcceptedEntryID, value.PolicyCallID,
		value.SnapshotSHA256, value.DecisionSHA256, value.ActionSchema,
		append([]taskstate.Ref{}, value.AcceptanceRefs...), value.AcceptanceCommandID,
		value.AcceptanceCommandSHA}
}

func (value cognitionDecisionAcceptance) validate() error {
	_, sha, err := cognitionJSON(value.identity())
	if err != nil || value.Schema != cognitionDecisionAcceptanceSchemaV1 ||
		value.ID != "cognition_decision_acceptance_"+value.SHA256 || value.SHA256 != sha ||
		value.LedgerID == "" || value.CandidateEntryID == "" || value.AcceptedEntryID == "" ||
		value.PolicyCallID == "" || !cognitionDigestPattern.MatchString(value.SnapshotSHA256) ||
		!cognitionDigestPattern.MatchString(value.DecisionSHA256) || value.ActionSchema.Validate() != nil ||
		len(value.AcceptanceRefs) < 2 || value.AcceptanceCommandID == "" ||
		!cognitionDigestPattern.MatchString(value.AcceptanceCommandSHA) {
		return fmt.Errorf("%w: selected decision acceptance is invalid", ErrCognitionConflict)
	}
	return nil
}
