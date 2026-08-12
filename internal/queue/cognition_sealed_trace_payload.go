package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func loadCognitionTracePayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	workingVersion uint64,
	record cognitionTraceRecord,
) ([]byte, error) {
	switch record.Kind {
	case "action":
		return loadCognitionTraceActionPayloadTx(ctx, tx, episode, record)
	case CognitionTraceKindAcceptedFactMaterialization:
		return loadCognitionAcceptedFactMaterializationTracePayloadTx(ctx, tx, episode, record)
	case CognitionTraceKindProposalMaterialization:
		return loadCognitionProposalMaterializationTracePayloadTx(ctx, tx, episode, record)
	case "plan_revision":
		return loadCognitionPlanRevisionTracePayloadTx(ctx, tx, episode, record)
	case "context_projection":
		return loadCognitionTraceProjectionTx(ctx, tx, episode, record)
	case "runtime_snapshot":
		return loadCognitionTraceSnapshotTx(ctx, tx, episode, record)
	case "working_set_event", "working_set_snapshot":
		return loadCognitionTraceWorkingSetPayloadTx(
			ctx, tx, episode, workingVersion, record,
		)
	case "policy_timing", "accepted_decision_recovery":
		return loadCognitionTraceDiagnosticPayloadTx(ctx, tx, episode.EpisodeID, record)
	case "policy_response_evidence", "policy_provider_generation_evidence",
		"policy_provider_response_capture":
		return loadCognitionPolicyEvidenceTracePayloadTx(ctx, tx, episode.EpisodeID, record)
	case CognitionTraceKindProviderBrainBootstrap:
		return loadCognitionBrainBootstrapTracePayloadTx(ctx, tx, episode.EpisodeID, record)
	}
	query, args, err := cognitionTracePayloadQuery(episode.EpisodeID, record)
	if err != nil {
		return nil, err
	}
	var raw []byte
	var persistedSHA string
	if err := tx.QueryRow(ctx, query, args...).Scan(&raw, &persistedSHA); err != nil {
		return nil, fmt.Errorf("load sealed cognition %s %q: %w", record.Kind, record.ID, err)
	}
	if !json.Valid(raw) || persistedSHA != record.SHA256 || cognitionPayloadSHA(raw) != record.SHA256 {
		return nil, fmt.Errorf("%w: sealed cognition %s payload changed", ErrCognitionConflict, record.Kind)
	}
	if record.Kind == "belief_revision" {
		if err := validateCognitionBeliefRevisionTracePayload(raw, record.SHA256); err != nil {
			return nil, err
		}
	}
	return append([]byte(nil), raw...), nil
}

func cognitionTracePayloadQuery(
	episodeID cognition.EpisodeID,
	record cognitionTraceRecord,
) (string, []any, error) {
	args := []any{episodeID, record.ID, record.Sequence}
	switch record.Kind {
	case "transition":
		return `SELECT transition_json,transition_sha256 FROM cognition_transitions
			WHERE episode_id=$1 AND transition_id=$2 AND revision=$3`, args, nil
	case "policy_attempt":
		return `SELECT attempt_json,attempt_sha256 FROM cognition_policy_calls
			WHERE episode_id=$1 AND call_id||':attempt'=$2`, args[:2], nil
	case "policy_result":
		return `SELECT result_json,result_sha256 FROM cognition_policy_calls
			WHERE episode_id=$1 AND call_id||':result'=$2 AND result_json IS NOT NULL`, args[:2], nil
	case "provider_process_observation":
		return `SELECT receipt_json,receipt_sha256 FROM cognition_provider_process_observations
			WHERE episode_id=$1 AND observation_id=$2 AND sequence=$3`, args, nil
	case "provider_activation_failure":
		return `SELECT receipt_json,receipt_sha256 FROM cognition_provider_activation_failures
			WHERE episode_id=$1 AND record_id=$2 AND record_number=$3`, args, nil
	case "policy_abandonment":
		return `SELECT descriptor_json,descriptor_json_sha256 FROM cognition_policy_call_abandonments
			WHERE episode_id=$1 AND abandonment_id=$2 AND recovery_attempt=$3`, args, nil
	case "cancellation_evidence":
		return `SELECT source_evidence_json,source_evidence_json_sha256
			FROM cognition_episode_cancellations
			WHERE episode_id=$1 AND source_evidence_id=$2`, args[:2], nil
	case "lifecycle_retirement":
		return `SELECT descriptor_json,descriptor_json_sha256
			FROM cognition_lifecycle_retirements
			WHERE episode_id=$1 AND retirement_id=$2`, args[:2], nil
	case "reconciliation_command":
		return `SELECT command_json,command_sha256 FROM cognition_reconciliations
			WHERE episode_id=$1 AND reconciliation_id=$2`, args[:2], nil
	case "reconciliation_receipt":
		return `SELECT receipt_json,receipt_sha256 FROM cognition_reconciliations
			WHERE episode_id=$1 AND reconciliation_id=$2`, args[:2], nil
	case "belief_revision":
		return cognitionBeliefRevisionTraceQuery, args, nil
	case "episode_progress_command":
		return `SELECT command_json,command_sha256 FROM cognition_episode_progress
			WHERE episode_id=$1 AND command_id=$2 AND output_graph_version=$3`, args, nil
	case "episode_progress":
		return `SELECT progress_json,progress_sha256 FROM cognition_episode_progress
			WHERE episode_id=$1 AND command_id=$2 AND output_graph_version=$3`, args, nil
	case "action_event":
		return `SELECT events.event_json,events.event_sha256 FROM cognition_action_events events
			JOIN cognition_actions actions ON actions.action_id=events.action_id
			WHERE actions.episode_id=$1 AND events.action_id||':'||events.status=$2 AND events.sequence=$3`, args, nil
	case "obligation_graph":
		return `SELECT graph_json,graph_json_sha256 FROM cognition_obligation_graphs
			WHERE episode_id=$1 AND command_id=$2 AND graph_version=$3`, args, nil
	default:
		return "", nil, fmt.Errorf("%w: unregistered cognition trace kind %q", ErrCognitionConflict, record.Kind)
	}
}

func loadCognitionTraceProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	record cognitionTraceRecord,
) ([]byte, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM cognition_runtime_snapshots
		 WHERE episode_id=$1 AND projection_id=$2 AND call_ordinal=$3)
	`, episode.EpisodeID, record.ID, record.Sequence).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: sealed cognition projection is not bound to its snapshot", ErrCognitionConflict)
	}
	projection, err := loadContextProjectionTx(ctx, tx, record.ID)
	if err != nil {
		return nil, err
	}
	if projection.Authority.Mode != ContextProjectionModeLive ||
		projection.Authority.JobID != episode.Authority.JobID ||
		projection.Authority.Generation != episode.Authority.Generation {
		return nil, fmt.Errorf("%w: sealed live cognition projection identity changed", ErrCognitionConflict)
	}
	raw, err := json.Marshal(projection.Projection)
	if err != nil {
		return nil, err
	}
	if cognitionPayloadSHA(raw) != record.SHA256 {
		return nil, fmt.Errorf("%w: sealed cognition projection payload changed", ErrCognitionConflict)
	}
	return raw, nil
}

func loadCognitionTraceSnapshotTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	record cognitionTraceRecord,
) ([]byte, error) {
	var (
		revisionNumber, graphVersion, actorAttempt            int64
		revisionSHA, obligationID, projectionID, workingSetID string
		jobID, generation, stepID                             int64
		workerID                                              string
		budgetJSON, evidenceJSON                              []byte
	)
	if err := tx.QueryRow(ctx, `
		SELECT expected_revision,expected_revision_sha256,obligation_node_id,graph_version,
		       projection_id,working_set_id,job_id,generation,step_id,actor_attempt,
		       actor_worker_id,runtime_budget_json,evidence_refs_json
		FROM cognition_runtime_snapshots
		WHERE episode_id=$1 AND preparation_id=$2 AND call_ordinal=$3 AND snapshot_sha256=$4
	`, episode.EpisodeID, record.ID, record.Sequence, record.SHA256).Scan(
		&revisionNumber, &revisionSHA, &obligationID, &graphVersion, &projectionID,
		&workingSetID, &jobID, &generation, &stepID, &actorAttempt, &workerID,
		&budgetJSON, &evidenceJSON,
	); err != nil {
		return nil, fmt.Errorf("load sealed cognition runtime snapshot: %w", err)
	}
	graph, found, err := loadCognitionObligationGraphVersionTx(
		ctx, tx, episode.EpisodeID, uint64(graphVersion),
	)
	if err != nil || !found {
		return nil, fmt.Errorf("%w: runtime snapshot graph is unavailable: %v", ErrCognitionConflict, err)
	}
	current, exists := cognitionTraceObligation(graph.Graph, cognition.ObligationID(obligationID))
	if !exists || current.Status != cognition.ObligationActive {
		return nil, fmt.Errorf("%w: runtime snapshot obligation changed", ErrCognitionConflict)
	}
	projection, err := loadContextProjectionTx(ctx, tx, projectionID)
	if err != nil || string(projection.Projection.WorkingSetID) != workingSetID {
		return nil, fmt.Errorf("%w: runtime snapshot projection changed: %v", ErrCognitionConflict, err)
	}
	var budget cognition.RuntimeBudget
	var evidence []cognition.EvidenceRef
	if err := json.Unmarshal(budgetJSON, &budget); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(evidenceJSON, &evidence); err != nil {
		return nil, err
	}
	revision := cognition.WorldRevision{
		EpisodeID: episode.EpisodeID, Number: uint64(revisionNumber), SHA256: revisionSHA,
	}
	ref := cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projectionID), SHA256: projection.Projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(workingSetID),
		WorkingSetVersion: projection.Projection.WorkingSetVersion,
		RendererVersion:   projection.Projection.RendererVersion,
	}
	attempt := cognition.AttemptRef{
		JobID: jobID, Generation: generation, StepID: stepID,
		Attempt: uint64(actorAttempt), WorkerID: workerID,
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		episode.Goal, revision, current, episode.ActionCatalog, attempt, ref, budget, evidence,
	)
	if err != nil || snapshot.SHA256() != record.SHA256 {
		return nil, fmt.Errorf("%w: runtime snapshot identity changed: %v", ErrCognitionConflict, err)
	}
	return marshalCognitionRuntimeSnapshot(snapshot)
}

func loadCognitionObligationGraphVersionTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	version uint64,
) (CognitionObligationGraphRecord, bool, error) {
	return scanCognitionObligationGraph(tx.QueryRow(ctx, `
		SELECT episode_id,graph_version,command_id,command_sha256,command_kind,graph_json,graph_sha256,
		       job_id,generation,step_id,actor_attempt,actor_worker_id,created_at
		FROM cognition_obligation_graphs WHERE episode_id=$1 AND graph_version=$2
	`, episodeID, int64(version)), episodeID)
}

func marshalCognitionRuntimeSnapshot(snapshot cognition.RuntimeSnapshot) ([]byte, error) {
	return json.Marshal(struct {
		Goal              cognition.GoalExpression       `json:"goal"`
		CurrentRevision   cognition.WorldRevision        `json:"current_revision"`
		CurrentObligation cognition.Obligation           `json:"current_obligation"`
		ActionCatalog     cognition.ActionCatalog        `json:"action_catalog"`
		Attempt           cognition.AttemptRef           `json:"attempt"`
		ContextProjection cognition.ContextProjectionRef `json:"context_projection"`
		Budget            cognition.RuntimeBudget        `json:"budget"`
		EvidenceRefs      []cognition.EvidenceRef        `json:"evidence_refs"`
	}{
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), snapshot.ContextProjection(),
		snapshot.Budget(), snapshot.EvidenceRefs(),
	})
}

func cognitionPayloadSHA(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func cognitionTraceObligation(
	graph cognition.ObligationGraphSnapshot,
	id cognition.ObligationID,
) (cognition.Obligation, bool) {
	for _, obligation := range graph.Obligations {
		if obligation.ID == id {
			return obligation.Clone(), true
		}
	}
	return cognition.Obligation{}, false
}
