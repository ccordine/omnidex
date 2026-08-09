package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func insertAcceptedIntentArtifactTx(
	ctx context.Context,
	tx pgx.Tx,
	envelope artifacts.Envelope,
) (int64, string, error) {
	var existingID int64
	err := tx.QueryRow(ctx, `
		SELECT id FROM artifacts
		WHERE job_id=$1 AND kind=$2
		ORDER BY id LIMIT 1
		FOR UPDATE
	`, envelope.JobID, artifacts.KindIntent).Scan(&existingID)
	if err == nil {
		return 0, "", fmt.Errorf(
			"job %d already has unbound intent artifact %d",
			envelope.JobID, existingID,
		)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", fmt.Errorf("inspect existing intent artifact: %w", err)
	}
	var artifactID int64
	var payloadSHA string
	err = tx.QueryRow(ctx, `
		INSERT INTO artifacts (job_id, step_id, kind, version, payload_json)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id, encode(digest(payload_json::text, 'sha256'), 'hex')
	`, envelope.JobID, envelope.StepID, envelope.Kind,
		envelope.Version, string(envelope.Payload)).Scan(&artifactID, &payloadSHA)
	if err != nil {
		return 0, "", fmt.Errorf("insert accepted intent artifact: %w", err)
	}
	return artifactID, payloadSHA, nil
}

func insertAcceptedIntentProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	projection acceptedIntentProjection,
) error {
	result, err := tx.Exec(ctx, `
		INSERT INTO task_artifact_projections (
			artifact_id, job_id, step_id, job_generation, ledger_id,
			artifact_kind, artifact_version, projection_schema, payload_sha256,
			objective_node_id, ledger_start_version, ledger_end_version
		) VALUES ($1, $2, $3, $4, $5, $6, '1', $7, $8, $9, $10, $11)
	`, projection.ArtifactID, projection.JobID, projection.StepID,
		projection.JobGeneration, projection.LedgerID, artifacts.KindIntent,
		acceptedIntentProjectionSchema, projection.PayloadSHA256,
		projection.ObjectiveNodeID, int64(projection.LedgerStart), int64(projection.LedgerEnd))
	if err := requireTaskLedgerRow(result, err, "insert accepted intent projection"); err != nil {
		return err
	}
	for _, item := range projection.Items {
		result, err = tx.Exec(ctx, `
			INSERT INTO task_artifact_projection_items (
				artifact_id, job_id, ledger_id, item_kind, ordinal,
				node_id, entry_id, source_uri, source_version, source_sha256
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, projection.ArtifactID, projection.JobID, projection.LedgerID,
			item.Kind, item.Ordinal, nullableTaskText(string(item.NodeID)),
			nullableTaskText(string(item.EntryID)), item.SourceURI,
			item.SourceVersion, item.SourceSHA256)
		if err := requireTaskLedgerRow(result, err, "insert accepted intent projection item"); err != nil {
			return err
		}
	}
	return nil
}

func loadAcceptedIntentProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	payload []byte,
) (acceptedIntentProjection, bool, error) {
	var projection acceptedIntentProjection
	var ledgerID, objectiveID string
	var startVersion, endVersion int64
	var artifactKind, artifactVersion, projectionSchema string
	var samePayload bool
	err := tx.QueryRow(ctx, `
		SELECT projection.artifact_id, projection.job_id, projection.step_id,
		       projection.job_generation, projection.ledger_id,
		       projection.artifact_kind, projection.artifact_version,
		       projection.projection_schema, projection.payload_sha256,
		       projection.objective_node_id, projection.ledger_start_version,
		       projection.ledger_end_version,
		       artifact.payload_json = $2::jsonb
		FROM task_artifact_projections AS projection
		JOIN artifacts AS artifact ON artifact.id=projection.artifact_id
		WHERE projection.job_id=$1 AND projection.artifact_kind='intent'
		FOR UPDATE OF projection
	`, jobID, string(payload)).Scan(
		&projection.ArtifactID, &projection.JobID, &projection.StepID,
		&projection.JobGeneration, &ledgerID, &artifactKind, &artifactVersion,
		&projectionSchema, &projection.PayloadSHA256, &objectiveID,
		&startVersion, &endVersion, &samePayload,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return acceptedIntentProjection{}, false, nil
	}
	if err != nil {
		return acceptedIntentProjection{}, false, fmt.Errorf("load accepted intent projection: %w", err)
	}
	if !samePayload || artifactKind != artifacts.KindIntent || artifactVersion != "1" ||
		projectionSchema != acceptedIntentProjectionSchema || startVersion <= 0 || endVersion <= startVersion {
		return acceptedIntentProjection{}, false, fmt.Errorf(
			"%w: accepted intent projection for job %d disagrees with its artifact",
			taskstate.ErrInvalidState, jobID,
		)
	}
	projection.LedgerID = taskstate.LedgerID(ledgerID)
	projection.ObjectiveNodeID = taskstate.NodeID(objectiveID)
	projection.LedgerStart = uint64(startVersion)
	projection.LedgerEnd = uint64(endVersion)
	items, err := loadAcceptedIntentProjectionItemsTx(ctx, tx, projection)
	if err != nil {
		return acceptedIntentProjection{}, false, err
	}
	projection.Items = items
	return projection, true, nil
}

func loadAcceptedIntentProjectionItemsTx(
	ctx context.Context,
	tx pgx.Tx,
	projection acceptedIntentProjection,
) ([]acceptedIntentProjectionItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT item_kind, ordinal, COALESCE(node_id, ''), COALESCE(entry_id, ''),
		       source_uri, source_version, source_sha256
		FROM task_artifact_projection_items
		WHERE artifact_id=$1 AND job_id=$2 AND ledger_id=$3
		ORDER BY item_kind, ordinal
	`, projection.ArtifactID, projection.JobID, projection.LedgerID)
	if err != nil {
		return nil, fmt.Errorf("load accepted intent projection items: %w", err)
	}
	defer rows.Close()
	items := make([]acceptedIntentProjectionItem, 0)
	for rows.Next() {
		var item acceptedIntentProjectionItem
		if err := rows.Scan(
			&item.Kind, &item.Ordinal, &item.NodeID, &item.EntryID,
			&item.SourceURI, &item.SourceVersion, &item.SourceSHA256,
		); err != nil {
			return nil, fmt.Errorf("scan accepted intent projection item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accepted intent projection items: %w", err)
	}
	return items, nil
}
