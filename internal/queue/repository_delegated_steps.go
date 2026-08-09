package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	delegatedStepSpacing = 5
	// MaxDelegatedSubtasks is the single queue-authoritative expansion bound.
	MaxDelegatedSubtasks = 6
)

type delegatedAnchor struct {
	sortIndex  int
	generation int64
}

type delegatedContext struct {
	key   string
	value string
}

func (r *Repository) ExpandDelegatedSubtasks(
	ctx context.Context,
	jobID int64,
	anchorStepID int64,
	subtasks []artifacts.Subtask,
) ([]model.Step, error) {
	if err := validateDelegatedExpansion(jobID, anchorStepID, subtasks); err != nil {
		return nil, err
	}
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("postgres repository is not configured")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin delegated expansion for job %d: %w", jobID, err)
	}
	defer tx.Rollback(ctx)

	anchor, err := lockDelegatedAnchorTx(ctx, tx, jobID, anchorStepID)
	if err != nil {
		return nil, err
	}
	if err := rejectForeignCurrentStepsTx(ctx, tx, jobID, anchor.generation); err != nil {
		return nil, err
	}
	shift := len(subtasks) * delegatedStepSpacing
	result, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET sort_index = sort_index + $3, updated_at = NOW()
		WHERE job_id = $1 AND sort_index > $2
		  AND generation = $4
		  AND superseded_at_generation IS NULL
	`, jobID, anchor.sortIndex, shift, anchor.generation)
	if err != nil {
		return nil, fmt.Errorf("shift current tail for job %d: %w", jobID, err)
	}
	if result.RowsAffected() == 0 {
		return nil, fmt.Errorf(
			"%w: planning step %d has no current tail to expand",
			ErrInvalidJobGeneration, anchorStepID,
		)
	}

	created := make([]model.Step, 0, len(subtasks))
	for index, subtask := range subtasks {
		sortIndex := anchor.sortIndex + ((index + 1) * delegatedStepSpacing)
		step, err := scanStep(tx.QueryRow(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status, generation)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, job_id, action, sort_index, status, generation, superseded_at_generation,
			          worker_id, output, error, started_at, finished_at, created_at, updated_at
		`, jobID, delegatedSubtaskAction, sortIndex, model.StepStatusPending, anchor.generation))
		if err != nil {
			return nil, fmt.Errorf("insert delegated step %d for job %d: %w", index, jobID, err)
		}
		for _, item := range delegatedContexts(subtask) {
			if item.value == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO step_contexts (step_id, key, value)
				VALUES ($1, $2, $3)
			`, step.ID, item.key, item.value); err != nil {
				return nil, fmt.Errorf("write delegated step %d context %q: %w", step.ID, item.key, err)
			}
		}
		created = append(created, step)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit delegated expansion for job %d: %w", jobID, err)
	}
	return created, nil
}

func lockDelegatedAnchorTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	anchorStepID int64,
) (delegatedAnchor, error) {
	job, err := lockedJobTx(ctx, tx, jobID)
	if err != nil {
		return delegatedAnchor{}, fmt.Errorf("lock delegated expansion job %d: %w", jobID, err)
	}
	if job.CurrentGeneration <= 0 {
		return delegatedAnchor{}, fmt.Errorf(
			"%w: job %d has invalid generation %d",
			ErrInvalidJobGeneration, jobID, job.CurrentGeneration,
		)
	}
	if job.Status != model.JobStatusRunning {
		return delegatedAnchor{}, fmt.Errorf(
			"%w: delegated expansion job %d status is %q, expected %q",
			ErrStepNotWritable, jobID, job.Status, model.JobStatusRunning,
		)
	}

	var anchor delegatedAnchor
	var action, status string
	var supersededAt *int64
	err = tx.QueryRow(ctx, `
		SELECT action, sort_index, status, generation, superseded_at_generation
		FROM job_steps
		WHERE id = $1 AND job_id = $2
		FOR UPDATE
	`, anchorStepID, jobID).Scan(
		&action, &anchor.sortIndex, &status, &anchor.generation, &supersededAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return delegatedAnchor{}, fmt.Errorf(
			"%w: step %d does not belong to job %d",
			ErrStepNotWritable, anchorStepID, jobID,
		)
	}
	if err != nil {
		return delegatedAnchor{}, fmt.Errorf("lock delegated anchor %d: %w", anchorStepID, err)
	}
	if supersededAt != nil || anchor.generation != job.CurrentGeneration {
		return delegatedAnchor{}, fmt.Errorf(
			"%w: step %d generation %d is not job %d generation %d",
			ErrStaleJobGeneration, anchorStepID, anchor.generation, jobID, job.CurrentGeneration,
		)
	}
	if action != replanPlanningBoundary || status != model.StepStatusRunning {
		return delegatedAnchor{}, fmt.Errorf(
			"%w: step %d is %s/%s, expected %s/%s",
			ErrStepNotWritable, anchorStepID, action, status,
			replanPlanningBoundary, model.StepStatusRunning,
		)
	}
	return anchor, nil
}

func rejectForeignCurrentStepsTx(ctx context.Context, tx pgx.Tx, jobID, generation int64) error {
	var stepID, stepGeneration int64
	err := tx.QueryRow(ctx, `
		SELECT id, generation
		FROM job_steps
		WHERE job_id = $1
		  AND superseded_at_generation IS NULL
		  AND generation <> $2
		ORDER BY id
		FOR UPDATE
		LIMIT 1
	`, jobID, generation).Scan(&stepID, &stepGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("validate current steps for job %d: %w", jobID, err)
	}
	return fmt.Errorf(
		"%w: unsuperseded step %d belongs to generation %d, expected %d",
		ErrInvalidJobGeneration, stepID, stepGeneration, generation,
	)
}

func validateDelegatedExpansion(jobID, anchorStepID int64, subtasks []artifacts.Subtask) error {
	if jobID <= 0 || anchorStepID <= 0 {
		return fmt.Errorf("delegated expansion requires positive job and anchor step identities")
	}
	if len(subtasks) == 0 {
		return fmt.Errorf("delegated expansion requires at least one subtask")
	}
	if len(subtasks) > MaxDelegatedSubtasks {
		return fmt.Errorf("delegated expansion exceeds the %d-step limit", MaxDelegatedSubtasks)
	}
	seen := make(map[string]struct{}, len(subtasks))
	for index, subtask := range subtasks {
		id := strings.TrimSpace(subtask.ID)
		if id == "" {
			return fmt.Errorf("delegated subtask %d requires an identity", index)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("delegated subtask identity %q is duplicated", id)
		}
		seen[id] = struct{}{}
		switch strings.TrimSpace(subtask.Kind) {
		case artifacts.SubtaskKindResearch, artifacts.SubtaskKindAnalyze, artifacts.SubtaskKindExecute:
		default:
			return fmt.Errorf("delegated subtask %q has invalid kind %q", id, subtask.Kind)
		}
		if strings.TrimSpace(subtask.RoleID) == "" || strings.TrimSpace(subtask.ObjectiveID) == "" ||
			strings.TrimSpace(subtask.Objective) == "" {
			return fmt.Errorf("delegated subtask %q requires role and objective authority", id)
		}
		if subtask.Priority < 1 || subtask.Priority > 100 {
			return fmt.Errorf("delegated subtask %q priority must be between 1 and 100", id)
		}
		if !containsNonempty(subtask.SuccessCriteria) {
			return fmt.Errorf("delegated subtask %q requires success criteria", id)
		}
	}
	return nil
}

func containsNonempty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func delegatedContexts(subtask artifacts.Subtask) []delegatedContext {
	return []delegatedContext{
		{key: "subtask_id", value: strings.TrimSpace(subtask.ID)},
		{key: "subtask_kind", value: strings.TrimSpace(subtask.Kind)},
		{key: "subtask_role_id", value: strings.TrimSpace(subtask.RoleID)},
		{key: "subtask_objective_id", value: strings.TrimSpace(subtask.ObjectiveID)},
		{key: "subtask_objective", value: strings.TrimSpace(subtask.Objective)},
		{key: "subtask_priority", value: fmt.Sprintf("%d", subtask.Priority)},
		{key: "subtask_capabilities", value: strings.Join(subtask.RequiredCapabilities, ", ")},
		{key: "subtask_constraints", value: strings.Join(subtask.Constraints, " | ")},
		{key: "subtask_success", value: strings.Join(subtask.SuccessCriteria, " | ")},
	}
}
