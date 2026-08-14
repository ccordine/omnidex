package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

var (
	ErrProjectNotFound        = errors.New("project not found")
	ErrProjectVersionConflict = errors.New("project version conflict")
	ErrProjectActiveWork      = errors.New("project has active work")
)

func scanProject(row pgx.Row) (model.Project, error) {
	var project model.Project
	var settings []byte
	err := row.Scan(
		&project.ID, &project.Location, &project.Name, &project.Description,
		&project.ProjectState, &settings,
		&project.LastSeenAt, &project.CreatedAt, &project.UpdatedAt,
	)
	if err != nil {
		return model.Project{}, err
	}
	project.Settings = json.RawMessage(settings)
	return project, nil
}

const projectSelectColumns = `
	id, location, name, description, project_state, settings,
	last_seen_at, created_at, updated_at
`

func (r *Repository) ListProjects(ctx context.Context, limit, offset int) ([]model.Project, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+projectSelectColumns+` FROM projects
		ORDER BY updated_at DESC, id DESC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []model.Project{}
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (r *Repository) GetProject(ctx context.Context, id int64) (model.Project, error) {
	project, err := scanProject(r.pool.QueryRow(ctx, `
		SELECT `+projectSelectColumns+` FROM projects WHERE id=$1
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Project{}, ErrProjectNotFound
	}
	return project, err
}

func (r *Repository) GetProjectByLocation(ctx context.Context, location string) (model.Project, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return model.Project{}, fmt.Errorf("location is required")
	}
	project, err := scanProject(r.pool.QueryRow(ctx, `
		SELECT `+projectSelectColumns+` FROM projects WHERE location=$1
	`, location))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Project{}, ErrProjectNotFound
	}
	return project, err
}

func (r *Repository) CreateProject(
	ctx context.Context,
	name, location, description string,
) (model.Project, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return model.Project{}, fmt.Errorf("location is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = projectNameFromLocation(location)
	}
	name = SanitizeUTF8Text(name)
	description = SanitizeUTF8Text(strings.TrimSpace(description))
	return scanProject(r.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name,description,last_seen_at)
		VALUES($1,$2,$3,NOW()) RETURNING `+projectSelectColumns,
		location, name, description,
	))
}

func (r *Repository) UpdateProjectAtRevision(
	ctx context.Context,
	id int64,
	expectedUpdatedAt time.Time,
	patch model.ProjectPatch,
) (model.Project, error) {
	if r == nil || r.pool == nil || ctx == nil || id <= 0 || expectedUpdatedAt.IsZero() {
		return model.Project{}, fmt.Errorf("revision-bound project update requires PostgreSQL, context, project, and expected revision")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Project{}, fmt.Errorf("begin revision-bound project update: %w", err)
	}
	defer rollbackTx(ctx, tx, "revision-bound project update")
	current, err := scanProject(tx.QueryRow(ctx, `SELECT `+projectSelectColumns+` FROM projects WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Project{}, ErrProjectNotFound
	}
	if err != nil {
		return model.Project{}, err
	}
	if !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return model.Project{}, fmt.Errorf("%w: project %d changed; reload server state and retry", ErrProjectVersionConflict, id)
	}
	if patch.Name != nil && strings.TrimSpace(*patch.Name) != "" {
		current.Name = *patch.Name
	}
	if patch.Location != nil && strings.TrimSpace(*patch.Location) != "" {
		current.Location = strings.TrimSpace(*patch.Location)
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	if patch.ProjectState != nil {
		current.ProjectState = strings.TrimSpace(*patch.ProjectState)
	}
	if patch.Settings != nil {
		current.Settings = *patch.Settings
	}
	current.Settings = defaultJSON(current.Settings, `{}`)
	if err := validateProjectSettings(current.Settings); err != nil {
		return model.Project{}, err
	}
	current.Name = SanitizeUTF8Text(current.Name)
	current.Location = SanitizeUTF8Text(current.Location)
	current.Description = SanitizeUTF8Text(current.Description)
	current.ProjectState = SanitizeUTF8Text(current.ProjectState)
	current.Settings = SanitizeUTF8Bytes(current.Settings)
	updated, err := scanProject(tx.QueryRow(ctx, `
		UPDATE projects SET name=$2,location=$3,description=$4,
		 project_state=$5,settings=$6::jsonb,
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE id=$1 AND updated_at=$7 RETURNING `+projectSelectColumns,
		id, current.Name, current.Location, current.Description,
		current.ProjectState, string(current.Settings), expectedUpdatedAt,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Project{}, fmt.Errorf("%w: project %d changed during mutation", ErrProjectVersionConflict, id)
	}
	if err != nil {
		return model.Project{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Project{}, fmt.Errorf("commit revision-bound project update: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteProjectAtRevision(ctx context.Context, id int64, expectedUpdatedAt time.Time) error {
	if r == nil || r.pool == nil || ctx == nil || id <= 0 || expectedUpdatedAt.IsZero() {
		return fmt.Errorf("revision-bound project deletion requires PostgreSQL, context, project, and expected revision")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin revision-bound project deletion: %w", err)
	}
	defer rollbackTx(ctx, tx, "revision-bound project deletion")
	var currentUpdatedAt time.Time
	err = tx.QueryRow(ctx, `SELECT updated_at FROM projects WHERE id=$1 FOR UPDATE`, id).Scan(&currentUpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProjectNotFound
	}
	if err != nil {
		return fmt.Errorf("lock project for revision-bound deletion: %w", err)
	}
	if !currentUpdatedAt.Equal(expectedUpdatedAt) {
		return fmt.Errorf("%w: project %d changed; reload server state and retry", ErrProjectVersionConflict, id)
	}
	var activeCardID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM scrum_cards
		WHERE project_id=$1 AND (play_state IN ('running','queued') OR sync_job_id<>'')
		ORDER BY id LIMIT 1 FOR SHARE
	`, id).Scan(&activeCardID)
	if err == nil {
		return fmt.Errorf("%w: Scrum card %q must be paused before project deletion", ErrProjectActiveWork, activeCardID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("inspect active Scrum cards before project deletion: %w", err)
	}
	var activeJobID int64
	var activeJobStatus string
	err = tx.QueryRow(ctx, `
		SELECT id,status FROM jobs
		WHERE project_id=$1 AND status NOT IN ($2,$3,$4)
		ORDER BY id LIMIT 1 FOR SHARE
	`, id, model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled).
		Scan(&activeJobID, &activeJobStatus)
	if err == nil {
		return fmt.Errorf("%w: job %d remains %q", ErrProjectActiveWork, activeJobID, activeJobStatus)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("inspect nonterminal jobs before project deletion: %w", err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM projects WHERE id=$1 AND updated_at=$2`, id, expectedUpdatedAt)
	if err != nil {
		return fmt.Errorf("delete revision-bound project: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: project %d changed during deletion", ErrProjectVersionConflict, id)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit revision-bound project deletion: %w", err)
	}
	return nil
}

func (r *Repository) TouchProject(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE projects SET last_seen_at=NOW(),updated_at=NOW() WHERE id=$1
	`, id)
	return err
}

func (r *Repository) CountProjectJobs(ctx context.Context, projectID int64) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE project_id=$1`, projectID).Scan(&count)
	return count, err
}

func (r *Repository) CountProjectCards(ctx context.Context, projectID int64) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM scrum_cards WHERE project_id=$1`, projectID).Scan(&count)
	return count, err
}

func (r *Repository) HasRunningScrumPlay(ctx context.Context) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM scrum_cards WHERE play_state='running' LIMIT 1)
	`).Scan(&exists)
	return exists, err
}

func defaultJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) > 0 {
		return raw
	}
	return json.RawMessage(fallback)
}

func ProjectNameFromLocation(location string) string { return projectNameFromLocation(location) }

func NormalizeProjectLocation(location string) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", fmt.Errorf("location is required")
	}
	abs, err := filepath.Abs(location)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
