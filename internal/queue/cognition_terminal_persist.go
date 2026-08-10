package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type cognitionTerminalSealWriteAuthority struct {
	Kind                 string
	Owner                model.StepAttemptAuthority
	LifecycleOperationID LifecycleOperationID
}

func workerCognitionTerminalAuthority(owner model.StepAttemptAuthority) cognitionTerminalSealWriteAuthority {
	return cognitionTerminalSealWriteAuthority{Kind: cognitionTerminalAuthorityWorker, Owner: owner}
}

func lifecycleCognitionTerminalAuthority(
	retirement cognitionLifecycleRetirement,
) cognitionTerminalSealWriteAuthority {
	return cognitionTerminalSealWriteAuthority{
		Kind: cognitionTerminalAuthorityLifecycle,
		Owner: model.StepAttemptAuthority{
			JobID: retirement.JobID, Generation: retirement.JobGeneration, StepID: retirement.StepID,
		},
		LifecycleOperationID: retirement.OperationID,
	}
}

func persistCognitionTerminalSealTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	outcome CognitionEpisodeStatus,
	revision cognition.WorldRevision,
	completion cognition.CompletionResult,
	graph cognition.ObligationGraphSnapshot,
	ledgerVersion, workingVersion uint64,
	traceJSON []byte,
	traceSHA, publicOutcome string,
	authority cognitionTerminalSealWriteAuthority,
) error {
	if err := validateCognitionTerminalWriteAuthority(authority, outcome); err != nil {
		return err
	}
	completionJSON, completionSHA, err := cognitionJSON(completion)
	if err != nil {
		return err
	}
	var attempt any
	var worker any
	var operation any
	if authority.Kind == cognitionTerminalAuthorityWorker {
		attempt, worker = authority.Owner.Attempt, authority.Owner.WorkerID
	} else {
		operation = authority.LifecycleOperationID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_terminal_seals (
		 episode_id,job_id,generation,step_id,final_revision,final_revision_sha256,
		 outcome,completion_json,completion_sha256,obligation_graph_sha256,
		 ledger_version,working_set_version,trace_json,trace_sha256,
		 authority_kind,sealed_attempt,sealed_worker_id,lifecycle_operation_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`, episodeID, authority.Owner.JobID, authority.Owner.Generation, authority.Owner.StepID,
		int64(revision.Number), revision.SHA256, outcome, string(completionJSON), completionSHA,
		graph.SHA256, int64(ledgerVersion), int64(workingVersion), string(traceJSON), traceSHA,
		authority.Kind, attempt, worker, operation); err != nil {
		return fmt.Errorf("insert cognition terminal seal: %w", err)
	}
	result, err := tx.Exec(ctx, `
		UPDATE cognition_episodes SET status=$2,terminal_outcome=$3,terminal_at=clock_timestamp(),
		       version=version+1,updated_at=clock_timestamp()
		WHERE episode_id=$1 AND status='active' AND current_revision=$4
	`, episodeID, outcome, publicOutcome, int64(revision.Number))
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: cognition episode changed during terminal seal", ErrCognitionConflict)
	}
	return nil
}

func validateCognitionTerminalWriteAuthority(
	authority cognitionTerminalSealWriteAuthority,
	outcome CognitionEpisodeStatus,
) error {
	switch authority.Kind {
	case cognitionTerminalAuthorityWorker:
		if authority.LifecycleOperationID != "" {
			return fmt.Errorf("worker terminal authority contains lifecycle identity")
		}
		return validateStepAttemptAuthority(authority.Owner)
	case cognitionTerminalAuthorityLifecycle:
		if outcome != CognitionEpisodeCanceled || authority.LifecycleOperationID == "" ||
			authority.Owner.JobID <= 0 || authority.Owner.Generation <= 0 || authority.Owner.StepID <= 0 ||
			authority.Owner.Attempt != 0 || authority.Owner.WorkerID != "" {
			return fmt.Errorf("lifecycle terminal authority is invalid")
		}
		_, err := ParseLifecycleOperationID(string(authority.LifecycleOperationID))
		return err
	default:
		return fmt.Errorf("unregistered cognition terminal authority %q", authority.Kind)
	}
}
