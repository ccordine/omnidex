package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const cognitionDecisionAcceptanceSchemaV1 = "omnidex.cognition-decision-acceptance.v1"
const cognitionDecisionAcceptancePolicyV1 = "cognition-policy-call-and-action-schema-v1"

type cognitionDecisionAcceptance struct {
	Schema               string                    `json:"schema"`
	ID                   string                    `json:"id"`
	SHA256               string                    `json:"sha256"`
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
}

func persistCognitionSelectedDecisionTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episode CognitionEpisode,
	policyCallID string,
	prepared cognitionruntime.PreparedSnapshot,
	command cognitionruntime.ReconciliationCommand,
	ledger taskstate.MaterializedState,
) (taskstate.MaterializedState, cognitionDecisionAcceptance, error) {
	input := cognitionstate.ModelProposalInput{
		Ledger: ledger, ScopeNodeID: taskstate.NodeID(command.Decision.ObligationID),
		Snapshot: prepared.Snapshot, Decision: command.Decision, ActionSchema: command.ActionSchema,
	}
	candidate, err := cognitionstate.MapSelectedDecisionCandidate(input)
	if err != nil {
		return taskstate.MaterializedState{}, cognitionDecisionAcceptance{}, err
	}
	if _, err := applyQueueOwnedTaskCommandTx(
		ctx, tx, authority.JobID, authority.Generation, candidate.Command(),
	); err != nil {
		return taskstate.MaterializedState{}, cognitionDecisionAcceptance{}, fmt.Errorf("persist selected decision candidate: %w", err)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, false)
	if err != nil {
		return taskstate.MaterializedState{}, cognitionDecisionAcceptance{}, err
	}
	restored, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return taskstate.MaterializedState{}, cognitionDecisionAcceptance{}, err
	}
	acceptance, acceptCommand, err := newCognitionDecisionAcceptanceTx(
		ctx, tx, episode, policyCallID, prepared, command, candidate.Descriptor().EntryID,
		restored.MaterializedState().Version,
	)
	if err != nil {
		return taskstate.MaterializedState{}, cognitionDecisionAcceptance{}, err
	}
	if _, err := applyQueueOwnedTaskCommandTx(
		ctx, tx, authority.JobID, authority.Generation, acceptCommand,
	); err != nil {
		return taskstate.MaterializedState{}, cognitionDecisionAcceptance{}, fmt.Errorf("accept selected cognition decision: %w", err)
	}
	header, err = loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, false)
	if err != nil {
		return taskstate.MaterializedState{}, cognitionDecisionAcceptance{}, err
	}
	restored, err = restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return taskstate.MaterializedState{}, cognitionDecisionAcceptance{}, err
	}
	return restored.MaterializedState(), acceptance, nil
}
