package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func buildV3AuthorityRevisionMetadata(job model.Job, feedback string) ([]byte, int64, int64, error) {
	if job.ID <= 0 {
		return nil, 0, 0, fmt.Errorf("V3 authority revision requires a positive parent job id")
	}
	feedback = SanitizeUTF8Text(strings.TrimSpace(feedback))
	if feedback == "" {
		return nil, 0, 0, fmt.Errorf("V3 authority revision requires non-empty user feedback")
	}
	metadata, err := decodeStrictV3AuthorityMetadata(job.Metadata)
	if err != nil {
		return nil, 0, 0, err
	}
	if _, removed := metadata["scrum_current_user_instruction"]; removed {
		return nil, 0, 0, fmt.Errorf("job metadata key scrum_current_user_instruction was removed; use v3_authority_directives")
	}

	revision, err := positiveMetadataInteger(metadata, "v3_authority_revision", 1)
	if err != nil {
		return nil, 0, 0, err
	}
	rootJobID, err := positiveMetadataInteger(metadata, "v3_root_job_id", job.ID)
	if err != nil {
		return nil, 0, 0, err
	}
	directives, err := v3AuthorityDirectivesFromObject(metadata)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(directives) >= model.V3MaxAuthorityDirectives {
		return nil, 0, 0, fmt.Errorf("V3 authority directive limit %d reached", model.V3MaxAuthorityDirectives)
	}
	directives = append(directives, feedback)

	nextRevision := revision + 1
	metadata["v3_authority_revision"] = nextRevision
	metadata["v3_root_job_id"] = rootJobID
	metadata["v3_parent_job_id"] = job.ID
	metadata["v3_authority_directives"] = directives
	delete(metadata, "telemetry_run_id")
	delete(metadata, "replan_feedback")
	delete(metadata, "superseded_by_job_id")

	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("encode V3 authority revision metadata: %w", err)
	}
	return raw, nextRevision, rootJobID, nil
}

func (r *Repository) reviseV3AuthorityTx(ctx context.Context, tx pgx.Tx, job model.Job, feedback, source string) (model.Job, error) {
	metadata, revision, rootJobID, err := buildV3AuthorityRevisionMetadata(job, feedback)
	if err != nil {
		return model.Job{}, err
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return model.Job{}, fmt.Errorf("V3 authority revision source is required")
	}

	if _, err := cancelJobTx(ctx, tx, job.ID, "superseded by a user authority revision"); err != nil {
		return model.Job{}, err
	}
	successor, err := r.enqueueJobTx(ctx, tx, job.Instruction, job.Pipeline, metadata)
	if err != nil {
		return model.Job{}, fmt.Errorf("enqueue V3 authority revision: %w", err)
	}
	if cardID, scrumJob, err := v3ScrumCardAuthority(job.Metadata); err != nil {
		return model.Job{}, err
	} else if scrumJob {
		result, err := tx.Exec(ctx, `
			UPDATE scrum_cards
			SET job_id = $2, updated_at = NOW()
			WHERE id = $3
			  AND project_id = (SELECT project_id FROM jobs WHERE id = $1)
			  AND job_id = $4
		`, job.ID, strconv.FormatInt(successor.ID, 10), cardID, strconv.FormatInt(job.ID, 10))
		if err != nil {
			return model.Job{}, fmt.Errorf("bind Scrum card %q to V3 authority revision: %w", cardID, err)
		}
		if result.RowsAffected() != 1 {
			return model.Job{}, fmt.Errorf("Scrum card %q is not authoritatively bound to job %d", cardID, job.ID)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET metadata = jsonb_set(metadata, '{superseded_by_job_id}', to_jsonb($2::bigint), true),
		    error = $3,
		    updated_at = NOW()
		WHERE id = $1
	`, job.ID, successor.ID, fmt.Sprintf("superseded by V3 authority revision job %d", successor.ID)); err != nil {
		return model.Job{}, err
	}
	if err := recordTelemetryJobEvent(ctx, tx, job.ID, "authority_superseded", map[string]any{
		"job_id": job.ID, "successor_job_id": successor.ID, "revision": revision, "source": source,
	}); err != nil {
		return model.Job{}, err
	}
	if err := recordTelemetryJobEvent(ctx, tx, successor.ID, "authority_revision_started", map[string]any{
		"job_id": successor.ID, "parent_job_id": job.ID, "root_job_id": rootJobID, "revision": revision, "source": source,
	}); err != nil {
		return model.Job{}, err
	}
	return successor, nil
}

func v3ScrumCardAuthority(raw []byte) (string, bool, error) {
	metadata, err := decodeStrictV3AuthorityMetadata(raw)
	if err != nil {
		return "", false, err
	}
	source, exists := metadata["source"]
	if !exists || source == nil {
		return "", false, nil
	}
	sourceText, ok := source.(string)
	if !ok {
		return "", false, fmt.Errorf("job metadata source must be a string")
	}
	if strings.TrimSpace(sourceText) != "omni-scrum" {
		return "", false, nil
	}
	cardID, ok := metadata["scrum_card_id"].(string)
	if !ok || strings.TrimSpace(cardID) == "" {
		return "", false, fmt.Errorf("V3 Scrum authority revision requires scrum_card_id metadata")
	}
	return strings.TrimSpace(cardID), true, nil
}

func jobUsesV3AuthorityTx(ctx context.Context, tx pgx.Tx, jobID int64) (bool, error) {
	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM job_steps
			WHERE job_id = $1 AND LEFT(action, 3) = 'v3_'
		)
	`, jobID).Scan(&active); err != nil {
		return false, err
	}
	return active, nil
}

func lockedJobTx(ctx context.Context, tx pgx.Tx, jobID int64) (model.Job, error) {
	return scanJob(tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
		FOR UPDATE
	`, jobID))
}

func v3AuthorityDirectivesFromMetadata(raw []byte) ([]string, error) {
	metadata, err := decodeStrictV3AuthorityMetadata(raw)
	if err != nil {
		return nil, err
	}
	return v3AuthorityDirectivesFromObject(metadata)
}

func v3AuthorityDirectivesFromObject(metadata map[string]any) ([]string, error) {
	value, exists := metadata["v3_authority_directives"]
	if !exists || value == nil {
		return []string{}, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("job metadata v3_authority_directives must be an array of strings")
	}
	if len(items) > model.V3MaxAuthorityDirectives {
		return nil, fmt.Errorf("job metadata v3_authority_directives exceeds limit %d", model.V3MaxAuthorityDirectives)
	}
	out := make([]string, 0, len(items))
	for index, item := range items {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("job metadata v3_authority_directives[%d] must be a non-empty string", index)
		}
		out = append(out, strings.TrimSpace(text))
	}
	return out, nil
}

func decodeStrictV3AuthorityMetadata(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var metadata map[string]any
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode V3 authority metadata: %w", err)
	}
	if metadata == nil {
		return nil, fmt.Errorf("V3 authority metadata must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode V3 authority metadata: multiple JSON values")
		}
		return nil, fmt.Errorf("decode V3 authority metadata: %w", err)
	}
	return metadata, nil
}

func positiveMetadataInteger(metadata map[string]any, key string, defaultValue int64) (int64, error) {
	value, exists := metadata[key]
	if !exists || value == nil {
		return defaultValue, nil
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("job metadata %s must be a positive integer", key)
	}
	parsed, err := number.Int64()
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("job metadata %s must be a positive integer", key)
	}
	return parsed, nil
}
