package queue

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/jackc/pgx/v5"
)

const channelSessionRevisionSchema = "omnidex.channel-session-revision.v2"

type ChannelSessionJobState struct {
	ID         int64     `json:"id"`
	Status     string    `json:"status"`
	Generation int64     `json:"generation"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ChannelSessionState struct {
	ChannelID                model.ChannelID         `json:"channel_id"`
	WorkspaceRoot            string                  `json:"workspace_root"`
	WorkspaceIdentity        string                  `json:"workspace_identity"`
	Revision                 string                  `json:"revision"`
	LatestMessageID          *int64                  `json:"latest_message_id,omitempty"`
	LatestTurnOperationID    *LifecycleOperationID   `json:"latest_turn_operation_id,omitempty"`
	LatestControlOperationID *LifecycleOperationID   `json:"latest_control_operation_id,omitempty"`
	LatestJob                *ChannelSessionJobState `json:"latest_job,omitempty"`
}

// ChannelSessionState returns only identities which prove whether a caller's
// persisted session snapshot is stale. It never returns transcript or steps.
func (r *Repository) ChannelSessionState(
	ctx context.Context,
	channelID model.ChannelID,
	workspaceIdentity string,
) (ChannelSessionState, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return ChannelSessionState{}, fmt.Errorf("channel session state requires PostgreSQL and context")
	}
	if err := channelID.Validate(); err != nil {
		return ChannelSessionState{}, err
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return ChannelSessionState{}, fmt.Errorf("channel session workspace identity: %w", err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ChannelSessionState{}, err
	}
	defer tx.Rollback(ctx)
	state, err := channelSessionStateTx(ctx, tx, channelID, workspaceIdentity)
	if err != nil {
		return ChannelSessionState{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChannelSessionState{}, err
	}
	return state, nil
}

func channelSessionStateTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID model.ChannelID,
	workspaceIdentity string,
) (ChannelSessionState, error) {
	if ctx == nil || tx == nil {
		return ChannelSessionState{}, fmt.Errorf("channel session state transaction requires PostgreSQL and context")
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return ChannelSessionState{}, fmt.Errorf("channel session workspace identity: %w", err)
	}
	var state ChannelSessionState
	state.WorkspaceIdentity = workspaceIdentity
	var scope model.ChannelScope
	var mode model.ChannelMode
	var channelUpdatedAt time.Time
	var latestTurnID, latestControlID *string
	var latestJobID, latestJobGeneration *int64
	var latestJobStatus *string
	var latestJobUpdatedAt *time.Time
	var activeJobID *int64
	var activeJobMetadata []byte
	var duplicateActiveJob bool
	err := tx.QueryRow(ctx, `
		SELECT channel.id,channel.scope,channel.mode,channel.workspace_root,
		       channel.updated_at,message.id,turn.operation_id,
		       control.operation_id,job.id,job.status,
		       job.current_generation,job.updated_at,active_job.id,
		       COALESCE(active_job.metadata,'null'::jsonb),
		       EXISTS (
		         SELECT 1 FROM jobs AS competing_job
		         WHERE competing_job.pipeline='chat'
		           AND competing_job.metadata->>'channel_id'=channel.id
		           AND competing_job.status IN ('pending','running','waiting_input')
		           AND competing_job.id<>active_job.id
		       )
		FROM ai_channels AS channel
		LEFT JOIN LATERAL (
		  SELECT id FROM ai_channel_messages
		  WHERE channel_id=channel.id ORDER BY id DESC LIMIT 1
		) AS message ON TRUE
		LEFT JOIN LATERAL (
		  SELECT operation_id
		  FROM (
		    SELECT operation_id,created_at
		    FROM channel_session_turn_operations
		    WHERE channel_id=channel.id
		    UNION ALL
		    SELECT operation.operation_id,operation.created_at
		    FROM job_lifecycle_operations AS operation
		    JOIN jobs AS bound_job ON bound_job.id=operation.job_id
		    WHERE bound_job.pipeline='chat'
		      AND bound_job.metadata->>'channel_id'=channel.id
		      AND operation.kind='submit_feedback'
		  ) AS turn_history
		  ORDER BY created_at DESC,operation_id DESC LIMIT 1
		) AS turn ON TRUE
		LEFT JOIN LATERAL (
		  SELECT operation.operation_id
		  FROM job_lifecycle_operations AS operation
		  JOIN jobs AS bound_job ON bound_job.id=operation.job_id
		  WHERE bound_job.pipeline='chat'
		    AND bound_job.metadata->>'channel_id'=channel.id
		    AND operation.kind IN ('interrupt_job','replan_job','cancel_job')
		  ORDER BY operation.created_at DESC,operation.operation_id DESC LIMIT 1
		) AS control ON TRUE
		LEFT JOIN LATERAL (
		  SELECT id,status,current_generation,updated_at
		  FROM jobs
		  WHERE pipeline='chat' AND metadata->>'channel_id'=channel.id
		  ORDER BY id DESC LIMIT 1
		) AS job ON TRUE
		LEFT JOIN LATERAL (
		  SELECT id,metadata
		  FROM jobs
		  WHERE pipeline='chat' AND metadata->>'channel_id'=channel.id
		    AND status IN ('pending','running','waiting_input')
		  ORDER BY id DESC LIMIT 1
		) AS active_job ON TRUE
		WHERE channel.id=$1
	`, channelID).Scan(
		&state.ChannelID,
		&scope,
		&mode,
		&state.WorkspaceRoot,
		&channelUpdatedAt,
		&state.LatestMessageID,
		&latestTurnID,
		&latestControlID,
		&latestJobID,
		&latestJobStatus,
		&latestJobGeneration,
		&latestJobUpdatedAt,
		&activeJobID,
		&activeJobMetadata,
		&duplicateActiveJob,
	)
	if err != nil {
		return ChannelSessionState{}, err
	}
	if scope != model.ChannelScopeUser || mode != model.ChannelModeAssistant {
		return ChannelSessionState{}, fmt.Errorf("channel %q is not an assistant user session", channelID)
	}
	if err := model.ValidateChannelWorkspaceRoot(state.WorkspaceRoot); err != nil {
		return ChannelSessionState{}, fmt.Errorf("channel %q workspace root: %w", channelID, err)
	}
	if err := requireCLIChatSessionWorkspaceBinding(
		state.ChannelID,
		state.WorkspaceRoot,
		workspaceIdentity,
	); err != nil {
		return ChannelSessionState{}, err
	}
	if channelUpdatedAt.IsZero() {
		return ChannelSessionState{}, fmt.Errorf("channel %q has no update authority", channelID)
	}
	if state.LatestMessageID != nil && *state.LatestMessageID < 1 {
		return ChannelSessionState{}, fmt.Errorf("channel %q has an invalid latest message identity", channelID)
	}
	if duplicateActiveJob {
		return ChannelSessionState{}, fmt.Errorf("channel %q has multiple active jobs", channelID)
	}
	if activeJobID != nil {
		if *activeJobID < 1 {
			return ChannelSessionState{}, fmt.Errorf("channel %q has an invalid active job identity", channelID)
		}
		if err := requireLifecycleWorkspaceAuthority(
			model.Job{ID: *activeJobID, Pipeline: model.PipelineChat, Metadata: activeJobMetadata},
			state.WorkspaceRoot,
			workspaceIdentity,
		); err != nil {
			return ChannelSessionState{}, fmt.Errorf("channel %q active job workspace: %w", channelID, err)
		}
	}
	if latestTurnID != nil {
		operationID, err := ParseLifecycleOperationID(*latestTurnID)
		if err != nil {
			return ChannelSessionState{}, err
		}
		state.LatestTurnOperationID = &operationID
	}
	if latestControlID != nil {
		operationID, err := ParseLifecycleOperationID(*latestControlID)
		if err != nil {
			return ChannelSessionState{}, err
		}
		state.LatestControlOperationID = &operationID
	}
	if latestJobID != nil || latestJobStatus != nil || latestJobGeneration != nil || latestJobUpdatedAt != nil {
		if latestJobID == nil || latestJobStatus == nil || latestJobGeneration == nil || latestJobUpdatedAt == nil ||
			*latestJobID < 1 || *latestJobGeneration < 1 || latestJobUpdatedAt.IsZero() {
			return ChannelSessionState{}, fmt.Errorf("channel %q has incomplete latest job authority", channelID)
		}
		switch *latestJobStatus {
		case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
			model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		default:
			return ChannelSessionState{}, fmt.Errorf("channel %q latest job has unregistered status %q", channelID, *latestJobStatus)
		}
		state.LatestJob = &ChannelSessionJobState{
			ID: *latestJobID, Status: *latestJobStatus,
			Generation: *latestJobGeneration, UpdatedAt: *latestJobUpdatedAt,
		}
	}
	state.Revision = channelSessionRevision(state, channelUpdatedAt)
	return state, nil
}

func channelSessionRevision(state ChannelSessionState, channelUpdatedAt time.Time) string {
	messageID := "none"
	if state.LatestMessageID != nil {
		messageID = strconv.FormatInt(*state.LatestMessageID, 10)
	}
	job := []string{"none", "none", "none", "none"}
	if state.LatestJob != nil {
		job = []string{
			strconv.FormatInt(state.LatestJob.ID, 10),
			state.LatestJob.Status,
			strconv.FormatInt(state.LatestJob.Generation, 10),
			state.LatestJob.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}
	}
	return "channel_session_revision_" + lifecycleIdentityDigest(
		channelSessionRevisionSchema,
		string(state.ChannelID),
		state.WorkspaceRoot,
		state.WorkspaceIdentity,
		channelUpdatedAt.UTC().Format(time.RFC3339Nano),
		messageID,
		lifecycleOperationIDOrNone(state.LatestTurnOperationID),
		lifecycleOperationIDOrNone(state.LatestControlOperationID),
		job[0], job[1], job[2], job[3],
	)
}

func lifecycleOperationIDOrNone(id *LifecycleOperationID) string {
	if id == nil {
		return "none"
	}
	return string(*id)
}
