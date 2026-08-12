package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertCognitionAcceptedFactMaterializationTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	value CognitionAcceptedFactMaterialization,
) error {
	if err := value.Validate(); err != nil {
		return err
	}
	identityRaw, err := exactjson.Canonical(value.identity())
	if err != nil || cognitionPayloadSHA(identityRaw) != value.SHA256 {
		return fmt.Errorf("%w: accepted-fact materialization identity changed", ErrCognitionConflict)
	}
	payloadRaw, err := exactjson.Canonical(value)
	if err != nil {
		return err
	}
	ledgerRaw, err := exactjson.Canonical(value.PreFactLedger)
	if err != nil || cognitionPayloadSHA(ledgerRaw) != value.PreFactLedgerJSONSHA256 {
		return fmt.Errorf("%w: accepted-fact materialization ledger changed", ErrCognitionConflict)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_accepted_fact_materializations (
			materialization_id,materialization_sha256,episode_id,job_id,generation,step_id,
			actor_attempt,actor_worker_id,ledger_id,transition_id,transition_sha256,
			transition_revision,action_id,call_ordinal,scope_obligation_id,authority_sha256,
			pre_fact_ledger_version,pre_fact_ledger_sha256,pre_fact_ledger_json,
			pre_fact_ledger_json_sha256,member_count,output_ledger_version,
			output_ledger_status,identity_json,identity_json_sha256,payload_json,payload_json_sha256
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
			$21,$22,$23,$24,$25,$26,$27
		)
	`, value.ID, value.SHA256, value.EpisodeID,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt, authority.WorkerID,
		value.LedgerID, value.TransitionID, value.TransitionSHA256,
		int64(value.TransitionRevision), nullableCognitionActionID(value.ActionID), int64(value.CallOrdinal),
		value.ScopeObligationID, value.FactAuthority.SHA256,
		int64(value.PreFactLedgerVersion), value.PreFactLedgerSHA256, string(ledgerRaw),
		value.PreFactLedgerJSONSHA256, len(value.Members), int64(value.OutputLedgerVersion),
		value.OutputLedgerStatus, string(identityRaw), value.SHA256,
		string(payloadRaw), cognitionPayloadSHA(payloadRaw))
	if err != nil {
		return fmt.Errorf("insert cognition accepted-fact materialization: %w", err)
	}
	for _, member := range value.Members {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_accepted_fact_materialization_members (
				materialization_id,position,fact_id,fact_sha256,command_id,command_sha256,
				entry_id,entry_uri,output_ledger_version,output_ledger_status
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, value.ID, member.Index, member.Fact.ID, member.Fact.SHA256,
			member.Command.CommandID, member.Fact.Mapping.CommandSHA256, member.Command.ID,
			member.EntryURI, int64(member.OutputLedgerVersion), member.OutputLedgerStatus); err != nil {
			return fmt.Errorf("insert cognition accepted-fact materialization member: %w", err)
		}
	}
	return nil
}

func nullableCognitionActionID(value cognition.ActionID) any {
	if value == "" {
		return nil
	}
	return value
}
