package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CancelCognitionEpisode(
	ctx context.Context,
	command cognitionruntime.CancellationCommand,
) (cognitionruntime.CancellationSeal, error) {
	normalized, err := newCognitionCancellationCommand(command)
	if err != nil {
		return cognitionruntime.CancellationSeal{}, err
	}
	if r == nil || r.pool == nil || ctx == nil {
		return cognitionruntime.CancellationSeal{}, fmt.Errorf("cognition cancellation requires PostgreSQL and context")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognitionruntime.CancellationSeal{}, err
	}
	defer tx.Rollback(ctx)
	terminal, active, err := prepareCognitionCancellationTx(ctx, tx, normalized)
	if err != nil {
		return cognitionruntime.CancellationSeal{}, err
	}
	if active {
		if err := insertCognitionCancellationTx(ctx, tx, normalized); err != nil {
			return cognitionruntime.CancellationSeal{}, err
		}
	} else if err := requireCognitionCancellationReplayTx(ctx, tx, normalized); err != nil {
		return cognitionruntime.CancellationSeal{}, err
	}
	seal, err := sealCognitionEpisodeTx(ctx, tx, terminal)
	if err != nil {
		return cognitionruntime.CancellationSeal{}, err
	}
	result := cognitionruntime.CancellationSeal{
		Episode: command.Binding.Episode, Code: command.Code,
		SourceEvidenceID: command.SourceEvidence.ID, TraceSHA256: seal.TraceSHA256,
	}
	if err := result.ValidateFor(command); err != nil {
		return cognitionruntime.CancellationSeal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return cognitionruntime.CancellationSeal{}, err
	}
	return result, nil
}

func prepareCognitionCancellationTx(
	ctx context.Context,
	tx pgx.Tx,
	command cognitionCancellationCommand,
) (CognitionTerminalCommand, bool, error) {
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, command.QueueAuthority); err != nil {
		return CognitionTerminalCommand{}, false, err
	} else if status != model.StepStatusRunning {
		return CognitionTerminalCommand{}, false, staleStepAttemptError(
			command.QueueAuthority, "cognition cancellation actor is not running", nil,
		)
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, command.Binding.Episode.ID, true)
	if err != nil {
		return CognitionTerminalCommand{}, false, err
	}
	if !found {
		return CognitionTerminalCommand{}, false, fmt.Errorf(
			"%w: %s", ErrCognitionEpisodeNotFound, command.Binding.Episode.ID,
		)
	}
	if err := cognitionAuthorityMatches(command.QueueAuthority, episode); err != nil {
		return CognitionTerminalCommand{}, false, err
	}
	if episode.CurrentRevision != command.Expected.ExpectedRevision {
		return CognitionTerminalCommand{}, false, fmt.Errorf(
			"%w: cognition cancellation observed a stale world revision", ErrCognitionConflict,
		)
	}
	active := episode.Status == CognitionEpisodeActive
	if !active && episode.Status != CognitionEpisodeCanceled {
		return CognitionTerminalCommand{}, false, fmt.Errorf(
			"%w: cognition episode is terminal with outcome %s", ErrCognitionConflict, episode.Status,
		)
	}
	if err := requireNoUnresolvedCognitionActionTx(ctx, tx, episode.EpisodeID); err != nil {
		return CognitionTerminalCommand{}, false, err
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episode.EpisodeID, true)
	if err != nil || !found {
		return CognitionTerminalCommand{}, false, fmt.Errorf("load cancellation graph: %w", err)
	}
	restored, err := cognition.RestoreObligationGraph(graph.Graph)
	if err != nil {
		return CognitionTerminalCommand{}, false, err
	}
	root, exists := restored.Obligation(graph.Graph.RootID)
	if !exists {
		return CognitionTerminalCommand{}, false, fmt.Errorf("%w: cancellation graph root is missing", ErrCognitionConflict)
	}
	completion, err := cognition.NewCompletionResult(
		root.ID, root.CompletionCheck, episode.CurrentRevision,
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		return CognitionTerminalCommand{}, false, err
	}
	terminal := CognitionTerminalCommand{
		Authority: command.QueueAuthority, EpisodeID: episode.EpisodeID,
		Outcome: CognitionEpisodeCanceled, GraphVersion: graph.Version,
		Completion: completion, ObligationGraph: graph.Graph.Clone(),
		PublicOutcome:    command.Expected.SourceEvidence.PublicMessage,
		ExpectedRevision: episode.CurrentRevision,
	}
	if err := validateCognitionTerminalCommand(terminal); err != nil {
		return CognitionTerminalCommand{}, false, err
	}
	return terminal, active, nil
}

func insertCognitionCancellationTx(
	ctx context.Context,
	tx pgx.Tx,
	command cognitionCancellationCommand,
) error {
	evidenceRaw, evidenceJSONSHA, err := cognitionJSON(command.Expected.SourceEvidence)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_episode_cancellations (
			episode_id,cancellation_code,expected_revision,expected_revision_sha256,
			source_evidence_id,source_evidence_json,source_evidence_sha256,source_evidence_json_sha256,
			job_id,generation,step_id,authority_kind,actor_attempt,actor_worker_id,lifecycle_operation_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULL)
	`, command.Binding.Episode.ID, command.Expected.Code,
		int64(command.Expected.ExpectedRevision.Number), command.Expected.ExpectedRevision.SHA256,
		command.Expected.SourceEvidence.ID, string(evidenceRaw), command.Expected.SourceEvidence.SHA256,
		evidenceJSONSHA, command.QueueAuthority.JobID, command.QueueAuthority.Generation,
		command.QueueAuthority.StepID, cognitionTerminalAuthorityWorker,
		command.QueueAuthority.Attempt, command.QueueAuthority.WorkerID)
	if err != nil {
		return fmt.Errorf("persist cognition cancellation evidence: %w", err)
	}
	return nil
}

func requireCognitionCancellationReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	command cognitionCancellationCommand,
) error {
	var code cognitionruntime.CancellationCode
	var revision int64
	var revisionSHA, evidenceID, evidenceRaw, evidenceSHA, evidenceJSONSHA, authorityKind, worker string
	var jobID, generation, stepID, attempt int64
	err := tx.QueryRow(ctx, `
		SELECT cancellation_code,expected_revision,expected_revision_sha256,
		       source_evidence_id,source_evidence_json,source_evidence_sha256,source_evidence_json_sha256,
		       job_id,generation,step_id,authority_kind,actor_attempt,actor_worker_id
		FROM cognition_episode_cancellations WHERE episode_id=$1
	`, command.Binding.Episode.ID).Scan(
		&code, &revision, &revisionSHA, &evidenceID, &evidenceRaw, &evidenceSHA, &evidenceJSONSHA,
		&jobID, &generation, &stepID, &authorityKind, &attempt, &worker,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: canceled cognition episode lacks cancellation evidence", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	wantRaw, wantJSONSHA, err := cognitionJSON(command.Expected.SourceEvidence)
	if err != nil {
		return err
	}
	if code != command.Expected.Code || uint64(revision) != command.Expected.ExpectedRevision.Number ||
		revisionSHA != command.Expected.ExpectedRevision.SHA256 || evidenceID != command.Expected.SourceEvidence.ID ||
		evidenceRaw != string(wantRaw) || evidenceSHA != command.Expected.SourceEvidence.SHA256 ||
		evidenceJSONSHA != wantJSONSHA || jobID != command.QueueAuthority.JobID ||
		generation != command.QueueAuthority.Generation || stepID != command.QueueAuthority.StepID ||
		authorityKind != cognitionTerminalAuthorityWorker || attempt != command.QueueAuthority.Attempt ||
		worker != command.QueueAuthority.WorkerID {
		return fmt.Errorf("%w: cognition cancellation replay changed exact binding or evidence", ErrCognitionConflict)
	}
	return nil
}
