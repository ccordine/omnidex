package queue

import (
	"context"
	"errors"
	"fmt"
	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (r *Repository) ListClaimSupportByJob(ctx context.Context, jobID int64, limit int) ([]model.ClaimSupportDetail, error) {
	if jobID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			cs.id,
			cs.claim_id,
			c.text,
			c.status,
			cs.evidence_id,
			COALESCE(e.kind, ''),
			COALESCE(e.source_ref, ''),
			cs.support_score,
			COALESCE(cs.rationale, ''),
			cs.created_at
		FROM claim_support cs
		JOIN claims c ON c.id = cs.claim_id
		LEFT JOIN evidence e ON e.id = cs.evidence_id
		WHERE c.job_id = $1
		ORDER BY cs.id ASC
		LIMIT $2
	`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.ClaimSupportDetail, 0, limit)
	for rows.Next() {
		var item model.ClaimSupportDetail
		if err := rows.Scan(&item.ID, &item.ClaimID, &item.ClaimText, &item.ClaimStatus, &item.EvidenceID, &item.EvidenceKind, &item.EvidenceSourceRef, &item.SupportScore, &item.Rationale, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) WriteMemoryCandidate(ctx context.Context, candidate model.MemoryCandidate) (int64, error) {
	if strings.TrimSpace(candidate.CandidateKind) == "" || strings.TrimSpace(candidate.Content) == "" {
		return 0, errors.New("memory candidate kind and content are required")
	}
	provenance := strings.TrimSpace(string(candidate.Provenance))
	if provenance == "" {
		provenance = `{}`
	}
	var id int64
	err := r.pool.QueryRow(ctx, `
        INSERT INTO memory_candidates (job_id, source_memory_id, candidate_kind, content, provenance, confidence, status)
        VALUES ($1, $2, $3, $4, $5::jsonb, $6, COALESCE(NULLIF($7, ''), 'candidate'))
        RETURNING id
    `, candidate.JobID, candidate.SourceMemoryID, candidate.CandidateKind, candidate.Content, provenance, candidate.Confidence, candidate.Status).Scan(&id)
	return id, err
}

func (r *Repository) ListMemoryCandidates(ctx context.Context, jobID int64, status string, limit int) ([]model.MemoryCandidate, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := r.pool.Query(ctx, `
        SELECT id, job_id, source_memory_id, candidate_kind, content, provenance, confidence, status, created_at, updated_at
        FROM memory_candidates
        WHERE ($1 = 0 OR job_id = $1)
          AND ($2 = '' OR status = $2)
        ORDER BY confidence DESC, id ASC
        LIMIT $3
    `, jobID, strings.TrimSpace(status), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.MemoryCandidate, 0, limit)
	for rows.Next() {
		var item model.MemoryCandidate
		if err := rows.Scan(&item.ID, &item.JobID, &item.SourceMemoryID, &item.CandidateKind, &item.Content, &item.Provenance, &item.Confidence, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetMemoryCandidate(ctx context.Context, id int64) (model.MemoryCandidate, error) {
	if id <= 0 {
		return model.MemoryCandidate{}, pgx.ErrNoRows
	}
	var item model.MemoryCandidate
	err := r.pool.QueryRow(ctx, `
        SELECT id, job_id, source_memory_id, candidate_kind, content, provenance, confidence, status, created_at, updated_at
        FROM memory_candidates
        WHERE id = $1
    `, id).Scan(&item.ID, &item.JobID, &item.SourceMemoryID, &item.CandidateKind, &item.Content, &item.Provenance, &item.Confidence, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *Repository) GetJobInspection(ctx context.Context, jobID int64, limit int) (model.JobInspection, error) {
	if jobID <= 0 {
		return model.JobInspection{}, nil
	}
	details, err := r.GetJobDetails(ctx, jobID)
	if err != nil {
		return model.JobInspection{}, err
	}
	artifactsList, err := r.ListArtifactsByJob(ctx, jobID, limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	evidenceList, err := r.ListEvidenceByJob(ctx, jobID, limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	claims, err := r.ListClaimsByJob(ctx, jobID, limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	support, err := r.ListClaimSupportByJob(ctx, jobID, limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	memoryCandidates, err := r.ListMemoryCandidates(ctx, jobID, "", limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	return model.JobInspection{
		Job:              details.Job,
		JobID:            jobID,
		Artifacts:        artifactsList,
		Evidence:         evidenceList,
		Claims:           claims,
		ClaimSupport:     support,
		MemoryCandidates: memoryCandidates,
	}, nil
}

func (r *Repository) UpdateMemoryCandidateStatus(ctx context.Context, id int64, status string) error {
	if id <= 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
        UPDATE memory_candidates
        SET status = $2, updated_at = NOW()
        WHERE id = $1
    `, id, strings.TrimSpace(status))
	return err
}

func (r *Repository) DeleteMemoryChunk(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid memory id")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM memory_chunks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteMemoryCandidate(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid candidate id")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM memory_candidates WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) MindStats(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	queries := map[string]string{
		"memory_chunks":     `SELECT COUNT(*) FROM memory_chunks`,
		"memory_candidates": `SELECT COUNT(*) FROM memory_candidates`,
		"candidate_pending": `SELECT COUNT(*) FROM memory_candidates WHERE status = 'candidate'`,
		"jobs":              `SELECT COUNT(*) FROM jobs`,
		"telemetry_events":  `SELECT COUNT(*) FROM omni_run_events`,
	}
	for key, query := range queries {
		var count int64
		if err := r.pool.QueryRow(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		out[key] = count
	}
	return out, nil
}

func (r *Repository) CountStepsByAction(ctx context.Context, jobID int64, action string) (int, error) {
	if jobID <= 0 || strings.TrimSpace(action) == "" {
		return 0, nil
	}
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1 AND action = $2
	`, jobID, strings.TrimSpace(action)).Scan(&count)
	return count, err
}

func (r *Repository) ExpandDelegatedSubtasks(ctx context.Context, jobID int64, anchorStepID int64, subtasks []artifacts.Subtask) ([]model.Step, error) {
	if jobID <= 0 || anchorStepID <= 0 || len(subtasks) == 0 {
		return nil, nil
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var anchorSort int
	if err := tx.QueryRow(ctx, `SELECT sort_index FROM job_steps WHERE id = $1 AND job_id = $2 FOR UPDATE`, anchorStepID, jobID).Scan(&anchorSort); err != nil {
		return nil, err
	}
	spacing := 5
	shift := len(subtasks) * spacing
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET sort_index = sort_index + $3, updated_at = NOW()
		WHERE job_id = $1 AND sort_index > $2
	`, jobID, anchorSort, shift); err != nil {
		return nil, err
	}
	created := make([]model.Step, 0, len(subtasks))
	for idx, subtask := range subtasks {
		sortIndex := anchorSort + ((idx + 1) * spacing)
		row := tx.QueryRow(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status)
			VALUES ($1, $2, $3, $4)
			RETURNING id, job_id, action, sort_index, status, worker_id, output, error, started_at, finished_at, created_at, updated_at
		`, jobID, "v3_subtask", sortIndex, model.StepStatusPending)
		step, err := scanStep(row)
		if err != nil {
			return nil, err
		}
		contexts := map[string]string{
			"subtask_id":           strings.TrimSpace(subtask.ID),
			"subtask_kind":         strings.TrimSpace(subtask.Kind),
			"subtask_role_id":      strings.TrimSpace(subtask.RoleID),
			"subtask_objective_id": strings.TrimSpace(subtask.ObjectiveID),
			"subtask_objective":    strings.TrimSpace(subtask.Objective),
			"subtask_priority":     fmt.Sprintf("%d", subtask.Priority),
			"subtask_capabilities": strings.Join(subtask.RequiredCapabilities, ", "),
			"subtask_constraints":  strings.Join(subtask.Constraints, " | "),
			"subtask_success":      strings.Join(subtask.SuccessCriteria, " | "),
		}
		for key, value := range contexts {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO step_contexts (step_id, key, value)
				VALUES ($1, $2, $3)
			`, step.ID, key, value); err != nil {
				return nil, err
			}
		}
		created = append(created, step)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return created, nil
}

func (r *Repository) WriteClaims(ctx context.Context, claims []model.ClaimRecord) ([]model.ClaimRecord, error) {
	if len(claims) == 0 {
		return nil, nil
	}
	saved := make([]model.ClaimRecord, 0, len(claims))
	for _, claim := range claims {
		var created model.ClaimRecord
		err := r.pool.QueryRow(ctx, `
            INSERT INTO claims (job_id, step_id, text, normalized_text, status, confidence)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING id, created_at
        `, claim.JobID, claim.StepID, claim.Text, claim.NormalizedText, claim.Status, claim.Confidence).Scan(&created.ID, &created.CreatedAt)
		if err != nil {
			return nil, err
		}
		claim.ID = created.ID
		claim.CreatedAt = created.CreatedAt
		saved = append(saved, claim)
	}
	return saved, nil
}

func (r *Repository) WriteClaimSupports(ctx context.Context, supports []model.ClaimSupportRecord) error {
	if len(supports) == 0 {
		return nil
	}
	for _, support := range supports {
		if support.ClaimID <= 0 || support.EvidenceID <= 0 {
			continue
		}
		if _, err := r.pool.Exec(ctx, `
            INSERT INTO claim_support (claim_id, evidence_id, support_score, rationale)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT (claim_id, evidence_id) DO UPDATE
            SET support_score = EXCLUDED.support_score,
                rationale = EXCLUDED.rationale
        `, support.ClaimID, support.EvidenceID, support.SupportScore, support.Rationale); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListJobs(ctx context.Context, status string, limit, offset int) ([]model.Job, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{}
	query := `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
	`

	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}

	query += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]model.Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *Repository) GetJobDetails(ctx context.Context, jobID int64) (model.JobDetails, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return model.JobDetails{}, err
	}

	stepsRows, err := r.pool.Query(ctx, `
		SELECT id, job_id, action, sort_index, status, worker_id, output, error, started_at, finished_at, created_at, updated_at
		FROM job_steps
		WHERE job_id = $1
		ORDER BY sort_index ASC, id ASC
	`, jobID)
	if err != nil {
		return model.JobDetails{}, err
	}
	defer stepsRows.Close()

	steps := []model.Step{}
	for stepsRows.Next() {
		step, err := scanStep(stepsRows)
		if err != nil {
			return model.JobDetails{}, err
		}
		steps = append(steps, step)
	}
	if err := stepsRows.Err(); err != nil {
		return model.JobDetails{}, err
	}

	ctxRows, err := r.pool.Query(ctx, `
		SELECT c.id, c.step_id, c.key, c.value, c.created_at
		FROM step_contexts c
		JOIN job_steps s ON s.id = c.step_id
		WHERE s.job_id = $1
		ORDER BY c.id ASC
	`, jobID)
	if err != nil {
		return model.JobDetails{}, err
	}
	defer ctxRows.Close()

	contexts := []model.StepContext{}
	for ctxRows.Next() {
		ctxValue, err := scanStepContext(ctxRows)
		if err != nil {
			return model.JobDetails{}, err
		}
		contexts = append(contexts, ctxValue)
	}
	if err := ctxRows.Err(); err != nil {
		return model.JobDetails{}, err
	}

	return model.JobDetails{Job: job, Steps: steps, Contexts: contexts}, nil
}

func (r *Repository) JobProjectID(ctx context.Context, jobID int64) (int64, error) {
	if jobID <= 0 {
		return 0, nil
	}
	var projectID *int64
	err := r.pool.QueryRow(ctx, `SELECT project_id FROM jobs WHERE id = $1`, jobID).Scan(&projectID)
	if err != nil {
		return 0, err
	}
	if projectID == nil || *projectID <= 0 {
		return 0, nil
	}
	return *projectID, nil
}

func (r *Repository) JobIDForStep(ctx context.Context, stepID int64) (int64, error) {
	if stepID <= 0 {
		return 0, nil
	}
	var jobID int64
	err := r.pool.QueryRow(ctx, `SELECT job_id FROM job_steps WHERE id = $1`, stepID).Scan(&jobID)
	return jobID, err
}

func (r *Repository) ListRecentSessionJobs(ctx context.Context, pipeline, sessionID string, beforeJobID int64, limit int) ([]model.Job, error) {
	pipeline = normalizePipeline(pipeline)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || beforeJobID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 6
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE pipeline = $1
		  AND COALESCE(metadata->>'session_id', '') = $2
		  AND id < $3
		ORDER BY id DESC
		LIMIT $4
	`, pipeline, sessionID, beforeJobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]model.Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(jobs)-1; i < j; i, j = i+1, j-1 {
		jobs[i], jobs[j] = jobs[j], jobs[i]
	}
	return jobs, nil
}

func (r *Repository) GetStepRuntimeState(ctx context.Context, jobID, stepID int64) (string, string, error) {
	var jobStatus string
	var stepStatus string
	err := r.pool.QueryRow(ctx, `
		SELECT j.status, s.status
		FROM jobs j
		JOIN job_steps s ON s.job_id = j.id
		WHERE j.id = $1 AND s.id = $2
	`, jobID, stepID).Scan(&jobStatus, &stepStatus)
	if err != nil {
		return "", "", err
	}
	return jobStatus, stepStatus, nil
}

func (r *Repository) AddStepContext(ctx context.Context, stepID int64, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("step context key is required")
	}
	value = SanitizeUTF8Text(value)
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, stepID, key, value); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, `
		UPDATE job_steps
		SET updated_at = NOW()
		WHERE id = $1
	`, stepID); err != nil {
		return err
	}
	return nil
}

func (r *Repository) ClaimNextStep(ctx context.Context, workerID string) (*model.ClaimedStep, error) {
	if paused, err := r.IsAIPaused(ctx); err != nil {
		return nil, err
	} else if paused {
		return nil, nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT
			s.id, s.job_id, s.action, s.sort_index, s.status, s.worker_id, s.output, s.error,
			s.started_at, s.finished_at, s.created_at, s.updated_at,
			j.id, j.instruction, j.pipeline, j.status, j.result, j.error, j.metadata, j.created_at, j.updated_at, j.completed_at
		FROM job_steps s
		JOIN jobs j ON j.id = s.job_id
		WHERE s.status = $1
		  AND j.status IN ($2, $3)
		  AND NOT EXISTS (
		      SELECT 1
		      FROM job_steps prev
		      WHERE prev.job_id = s.job_id
		        AND prev.sort_index < s.sort_index
		        AND prev.status <> $4
		  )
		ORDER BY s.sort_index ASC, s.id ASC
		FOR UPDATE OF s SKIP LOCKED
		LIMIT 1
	`, model.StepStatusPending, model.JobStatusPending, model.JobStatusRunning, model.StepStatusCompleted)

	step, job, err := scanClaim(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, worker_id = $3, started_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, step.ID, model.StepStatusRunning, workerID); err != nil {
		return nil, err
	}
	step.Status = model.StepStatusRunning
	step.WorkerID = workerID

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, job.ID, model.JobStatusRunning, model.JobStatusPending); err != nil {
		return nil, err
	}
	job.Status = model.JobStatusRunning

	if err := markTelemetryRunRunningForJob(ctx, tx, job.ID); err != nil {
		return nil, err
	}
	if err := recordTelemetryJobEvent(ctx, tx, job.ID, "run_running", map[string]any{
		"job_id":  job.ID,
		"step_id": step.ID,
		"action":  step.Action,
	}); err != nil {
		return nil, err
	}

	ctxRows, err := tx.Query(ctx, `
		SELECT c.id, c.step_id, c.key, c.value, c.created_at
		FROM step_contexts c
		JOIN job_steps s ON s.id = c.step_id
		WHERE s.job_id = $1
		  AND (s.status = $2 OR s.id = $3)
		ORDER BY c.id ASC
	`, job.ID, model.StepStatusCompleted, step.ID)
	if err != nil {
		return nil, err
	}
	defer ctxRows.Close()

	contexts := make([]model.StepContext, 0, 8)
	for ctxRows.Next() {
		ctxValue, err := scanStepContext(ctxRows)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, ctxValue)
	}
	if err := ctxRows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &model.ClaimedStep{Job: job, Step: step, Contexts: contexts}, nil
}

// AppendStepOutput appends text to a running step's output (used for live external-agent streaming).
func (r *Repository) AppendStepOutput(ctx context.Context, stepID int64, delta string) error {
	delta = SanitizeUTF8Text(delta)
	if delta == "" {
		return nil
	}
	if !strings.HasSuffix(delta, "\n") {
		delta += "\n"
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE job_steps
		SET output = COALESCE(output, '') || $2,
		    updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, stepID, delta, model.StepStatusRunning)
	return err
}
