package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func insertCognitionLifecycleRetirementTx(
	ctx context.Context,
	tx pgx.Tx,
	retirement cognitionLifecycleRetirement,
) error {
	if err := retirement.Validate(); err != nil {
		return err
	}
	identity := retirement
	identity.ID, identity.SHA256 = "", ""
	identityJSON, _, err := cognitionJSON(identity)
	if err != nil {
		return err
	}
	descriptorJSON, descriptorJSONSHA, err := cognitionJSON(retirement)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_lifecycle_retirements (
		 retirement_id,retirement_sha256,identity_json,descriptor_json,descriptor_json_sha256,
		 operation_id,operation_kind,operation_sha256,episode_id,job_id,job_generation,
		 step_id,cancellation_code,expected_revision,expected_revision_sha256,
		 graph_version,graph_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`, retirement.ID, retirement.SHA256, string(identityJSON), string(descriptorJSON), descriptorJSONSHA,
		retirement.OperationID, retirement.OperationKind, retirement.OperationSHA256,
		retirement.EpisodeID, retirement.JobID, retirement.JobGeneration, retirement.StepID,
		retirement.Code, int64(retirement.ExpectedRevision.Number), retirement.ExpectedRevision.SHA256,
		int64(retirement.GraphVersion), retirement.GraphSHA256)
	if err != nil {
		return fmt.Errorf("persist cognition lifecycle retirement: %w", err)
	}
	return nil
}

func insertLifecycleCognitionCancellationTx(
	ctx context.Context,
	tx pgx.Tx,
	retirement cognitionLifecycleRetirement,
	evidence cognitionruntime.CancellationEvidence,
) error {
	if err := retirement.Validate(); err != nil {
		return err
	}
	if err := evidence.Validate(); err != nil || evidence.Code != retirement.Code ||
		evidence.SourceErrorSHA256 != retirement.OperationSHA256 {
		return fmt.Errorf("%w: lifecycle cancellation evidence changed", ErrCognitionConflict)
	}
	raw, jsonSHA, err := cognitionJSON(evidence)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_episode_cancellations (
		 episode_id,cancellation_code,expected_revision,expected_revision_sha256,
		 source_evidence_id,source_evidence_json,source_evidence_sha256,source_evidence_json_sha256,
		 job_id,generation,step_id,authority_kind,actor_attempt,actor_worker_id,lifecycle_operation_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'lifecycle',NULL,NULL,$12)
	`, retirement.EpisodeID, retirement.Code, int64(retirement.ExpectedRevision.Number),
		retirement.ExpectedRevision.SHA256, evidence.ID, string(raw), evidence.SHA256, jsonSHA,
		retirement.JobID, retirement.JobGeneration, retirement.StepID, retirement.OperationID)
	if err != nil {
		return fmt.Errorf("persist lifecycle cognition cancellation: %w", err)
	}
	return nil
}

func insertCognitionLifecycleSealSetTx(
	ctx context.Context,
	tx pgx.Tx,
	set cognitionLifecycleSealSet,
) error {
	if err := set.Validate(); err != nil {
		return err
	}
	identity := set
	identity.SHA256 = ""
	identityJSON, _, err := cognitionJSON(identity)
	if err != nil {
		return err
	}
	raw, jsonSHA, err := cognitionJSON(set)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_lifecycle_operation_seals (
		 operation_id,operation_kind,operation_sha256,job_id,generation,episode_count,
		 seal_set_json,seal_set_sha256,identity_json,seal_set_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, set.OperationID, set.OperationKind, set.OperationSHA256, set.JobID, set.Generation,
		len(set.Entries), string(raw), set.SHA256, string(identityJSON), jsonSHA); err != nil {
		return fmt.Errorf("persist cognition lifecycle seal set: %w", err)
	}
	for position, entry := range set.Entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_lifecycle_operation_seal_episodes (
			 operation_id,position,episode_id,retirement_id,retirement_sha256,trace_sha256
			) VALUES ($1,$2,$3,$4,$5,$6)
		`, set.OperationID, position, entry.EpisodeID, entry.RetirementID,
			entry.RetirementSHA256, entry.TraceSHA256); err != nil {
			return fmt.Errorf("persist cognition lifecycle seal entry %d: %w", position, err)
		}
	}
	return nil
}

func loadCognitionLifecycleSealSetTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID LifecycleOperationID,
) (cognitionLifecycleSealSet, bool, error) {
	var raw string
	var jsonSHA string
	err := tx.QueryRow(ctx, `
		SELECT seal_set_json,seal_set_json_sha256
		FROM cognition_lifecycle_operation_seals WHERE operation_id=$1 FOR UPDATE
	`, operationID).Scan(&raw, &jsonSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognitionLifecycleSealSet{}, false, nil
	}
	if err != nil {
		return cognitionLifecycleSealSet{}, false, err
	}
	var set cognitionLifecycleSealSet
	if err := json.Unmarshal([]byte(raw), &set); err != nil || set.Validate() != nil {
		return cognitionLifecycleSealSet{}, false, fmt.Errorf("%w: persisted lifecycle seal set is invalid", ErrCognitionConflict)
	}
	wantRaw, wantJSONSHA, err := cognitionJSON(set)
	if err != nil || string(wantRaw) != raw || wantJSONSHA != jsonSHA {
		return cognitionLifecycleSealSet{}, false, fmt.Errorf("%w: lifecycle seal set projection changed", ErrCognitionConflict)
	}
	entries, err := loadCognitionLifecycleSealEntriesTx(ctx, tx, operationID)
	if err != nil || !reflect.DeepEqual(entries, set.Entries) {
		return cognitionLifecycleSealSet{}, false, fmt.Errorf("%w: normalized lifecycle seal set changed", ErrCognitionConflict)
	}
	return set, true, nil
}

func loadCognitionLifecycleSealEntriesTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID LifecycleOperationID,
) ([]cognitionLifecycleSealEntry, error) {
	rows, err := tx.Query(ctx, `
		SELECT episode_id,retirement_id,retirement_sha256,trace_sha256
		FROM cognition_lifecycle_operation_seal_episodes
		WHERE operation_id=$1 ORDER BY position FOR UPDATE
	`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]cognitionLifecycleSealEntry, 0)
	for rows.Next() {
		var entry cognitionLifecycleSealEntry
		if err := rows.Scan(&entry.EpisodeID, &entry.RetirementID,
			&entry.RetirementSHA256, &entry.TraceSHA256); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
