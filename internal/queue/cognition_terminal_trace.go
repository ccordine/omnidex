package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

const cognitionTraceAuthoritySchemaV2 = "omnidex.cognition-trace-authority.v2"

type cognitionTraceAuthority struct {
	Schema         string                  `json:"schema"`
	EpisodeID      cognition.EpisodeID     `json:"episode_id"`
	Revision       cognition.WorldRevision `json:"revision"`
	GraphVersion   uint64                  `json:"graph_version"`
	GraphSHA256    string                  `json:"graph_sha256"`
	LedgerVersion  uint64                  `json:"ledger_version"`
	WorkingVersion uint64                  `json:"working_set_version"`
	Records        []cognitionTraceRecord  `json:"records"`
}

type cognitionTraceRecord struct {
	Kind        string `json:"kind"`
	CallOrdinal int64  `json:"call_ordinal"`
	Phase       int    `json:"phase"`
	Sequence    int64  `json:"sequence"`
	ID          string `json:"id"`
	SHA256      string `json:"sha256"`
}

func buildCognitionTraceAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	ledgerVersion, workingVersion uint64,
) ([]byte, string, error) {
	if err := requireCognitionTraceSchemaAuthorityTx(ctx, tx); err != nil {
		return nil, "", err
	}
	rows, err := tx.Query(ctx, `
		SELECT kind,call_ordinal,phase,sequence,id,sha256 FROM (
			SELECT 'provider_process_observation'::text AS kind,sequence,
			       observation_id AS id,receipt_sha256 AS sha256,0 AS call_ordinal,5 AS phase
			FROM cognition_provider_process_observations WHERE episode_id=$1
			UNION ALL
			SELECT 'provider_activation_failure',record_number,record_id,receipt_sha256,0,4
			FROM cognition_provider_activation_failures WHERE episode_id=$1
			UNION ALL
			SELECT 'transition'::text AS kind,revision AS sequence,transition_id AS id,
			       transition_sha256 AS sha256,COALESCE(snapshots.call_ordinal,0) AS call_ordinal,
			       CASE WHEN transitions.action_id IS NULL THEN 10 ELSE 53 END AS phase
			FROM cognition_transitions transitions
			LEFT JOIN cognition_actions actions ON actions.action_id=transitions.action_id
			LEFT JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=actions.snapshot_sha256
			WHERE transitions.episode_id=$1
			UNION ALL
			SELECT 'context_projection',snapshots.call_ordinal,projections.projection_id,
			       substr(projections.projection_id,length('context_projection_')+1),
			       snapshots.call_ordinal,10
			FROM cognition_runtime_snapshots snapshots
			JOIN context_projections projections ON projections.projection_id=snapshots.projection_id
			WHERE snapshots.episode_id=$1
			UNION ALL
			SELECT 'runtime_snapshot',call_ordinal,preparation_id,snapshot_sha256,call_ordinal,20
			FROM cognition_runtime_snapshots WHERE episode_id=$1
			UNION ALL
			SELECT 'policy_attempt',0,calls.call_id||':attempt',calls.attempt_sha256,
			       snapshots.call_ordinal,30
			FROM cognition_policy_calls calls
			JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
			WHERE calls.episode_id=$1
			UNION ALL
			SELECT 'policy_result',0,calls.call_id||':result',calls.result_sha256,
			       snapshots.call_ordinal,31
			FROM cognition_policy_calls calls
			JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
			WHERE calls.episode_id=$1 AND calls.result_sha256 IS NOT NULL
			UNION ALL
			SELECT 'policy_abandonment',abandonments.recovery_attempt,
			       abandonments.abandonment_id,abandonments.descriptor_json_sha256,
			       abandonments.call_ordinal,33
			FROM cognition_policy_call_abandonments abandonments
			WHERE abandonments.episode_id=$1
			UNION ALL
			SELECT 'reconciliation_command',0,reconciliations.reconciliation_id,
			       reconciliations.command_sha256,snapshots.call_ordinal,40
			FROM cognition_reconciliations reconciliations
			JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=reconciliations.snapshot_sha256
			WHERE reconciliations.episode_id=$1
			UNION ALL
			SELECT 'reconciliation_receipt',0,reconciliations.reconciliation_id,
			       reconciliations.receipt_sha256,snapshots.call_ordinal,41
			FROM cognition_reconciliations reconciliations
			JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=reconciliations.snapshot_sha256
			WHERE reconciliations.episode_id=$1
			UNION ALL
			SELECT 'proposal_materialization',materializations.proposal_index,
			       materializations.materialization_id,materializations.payload_json_sha256,
			       materializations.call_ordinal,42 AS phase
			FROM cognition_proposal_materializations materializations
			WHERE materializations.episode_id=$1
			UNION ALL
			SELECT 'accepted_fact_materialization',materializations.transition_revision,
			       materializations.materialization_id,materializations.payload_json_sha256,
			       materializations.call_ordinal,
			       CASE WHEN materializations.action_id IS NULL THEN 11 ELSE 54 END
			FROM cognition_accepted_fact_materializations materializations
			WHERE materializations.episode_id=$1
			UNION ALL
			SELECT 'belief_revision',revisions.expected_ledger_version,revisions.revision_id,
			       revisions.descriptor_json_sha256,snapshots.call_ordinal,43
			FROM cognition_belief_revisions revisions
			JOIN cognition_runtime_snapshots snapshots
			  ON snapshots.snapshot_sha256=revisions.source_snapshot_sha256
			WHERE revisions.episode_id=$1
			UNION ALL
			SELECT 'plan_revision',applications.output_graph_version,revisions.plan_revision_id,
			       revisions.descriptor_json_sha256,snapshots.call_ordinal,44
			FROM cognition_plan_revisions revisions
			JOIN cognition_plan_revision_applications applications
			  ON applications.plan_revision_id=revisions.plan_revision_id
			JOIN cognition_runtime_snapshots snapshots
			  ON snapshots.snapshot_sha256=revisions.source_snapshot_sha256
			WHERE revisions.episode_id=$1
			UNION ALL
			SELECT 'episode_progress_command',progress.output_graph_version,progress.command_id,
			       progress.command_sha256,snapshots.call_ordinal,70
			FROM cognition_episode_progress progress
			JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=progress.source_snapshot_sha256
			WHERE progress.episode_id=$1
			UNION ALL
			SELECT 'episode_progress',progress.output_graph_version,progress.command_id,
			       progress.progress_sha256,snapshots.call_ordinal,71
			FROM cognition_episode_progress progress
			JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=progress.source_snapshot_sha256
			WHERE progress.episode_id=$1
			UNION ALL
			SELECT 'action_event',events.sequence,events.action_id||':'||events.status,events.event_sha256,
			       snapshots.call_ordinal,
			       CASE events.sequence WHEN 1 THEN 51 WHEN 2 THEN 52 ELSE 55 END
			FROM cognition_action_events events
			JOIN cognition_actions actions ON actions.action_id=events.action_id
			JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=actions.snapshot_sha256
			WHERE actions.episode_id=$1
			UNION ALL
			SELECT 'lifecycle_retirement',0,retirement_id,descriptor_json_sha256,0,79
			FROM cognition_lifecycle_retirements WHERE episode_id=$1
			UNION ALL
			SELECT 'cancellation_evidence',0,source_evidence_id,source_evidence_json_sha256,0,80
			FROM cognition_episode_cancellations WHERE episode_id=$1
			UNION ALL
			SELECT 'obligation_graph',graphs.graph_version,graphs.command_id,graphs.graph_json_sha256,
			       COALESCE(snapshots.call_ordinal,0),CASE WHEN graphs.graph_version=1 THEN 20 ELSE 72 END
			FROM cognition_obligation_graphs graphs
			LEFT JOIN cognition_episode_progress progress
			  ON progress.episode_id=graphs.episode_id AND progress.output_graph_version=graphs.graph_version
			LEFT JOIN cognition_runtime_snapshots snapshots
			  ON snapshots.snapshot_sha256=progress.source_snapshot_sha256
			WHERE graphs.episode_id=$1
		) records
	`, episode.EpisodeID)
	if err != nil {
		return nil, "", fmt.Errorf("read cognition trace authority: %w", err)
	}
	defer rows.Close()
	records := make([]cognitionTraceRecord, 0, 32)
	for rows.Next() {
		var record cognitionTraceRecord
		if err := rows.Scan(
			&record.Kind, &record.CallOrdinal, &record.Phase,
			&record.Sequence, &record.ID, &record.SHA256,
		); err != nil {
			return nil, "", err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	rows.Close()
	for index := range records {
		if records[index].Kind != "context_projection" {
			continue
		}
		projection, err := loadContextProjectionTx(ctx, tx, records[index].ID)
		if err != nil {
			return nil, "", err
		}
		raw, err := json.Marshal(projection.Projection)
		if err != nil {
			return nil, "", err
		}
		records[index].SHA256 = cognitionPayloadSHA(raw)
	}
	records, err = appendCognitionActionTraceRecordsTx(ctx, tx, episode.EpisodeID, records)
	if err != nil {
		return nil, "", err
	}
	records, err = appendCognitionBrainBootstrapTraceRecordsTx(
		ctx, tx, episode.EpisodeID, records,
	)
	if err != nil {
		return nil, "", err
	}
	records, err = appendCognitionWorkingSetTraceRecordsTx(
		ctx, tx, episode, workingVersion, records,
	)
	if err != nil {
		return nil, "", err
	}
	records, err = appendCognitionDiagnosticTraceRecordsTx(
		ctx, tx, episode.EpisodeID, records,
	)
	if err != nil {
		return nil, "", err
	}
	records, err = appendCognitionPolicyEvidenceTraceRecordsTx(
		ctx, tx, episode.EpisodeID, records,
	)
	if err != nil {
		return nil, "", err
	}
	if len(records) < 2 || len(records) > MaxCognitionTraceRecords {
		return nil, "", fmt.Errorf("cognition trace is missing required transition, graph, or action authority")
	}
	sort.Slice(records, func(left, right int) bool {
		return cognitionTraceRecordLess(records[left], records[right])
	})
	trace := cognitionTraceAuthority{
		Schema: cognitionTraceAuthoritySchemaV2, EpisodeID: episode.EpisodeID,
		Revision: episode.CurrentRevision, GraphVersion: graph.Version, GraphSHA256: graph.Graph.SHA256,
		LedgerVersion: ledgerVersion, WorkingVersion: workingVersion, Records: records,
	}
	return cognitionJSON(trace)
}
