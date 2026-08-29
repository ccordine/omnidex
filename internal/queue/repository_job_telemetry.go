package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func createTelemetryRunForJob(ctx context.Context, tx pgx.Tx, job model.Job, projectID *int64) (string, error) {
	if job.ID <= 0 {
		return "", fmt.Errorf("create telemetry run requires a positive job ID")
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
	modelRolesJSON, err := encodeTelemetryJSON(
		"run model roles", metadataValue(metadata, "model_roles"),
	)
	if err != nil {
		return "", err
	}
	summaryJSON, err := encodeTelemetryJSON("run summary", summary)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO omni_runs (session_id, workspace_id, task_kind, prompt_hash, prompt_summary, project_type, status, started_at, local_only, model_roles, summary)
		VALUES (NULLIF($1,''), NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), $7, $8, $9, $10, $11)
		RETURNING id::text
	`, firstMetadataString(metadata, "session_id"), workspaceID, taskKind, promptHash, promptSummary, projectType, "pending", job.CreatedAt, true, modelRolesJSON, summaryJSON).Scan(&id)
	return id, err
}

func completeTelemetryRunForJob(ctx context.Context, tx pgx.Tx, jobID int64, status string, summary any, completionEvidence any) error {
	if jobID <= 0 {
		return fmt.Errorf("complete telemetry run requires a positive job ID")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	summaryJSON, err := encodeTelemetryJSON("run completion summary", summary)
	if err != nil {
		return err
	}
	completionEvidenceJSON, err := encodeTelemetryJSON(
		"run completion evidence", completionEvidence,
	)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE omni_runs
		SET status = $2,
		    finished_at = NOW(),
		    duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint),
		    summary = $3,
		    completion_evidence = $4,
		    updated_at = NOW()
		WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid
	`, jobID, status, summaryJSON, completionEvidenceJSON)
	return err
}

func recordTelemetryJobEvent(ctx context.Context, tx pgx.Tx, jobID int64, eventType string, payload any) error {
	if jobID <= 0 {
		return fmt.Errorf("record telemetry job event requires a positive job ID")
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return fmt.Errorf("record telemetry job event requires an event type")
	}
	payloadJSON, err := encodeTelemetryJSON("job event payload", payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO omni_run_events (run_id, event_type, payload)
		SELECT NULLIF(metadata->>'telemetry_run_id', '')::uuid, $2, $3
		FROM jobs
		WHERE id = $1 AND NULLIF(metadata->>'telemetry_run_id', '') IS NOT NULL
	`, jobID, eventType, payloadJSON)
	return err
}

func markTelemetryRunRunningForJob(ctx context.Context, tx pgx.Tx, jobID int64) error {
	if jobID <= 0 {
		return fmt.Errorf("mark telemetry run running requires a positive job ID")
	}
	_, err := tx.Exec(ctx, `
		UPDATE omni_runs
		SET status = 'running', updated_at = NOW()
		WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid
		  AND status = 'pending'
	`, jobID)
	return err
}
