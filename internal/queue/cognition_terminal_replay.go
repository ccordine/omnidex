package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func loadCognitionTerminalSealTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
) (CognitionTerminalSeal, error) {
	var seal CognitionTerminalSeal
	var revision int64
	var sealedAttempt *int64
	var sealedWorker, lifecycleOperation *string
	err := tx.QueryRow(ctx, `
		SELECT episode_id,outcome,final_revision,final_revision_sha256,completion_sha256,
		       obligation_graph_sha256,ledger_version,working_set_version,trace_sha256,
		       job_id,generation,step_id,authority_kind,sealed_attempt,sealed_worker_id,
		       lifecycle_operation_id,created_at
		FROM cognition_terminal_seals WHERE episode_id=$1
	`, episodeID).Scan(
		&seal.EpisodeID, &seal.Outcome, &revision, &seal.FinalRevision.SHA256,
		&seal.CompletionSHA256, &seal.ObligationGraphSHA256, &seal.LedgerVersion,
		&seal.WorkingSetVersion, &seal.TraceSHA256, &seal.SealedBy.JobID,
		&seal.SealedBy.Generation, &seal.SealedBy.StepID, &seal.AuthorityKind,
		&sealedAttempt, &sealedWorker, &lifecycleOperation, &seal.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CognitionTerminalSeal{}, fmt.Errorf("%w: terminal cognition episode has no seal", ErrCognitionConflict)
	}
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	seal.FinalRevision = cognition.WorldRevision{
		EpisodeID: episodeID, Number: uint64(revision), SHA256: seal.FinalRevision.SHA256,
	}
	if sealedAttempt != nil {
		seal.SealedBy.Attempt = *sealedAttempt
	}
	if sealedWorker != nil {
		seal.SealedBy.WorkerID = *sealedWorker
	}
	if lifecycleOperation != nil {
		seal.LifecycleOperationID = LifecycleOperationID(*lifecycleOperation)
	}
	if err := seal.FinalRevision.Validate(); err != nil || !validCognitionTerminalStatus(seal.Outcome) ||
		!cognitionDigestPattern.MatchString(seal.CompletionSHA256) ||
		!cognitionDigestPattern.MatchString(seal.ObligationGraphSHA256) ||
		!cognitionDigestPattern.MatchString(seal.TraceSHA256) || validateCognitionTerminalSealAuthority(seal) != nil {
		return CognitionTerminalSeal{}, fmt.Errorf("%w: persisted cognition terminal seal is invalid", ErrCognitionConflict)
	}
	return seal, nil
}

func validateCognitionTerminalSealAuthority(seal CognitionTerminalSeal) error {
	switch seal.AuthorityKind {
	case cognitionTerminalAuthorityWorker:
		if seal.LifecycleOperationID != "" || validateStepAttemptAuthority(seal.SealedBy) != nil {
			return fmt.Errorf("worker cognition terminal authority is invalid")
		}
	case cognitionTerminalAuthorityLifecycle:
		if seal.Outcome != CognitionEpisodeCanceled || seal.LifecycleOperationID == "" ||
			seal.SealedBy.JobID <= 0 || seal.SealedBy.Generation <= 0 || seal.SealedBy.StepID <= 0 ||
			seal.SealedBy.Attempt != 0 || seal.SealedBy.WorkerID != "" {
			return fmt.Errorf("lifecycle cognition terminal authority is invalid")
		}
		if _, err := ParseLifecycleOperationID(string(seal.LifecycleOperationID)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unregistered cognition terminal authority %q", seal.AuthorityKind)
	}
	return nil
}

func requireCognitionTerminalReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	command CognitionTerminalCommand,
) (CognitionTerminalSeal, error) {
	seal, err := loadCognitionTerminalSealTx(ctx, tx, command.EpisodeID)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	_, completionSHA, err := cognitionJSON(command.Completion)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	if seal.Outcome != command.Outcome || seal.FinalRevision != command.ExpectedRevision ||
		seal.CompletionSHA256 != completionSHA || seal.ObligationGraphSHA256 != command.ObligationGraph.SHA256 ||
		seal.AuthorityKind != cognitionTerminalAuthorityWorker ||
		seal.SealedBy.JobID != command.Authority.JobID || seal.SealedBy.Generation != command.Authority.Generation ||
		seal.SealedBy.StepID != command.Authority.StepID || episode.TerminalOutcome != command.PublicOutcome {
		return CognitionTerminalSeal{}, fmt.Errorf("%w: cognition terminal replay changed content or authority", ErrCognitionConflict)
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, command.EpisodeID, false)
	if err != nil || !found || graph.Version != command.GraphVersion ||
		graph.Graph.SHA256 != command.ObligationGraph.SHA256 {
		return CognitionTerminalSeal{}, fmt.Errorf("%w: cognition terminal replay graph changed: %v", ErrCognitionConflict, err)
	}
	var persistedCompletion []byte
	if err := tx.QueryRow(ctx, `SELECT completion_json FROM cognition_terminal_seals WHERE episode_id=$1`, command.EpisodeID).
		Scan(&persistedCompletion); err != nil {
		return CognitionTerminalSeal{}, err
	}
	want, err := json.Marshal(command.Completion)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	if string(persistedCompletion) != string(want) {
		return CognitionTerminalSeal{}, fmt.Errorf("%w: cognition terminal completion projection changed", ErrCognitionConflict)
	}
	return seal, nil
}
