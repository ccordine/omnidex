package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func loadCognitionPlanRevisionTracePayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	record cognitionTraceRecord,
) ([]byte, error) {
	var raw []byte
	var persistedSHA, actionID string
	var outputGraphVersion, sourceCallOrdinal int64
	if err := tx.QueryRow(ctx, `
		SELECT revisions.descriptor_json,revisions.descriptor_json_sha256,
		       applications.action_id,applications.output_graph_version,snapshots.call_ordinal
		FROM cognition_plan_revisions revisions
		JOIN cognition_plan_revision_applications applications
		  ON applications.plan_revision_id=revisions.plan_revision_id
		JOIN cognition_runtime_snapshots snapshots
		  ON snapshots.snapshot_sha256=revisions.source_snapshot_sha256
		WHERE revisions.episode_id=$1 AND revisions.plan_revision_id=$2
	`, episode.EpisodeID, record.ID).Scan(
		&raw, &persistedSHA, &actionID, &outputGraphVersion, &sourceCallOrdinal,
	); err != nil {
		return nil, fmt.Errorf("load sealed cognition plan revision %q: %w", record.ID, err)
	}
	if outputGraphVersion != record.Sequence || sourceCallOrdinal != record.CallOrdinal ||
		persistedSHA != record.SHA256 || cognitionPayloadSHA(raw) != record.SHA256 {
		return nil, fmt.Errorf("%w: sealed cognition plan revision authority changed", ErrCognitionConflict)
	}
	value, err := validateCognitionPlanRevisionTracePayload(raw, record.SHA256)
	if err != nil || value.EpisodeID != episode.EpisodeID || value.ID != record.ID {
		return nil, fmt.Errorf("%w: sealed cognition plan revision payload is invalid", ErrCognitionConflict)
	}
	action, found, err := loadCognitionActionTx(ctx, tx, cognition.ActionID(actionID), false)
	if err != nil || !found {
		return nil, fmt.Errorf("%w: sealed cognition plan revision action is unavailable: %v", ErrCognitionConflict, err)
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(action.Decision)
	if err != nil || action.EpisodeID != episode.EpisodeID ||
		action.SnapshotSHA256 != value.SourceSnapshotSHA256 ||
		decisionSHA != value.SourceDecisionSHA256 {
		return nil, fmt.Errorf("%w: sealed cognition plan revision source changed", ErrCognitionConflict)
	}
	graph, found, err := loadCognitionObligationGraphVersionTx(
		ctx, tx, episode.EpisodeID, uint64(outputGraphVersion),
	)
	if err != nil || !found || graph.CommandID != value.ID ||
		graph.CommandKind != CognitionObligationPlanRevise ||
		graph.Graph.SHA256 != value.ResultGraphSHA256 {
		return nil, fmt.Errorf("%w: sealed cognition plan revision result graph changed: %v", ErrCognitionConflict, err)
	}
	return append([]byte(nil), raw...), nil
}

func validateCognitionPlanRevisionTracePayload(
	raw []byte,
	sha256 string,
) (cognition.PlanRevisionMaterialization, error) {
	var value cognition.PlanRevisionMaterialization
	if !json.Valid(raw) || cognitionPayloadSHA(raw) != sha256 ||
		json.Unmarshal(raw, &value) != nil || value.Validate() != nil {
		return value, fmt.Errorf("%w: sealed cognition plan revision payload is invalid", ErrCognitionConflict)
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, raw) {
		return value, fmt.Errorf("%w: sealed cognition plan revision is not canonical", ErrCognitionConflict)
	}
	return value, nil
}
