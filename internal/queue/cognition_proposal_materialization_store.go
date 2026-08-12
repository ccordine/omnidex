package queue

import (
	"bytes"
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func insertCognitionProposalMaterializationsTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	values []CognitionProposalMaterialization,
) error {
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		identityRaw, err := exactjson.Canonical(value.identity())
		if err != nil || cognitionPayloadSHA(identityRaw) != value.SHA256 {
			return fmt.Errorf("%w: proposal materialization identity changed", ErrCognitionConflict)
		}
		payloadRaw, err := exactjson.Canonical(value)
		if err != nil {
			return err
		}
		ledgerRaw, err := exactjson.Canonical(value.PreProposalLedger)
		if err != nil || cognitionPayloadSHA(ledgerRaw) != value.PreProposalLedgerJSONSHA256 {
			return fmt.Errorf("%w: proposal materialization ledger changed", ErrCognitionConflict)
		}
		payloadSHA := cognitionPayloadSHA(payloadRaw)
		_, err = tx.Exec(ctx, `
			INSERT INTO cognition_proposal_materializations (
				materialization_id,materialization_sha256,episode_id,job_id,generation,step_id,
				actor_attempt,actor_worker_id,reconciliation_id,policy_call_id,call_ordinal,
				snapshot_sha256,decision_sha256,proposal_index,proposal_kind,proposal_json,
				source_kind,ledger_id,pre_proposal_ledger_version,pre_proposal_ledger_sha256,
				pre_proposal_ledger_json,pre_proposal_ledger_json_sha256,
				mapping_id,mapping_sha256,command_id,command_sha256,entry_id,entry_uri,
				output_ledger_version,output_ledger_status,identity_json,identity_json_sha256,
				payload_json,payload_json_sha256
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
				$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34
			)
		`, value.ID, value.SHA256, value.EpisodeID, authority.JobID, authority.Generation,
			authority.StepID, authority.Attempt, authority.WorkerID, value.ReconciliationID,
			value.PolicyCallID, int64(value.CallOrdinal), value.SnapshotSHA256, value.DecisionSHA256,
			value.ProposalIndex, value.Proposal.Kind, value.Proposal, value.SourceKind,
			value.ReplayDescriptor.LedgerID, int64(value.PreProposalLedgerVersion),
			value.PreProposalLedgerSHA256, string(ledgerRaw), value.PreProposalLedgerJSONSHA256,
			value.ReplayDescriptor.ID, value.ReplayDescriptor.SHA256,
			value.Command.CommandID, value.ReplayDescriptor.CommandSHA256, value.Command.ID,
			value.EntryURI, int64(value.OutputLedgerVersion), value.OutputLedgerStatus,
			string(identityRaw), value.SHA256, string(payloadRaw), payloadSHA)
		if err != nil {
			return fmt.Errorf("insert cognition proposal materialization: %w", err)
		}
	}
	return nil
}

func requireCognitionProposalMaterializationReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	prepared CognitionRuntimeSnapshotRecord,
	command cognitionruntime.ReconciliationCommand,
	receipt cognitionruntime.ReconciliationReceipt,
) error {
	values, raws, err := loadCognitionProposalMaterializationRowsTx(ctx, tx, receipt.ID)
	if err != nil {
		return err
	}
	expectedCount := 0
	for _, proposal := range command.Decision.Proposals {
		if proposal.Kind != cognition.ProposalRevision {
			expectedCount++
		}
	}
	if len(values) != expectedCount {
		return fmt.Errorf("%w: reconciliation proposal materialization count changed", ErrCognitionConflict)
	}
	if len(values) == 0 {
		return nil
	}
	var policyCallID string
	if err := tx.QueryRow(ctx, `
		SELECT policy_call_id FROM cognition_reconciliations WHERE reconciliation_id=$1
	`, receipt.ID).Scan(&policyCallID); err != nil {
		return err
	}
	if values[0].PolicyCallID != policyCallID || values[0].CallOrdinal != prepared.CallOrdinal {
		return fmt.Errorf("%w: proposal materialization call authority changed", ErrCognitionConflict)
	}
	preLedger, err := loadTaskLedgerAtVersionTx(
		ctx, tx, episode.Authority.JobID, values[0].PreProposalLedgerVersion,
	)
	if err != nil {
		return err
	}
	input := cognitionstate.ModelProposalInput{
		Ledger: preLedger, ScopeNodeID: taskstate.NodeID(command.Decision.ObligationID),
		Snapshot: prepared.Prepared.Snapshot, Decision: command.Decision, ActionSchema: command.ActionSchema,
	}
	want, err := newCognitionProposalMaterializations(
		episode.EpisodeID, policyCallID, prepared.CallOrdinal, input, receipt,
	)
	if err != nil || len(want) != len(values) {
		return fmt.Errorf("%w: rederive proposal materialization replay: %v", ErrCognitionConflict, err)
	}
	for index := range want {
		wantRaw, marshalErr := exactjson.Canonical(want[index])
		if marshalErr != nil || !reflect.DeepEqual(values[index], want[index]) ||
			!bytes.Equal(raws[index], wantRaw) {
			return fmt.Errorf("%w: proposal materialization replay changed index %d", ErrCognitionConflict, index)
		}
	}
	return nil
}

func loadCognitionProposalMaterializationRowsTx(
	ctx context.Context,
	tx pgx.Tx,
	reconciliationID string,
) ([]CognitionProposalMaterialization, [][]byte, error) {
	rows, err := tx.Query(ctx, `
		SELECT payload_json,payload_json_sha256
		FROM cognition_proposal_materializations
		WHERE reconciliation_id=$1 ORDER BY proposal_index
	`, reconciliationID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	values := make([]CognitionProposalMaterialization, 0)
	raws := make([][]byte, 0)
	for rows.Next() {
		var raw []byte
		var payloadSHA string
		if err := rows.Scan(&raw, &payloadSHA); err != nil {
			return nil, nil, err
		}
		value, err := DecodeCognitionProposalMaterialization(raw, payloadSHA)
		if err != nil {
			return nil, nil, err
		}
		values = append(values, value)
		raws = append(raws, append([]byte(nil), raw...))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return values, raws, nil
}

func cognitionProposalMaterializationForTrace(
	values []CognitionProposalMaterialization,
	id string,
) (CognitionProposalMaterialization, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return CognitionProposalMaterialization{}, false
}
