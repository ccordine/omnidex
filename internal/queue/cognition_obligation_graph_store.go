package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func initialCognitionObligationGraph(
	command CognitionEpisodeStart,
) (cognition.ObligationGraphSnapshot, cognitionObligationDescriptor, error) {
	generation := cognition.InitialObligationGeneration
	graph, err := cognition.NewObligationGraph(generation, command.Root.ID, []cognition.ObligationSpec{command.Root})
	if err != nil {
		return cognition.ObligationGraphSnapshot{}, cognitionObligationDescriptor{}, err
	}
	if err := graph.RefreshReadiness(generation); err != nil {
		return cognition.ObligationGraphSnapshot{}, cognitionObligationDescriptor{}, err
	}
	if err := graph.Transition(command.Root.ID, generation, cognition.ObligationActive); err != nil {
		return cognition.ObligationGraphSnapshot{}, cognitionObligationDescriptor{}, err
	}
	snapshot := graph.Snapshot()
	raw, _, err := cognitionJSON(struct {
		Schema     string              `json:"schema"`
		JobID      int64               `json:"job_id"`
		Generation int64               `json:"generation"`
		StepID     int64               `json:"step_id"`
		EpisodeID  cognition.EpisodeID `json:"episode_id"`
		Graph      string              `json:"graph_sha256"`
	}{cognitionObligationCommandSchemaV1, command.Authority.JobID, command.Authority.Generation,
		command.Authority.StepID, command.EpisodeID, snapshot.SHA256})
	if err != nil {
		return cognition.ObligationGraphSnapshot{}, cognitionObligationDescriptor{}, err
	}
	digest := sha256.Sum256(raw)
	sha := hex.EncodeToString(digest[:])
	return snapshot, cognitionObligationDescriptor{
		ID: "cognition_graph_command_" + sha, SHA256: sha,
		Kind: CognitionObligationInitial, Raw: raw,
	}, nil
}

func (r *Repository) CognitionObligationGraph(
	ctx context.Context,
	episodeID cognition.EpisodeID,
) (CognitionObligationGraphRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return CognitionObligationGraphRecord{}, fmt.Errorf("cognition obligation graph requires PostgreSQL and context")
	}
	if err := cognitionEpisodeIdentityValid(episodeID); err != nil {
		return CognitionObligationGraphRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CognitionObligationGraphRecord{}, err
	}
	defer tx.Rollback(ctx)
	record, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episodeID, false)
	if err != nil {
		return CognitionObligationGraphRecord{}, err
	}
	if !found {
		return CognitionObligationGraphRecord{}, fmt.Errorf("%w: episode %q has no obligation graph", ErrCognitionEpisodeNotFound, episodeID)
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionObligationGraphRecord{}, err
	}
	return record, nil
}

func loadCurrentCognitionObligationGraphTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	lock bool,
) (CognitionObligationGraphRecord, bool, error) {
	query := `
		SELECT episode_id,graph_version,command_id,command_sha256,command_kind,graph_json,graph_sha256,
		       job_id,generation,step_id,actor_attempt,actor_worker_id,created_at
		FROM cognition_obligation_graphs
		WHERE episode_id=$1 ORDER BY graph_version DESC LIMIT 1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanCognitionObligationGraph(tx.QueryRow(ctx, query, episodeID), episodeID)
}

func loadCognitionObligationGraphByCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	commandID string,
) (CognitionObligationGraphRecord, bool, error) {
	return scanCognitionObligationGraph(tx.QueryRow(ctx, `
		SELECT episode_id,graph_version,command_id,command_sha256,command_kind,graph_json,graph_sha256,
		       job_id,generation,step_id,actor_attempt,actor_worker_id,created_at
		FROM cognition_obligation_graphs WHERE command_id=$1
	`, commandID), "")
}

type cognitionGraphRow interface {
	Scan(...any) error
}

func scanCognitionObligationGraph(
	row cognitionGraphRow,
	episodeID cognition.EpisodeID,
) (CognitionObligationGraphRecord, bool, error) {
	var (
		version, attempt                        int64
		jobID, generation, stepID               int64
		commandID, commandSHA, commandKind, raw string
		persistedGraphSHA                       string
		workerID                                string
		record                                  CognitionObligationGraphRecord
	)
	var persistedEpisodeID cognition.EpisodeID
	if err := row.Scan(
		&persistedEpisodeID, &version, &commandID, &commandSHA, &commandKind, &raw, &persistedGraphSHA,
		&jobID, &generation, &stepID, &attempt, &workerID, &record.CreatedAt,
	); errors.Is(err, pgx.ErrNoRows) {
		return CognitionObligationGraphRecord{}, false, nil
	} else if err != nil {
		return CognitionObligationGraphRecord{}, false, fmt.Errorf("scan cognition obligation graph: %w", err)
	}
	if version <= 0 || uint64(version) > math.MaxInt64 {
		return CognitionObligationGraphRecord{}, false, fmt.Errorf("cognition obligation graph has invalid version %d", version)
	}
	if err := json.Unmarshal([]byte(raw), &record.Graph); err != nil {
		return CognitionObligationGraphRecord{}, false, fmt.Errorf("decode cognition obligation graph: %w", err)
	}
	if err := record.Graph.Validate(); err != nil {
		return CognitionObligationGraphRecord{}, false, err
	}
	if persistedGraphSHA != record.Graph.SHA256 {
		return CognitionObligationGraphRecord{}, false, fmt.Errorf("cognition obligation graph hash projection changed")
	}
	record.EpisodeID = persistedEpisodeID
	if episodeID != "" && persistedEpisodeID != episodeID {
		return CognitionObligationGraphRecord{}, false, fmt.Errorf("cognition obligation graph episode projection changed")
	}
	record.Version = uint64(version)
	record.CommandID, record.CommandSHA256 = commandID, commandSHA
	record.CommandKind = CognitionObligationCommandKind(commandKind)
	record.Actor = model.StepAttemptAuthority{
		JobID: jobID, Generation: generation, StepID: stepID, Attempt: attempt, WorkerID: workerID,
	}
	return record, true, nil
}

func insertCognitionObligationGraphTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	version uint64,
	descriptor cognitionObligationDescriptor,
	graph cognition.ObligationGraphSnapshot,
	authority model.StepAttemptAuthority,
) (CognitionObligationGraphRecord, error) {
	if version == 0 || version > math.MaxInt64 {
		return CognitionObligationGraphRecord{}, fmt.Errorf("cognition obligation graph version exceeds PostgreSQL BIGINT")
	}
	raw, graphJSONSHA, err := cognitionJSON(graph)
	if err != nil {
		return CognitionObligationGraphRecord{}, err
	}
	if err := graph.Validate(); err != nil {
		return CognitionObligationGraphRecord{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_obligation_graphs (
			episode_id,graph_version,job_id,generation,step_id,command_id,command_sha256,
			command_kind,graph_json,graph_sha256,graph_json_sha256,actor_attempt,actor_worker_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, episodeID, int64(version), authority.JobID, authority.Generation, authority.StepID,
		descriptor.ID, descriptor.SHA256, descriptor.Kind, string(raw), graph.SHA256, graphJSONSHA,
		authority.Attempt, authority.WorkerID); err != nil {
		return CognitionObligationGraphRecord{}, fmt.Errorf("append cognition obligation graph: %w", err)
	}
	record, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episodeID, false)
	if err != nil || !found {
		return CognitionObligationGraphRecord{}, fmt.Errorf("reload cognition obligation graph: %w", err)
	}
	return record, nil
}
