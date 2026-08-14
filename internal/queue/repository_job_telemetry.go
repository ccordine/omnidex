package queue

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func createTelemetryRunForJob(ctx context.Context, tx pgx.Tx, job model.Job, projectID *int64) (string, error) {
	if job.ID <= 0 {
		return "", nil
	}
	metadata := decodeMetadataObject(job.Metadata)
	workspaceID := strings.TrimSpace(firstMetadataString(metadata, "workspace_id", "workspace", "workspace_root", "project_location"))
	if workspaceID == "" {
		workspaceID = projectLocationFromMetadata(job.Metadata)
	}
	projectType := strings.TrimSpace(firstMetadataString(metadata, "project_type", "framework", "stack"))
	taskKind := strings.TrimSpace(firstMetadataString(metadata, "task_kind", "kind"))
	if taskKind == "" {
		taskKind = inferTelemetryTaskKind(job.Pipeline, metadata)
	}
	promptHash := telemetryPromptHash(job.Instruction)
	promptSummary := telemetryPromptSummary(job.Instruction, 240)
	summary := map[string]any{
		"job_id":         job.ID,
		"pipeline":       job.Pipeline,
		"project_id":     projectID,
		"prompt_summary": promptSummary,
	}
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO omni_runs (session_id, workspace_id, task_kind, prompt_hash, prompt_summary, project_type, playbook_id, status, started_at, local_only, model_roles, summary)
		VALUES (NULLIF($1,''), NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), $8, $9, $10, $11, $12)
		RETURNING id::text
	`, firstMetadataString(metadata, "session_id"), workspaceID, taskKind, promptHash, promptSummary, projectType, firstMetadataString(metadata, "playbook_id"), "pending", job.CreatedAt, true, jsonParam(metadataValue(metadata, "model_roles")), jsonParam(summary)).Scan(&id)
	return id, err
}

func completeTelemetryRunForJob(ctx context.Context, tx pgx.Tx, jobID int64, status string, summary any, completionEvidence any) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	_, err := tx.Exec(ctx, `
		UPDATE omni_runs
		SET status = $2,
		    finished_at = NOW(),
		    duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint),
		    summary = $3,
		    completion_evidence = $4,
		    updated_at = NOW()
		WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid
	`, jobID, status, jsonParam(summary), jsonParam(completionEvidence))
	return err
}

func recordTelemetryJobEvent(ctx context.Context, tx pgx.Tx, jobID int64, eventType string, payload any) error {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO omni_run_events (run_id, event_type, payload)
		SELECT NULLIF(metadata->>'telemetry_run_id', '')::uuid, $2, $3
		FROM jobs
		WHERE id = $1 AND NULLIF(metadata->>'telemetry_run_id', '') IS NOT NULL
	`, jobID, eventType, jsonParam(payload))
	return err
}

func markTelemetryRunRunningForJob(ctx context.Context, tx pgx.Tx, jobID int64) error {
	if jobID <= 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE omni_runs
		SET status = 'running', updated_at = NOW()
		WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid
		  AND status = 'pending'
	`, jobID)
	return err
}
